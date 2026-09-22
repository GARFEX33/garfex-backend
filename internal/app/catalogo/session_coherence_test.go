package catalogo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

func TestCatalogMutationsMatchFreshCatalogAndFailuresRemainInvisible(t *testing.T) {
	persistErr := errors.New("persist failed")
	cases := []struct {
		name    string
		initial domain.ResourceFamily
		current domain.CatalogRecord
		mutate  func(*Service) error
		want    domain.ResourceFamily
	}{
		{name: "create", initial: family("CONDUCTORES", "Conductores", true),
			mutate: func(s *Service) error {
				_, err := s.Create(context.Background(), domain.KindFamily, familyRecord(0, "CANALIZACIONES", "Canalizaciones", true))
				return err
			},
			want: family("CANALIZACIONES", "Canalizaciones", true)},
		{name: "update including allowed code rename", initial: family("CONDUCTORES", "Conductores", true), current: familyRecord(5, "CONDUCTORES", "Conductores", true),
			mutate: func(s *Service) error {
				_, err := s.Update(context.Background(), domain.KindFamily, familyRecord(5, "CABLES", "Cables", true))
				return err
			},
			want: family("CABLES", "Cables", true)},
		{name: "deactivate", initial: family("CONDUCTORES", "Conductores", true), current: familyRecord(5, "CONDUCTORES", "Conductores", true),
			mutate: func(s *Service) error { return s.Deactivate(context.Background(), domain.KindFamily, 5) }, want: family("CONDUCTORES", "Conductores", false)},
		{name: "reactivate", initial: family("CONDUCTORES", "Conductores", false), current: familyRecord(5, "CONDUCTORES", "Conductores", false),
			mutate: func(s *Service) error { return s.Reactivate(context.Background(), domain.KindFamily, 5) }, want: family("CONDUCTORES", "Conductores", true)},
	}
	for _, tt := range cases {
		t.Run(tt.name+" publishes the fresh-session shape", func(t *testing.T) {
			initial := baseSnapshot()
			initial.Families = []domain.ResourceFamily{tt.initial}
			authority := domain.NewCatalogAuthority(initial)
			repo := &fakeCatalogAdminRepository{getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
				return tt.current, nil
			}}
			if err := tt.mutate(NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority)); err != nil {
				t.Fatal(err)
			}
			got, version := authority.Current()
			fresh := initial
			if tt.name == "create" {
				fresh.Families = append(append([]domain.ResourceFamily(nil), initial.Families...), tt.want)
			} else {
				fresh.Families = []domain.ResourceFamily{tt.want}
			}
			if version != 2 || !reflect.DeepEqual(got, fresh) {
				t.Fatalf("same-session catalog = %+v at v%d, fresh catalog = %+v", got.Families, version, fresh.Families)
			}
		})
		t.Run(tt.name+" persistence failure retains the previous version", func(t *testing.T) {
			initial := baseSnapshot()
			initial.Families = []domain.ResourceFamily{tt.initial}
			authority := domain.NewCatalogAuthority(initial)
			repo := &fakeCatalogAdminRepository{getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
				return tt.current, nil
			},
				insertFn: func(context.Context, domain.CatalogRecord) (int64, error) { return 0, persistErr }, updateFn: func(context.Context, domain.CatalogRecord) error { return persistErr },
				setActiveFn: func(context.Context, domain.CatalogKindCode, int64, bool) error { return persistErr }}
			if err := tt.mutate(NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority)); !errors.Is(err, persistErr) {
				t.Fatalf("mutation error = %v, want persistence failure", err)
			}
			got, version := authority.Current()
			if version != 1 || !reflect.DeepEqual(got, initial) {
				t.Fatalf("failed mutation exposed version %d", version)
			}
		})
	}
}

func TestCatalogHardDeletePublicationFollowsAuthoritativeGuards(t *testing.T) {
	persistErr := errors.New("delete persist failed")
	cases := []struct {
		name           string
		dependencies   []domain.CatalogDependency
		referenced     bool
		deleteErr      error
		wantErr        error
		wantVersion    uint64
		wantDeleteCall int
		wantFamilies   int
	}{
		{
			name: "non-blocking historical dependency",
			dependencies: []domain.CatalogDependency{{
				Kind: domain.KindPresentationField, Count: 1, Blocking: false,
			}},
			wantErr:        domain.ErrCatalogInUse,
			wantVersion:    1,
			wantDeleteCall: 0,
			wantFamilies:   1,
		},
		{
			name:           "resource history reference",
			referenced:     true,
			wantErr:        domain.ErrCatalogInUse,
			wantVersion:    1,
			wantDeleteCall: 0,
			wantFamilies:   1,
		},
		{
			name:           "repository delete failure",
			deleteErr:      persistErr,
			wantErr:        persistErr,
			wantVersion:    1,
			wantDeleteCall: 1,
			wantFamilies:   1,
		},
		{
			name:           "inactive dependency-free target publishes after delete",
			wantVersion:    2,
			wantDeleteCall: 1,
			wantFamilies:   0,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			initial := baseSnapshot()
			initial.Families = []domain.ResourceFamily{family("CONDUCTORES", "Conductores", false)}
			authority := domain.NewCatalogAuthority(initial)
			repo := &fakeCatalogAdminRepository{
				getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
					return familyRecord(5, "CONDUCTORES", "Conductores", false), nil
				},
				dependentsFn: func(context.Context, domain.CatalogKindCode, int64) ([]domain.CatalogDependency, error) {
					return tt.dependencies, nil
				},
				referencedByResourcesFn: func(context.Context, domain.CatalogKindCode, int64) (bool, error) {
					return tt.referenced, nil
				},
				deleteFn: func(context.Context, domain.CatalogKindCode, int64) error {
					return tt.deleteErr
				},
			}
			svc := NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority)
			before := initial

			err := svc.Delete(context.Background(), domain.KindFamily, 5)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Delete() error = %v, want %v", err, tt.wantErr)
			}
			got, version := authority.Current()
			if version != tt.wantVersion || len(got.Families) != tt.wantFamilies {
				t.Fatalf("published catalog = families %d at version %d, want %d at version %d", len(got.Families), version, tt.wantFamilies, tt.wantVersion)
			}
			if tt.wantVersion == 1 && !reflect.DeepEqual(got, before) {
				t.Fatal("authority changed after rejected or failed hard delete")
			}
			if len(svc.snapshot.Families) != len(got.Families) {
				t.Fatalf("service snapshot families = %d, published families = %d", len(svc.snapshot.Families), len(got.Families))
			}
			if repo.deleteCalls != tt.wantDeleteCall {
				t.Fatalf("repo.Delete called %d times, want %d", repo.deleteCalls, tt.wantDeleteCall)
			}
		})
	}
}

// TestCatalogWriteIndeterminateLatchesWriterUnavailable proves 4C's
// writer-unavailable latch (design "Catalog transaction and coherent
// publication"): a repoV2 error wrapping ErrCatalogWriteIndeterminate
// publishes nothing and blocks every subsequent V2 write — even one that
// would otherwise succeed — until ResetCatalogWriterAvailability runs.
func TestServiceV2WriteIndeterminateLatchesWriterUnavailable(t *testing.T) {
	attempts := 0
	repoV2 := &fakeCatalogAdminRepositoryV2{
		insertFn: func(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
			attempts++
			return domain.CatalogWriteResult{}, fmt.Errorf("commit outcome unknown: %w", ErrCatalogWriteIndeterminate)
		},
	}
	authority := domain.NewCatalogAuthority(baseSnapshot())
	svc := NewServiceWithCatalogAuthority(&fakeCatalogAdminRepository{}, domain.NewCatalogRegistry(), authority).WithCatalogAdminRepositoryV2(repoV2)

	valid := domain.CatalogRecord{Active: true, Values: map[string]domain.CatalogValue{
		"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}}, "code": {Text: "CANALIZACIONES"}, "name": {Text: "Canalizaciones"},
	}}
	if _, err := svc.CreateV2(context.Background(), domain.KindFamily, valid); !errors.Is(err, ErrCatalogWriterUnavailable) {
		t.Fatalf("first CreateV2 error = %v, want ErrCatalogWriterUnavailable", err)
	}
	if attempts != 1 {
		t.Fatalf("repoV2.Insert attempts = %d, want 1", attempts)
	}

	// A second write, backed by a repository call that WOULD succeed, must
	// still be rejected without ever reaching the repository.
	if _, err := svc.CreateV2(context.Background(), domain.KindFamily, valid); !errors.Is(err, ErrCatalogWriterUnavailable) {
		t.Fatalf("second CreateV2 error = %v, want ErrCatalogWriterUnavailable", err)
	}
	if attempts != 1 {
		t.Fatalf("repoV2.Insert attempts = %d after latch, want still 1 (no retry)", attempts)
	}
	if _, version := authority.Current(); version != 1 {
		t.Fatalf("authority version = %d, want 1 (nothing ever published)", version)
	}

	svc.ResetCatalogWriterAvailability()
	repoV2.insertFn = func(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
		rec.ID = 9
		return domain.NewCatalogWriteResult(&rec, baseSnapshot()), nil
	}
	got, err := svc.CreateV2(context.Background(), domain.KindFamily, valid)
	if err != nil {
		t.Fatalf("CreateV2 after reset returned error: %v", err)
	}
	if got.ID != 9 {
		t.Fatalf("CreateV2 after reset returned ID %d, want 9", got.ID)
	}
	if _, version := authority.Current(); version != 2 {
		t.Fatalf("authority version = %d after reset+write, want 2", version)
	}
}

// TestServiceV2PublishesExactlyOnceAfterRepositoryCommit proves publication
// order: the authority is still at its pre-call version WHILE repoV2.Update
// runs, and only advances once, after the call returns successfully —
// never before, never twice.
func TestServiceV2PublishesExactlyOnceAfterRepositoryCommit(t *testing.T) {
	current := familyRecord(5, "CONDUCTORES", "Conductores", true)
	var versionDuringCall uint64
	authority := domain.NewCatalogAuthority(baseSnapshot())
	repoV2 := &fakeCatalogAdminRepositoryV2{
		updateFn: func(ctx context.Context, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogWriteResult, error) {
			_, versionDuringCall = authority.Current()
			return domain.NewCatalogWriteResult(&rec, baseSnapshot()), nil
		},
	}
	repo := &fakeCatalogAdminRepository{getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
		return current, nil
	}}
	svc := NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority).WithCatalogAdminRepositoryV2(repoV2)

	if _, err := svc.UpdateRevision(context.Background(), domain.KindFamily, current, 3); err != nil {
		t.Fatalf("UpdateRevision returned error: %v", err)
	}
	if versionDuringCall != 1 {
		t.Fatalf("authority version during repoV2.Update = %d, want 1 (not published yet)", versionDuringCall)
	}
	if _, version := authority.Current(); version != 2 {
		t.Fatalf("authority version after commit = %d, want 2 (published exactly once)", version)
	}
}

// TestUnavailableWriterLatchAlsoBlocksProductionCreateEntrypoint proves the
// writer-unavailable latch (design "Catalog transaction and coherent
// publication") also blocks Create — the actual production-routed V2 entry
// point since 4D — not only its additive CreateV2 sibling: both share the
// same insertLocked/prepareV2Write code, so this closes the real coverage
// gap (4E) without inventing any new indeterminate-commit concept for the
// legacy port, which has no channel to express one (see apply-progress.md).
func TestUnavailableWriterLatchAlsoBlocksProductionCreateEntrypoint(t *testing.T) {
	repoV2 := &fakeCatalogAdminRepositoryV2{
		insertFn: func(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
			return domain.CatalogWriteResult{}, fmt.Errorf("commit outcome unknown: %w", ErrCatalogWriteIndeterminate)
		},
	}
	authority := domain.NewCatalogAuthority(baseSnapshot())
	svc := NewServiceWithCatalogAuthority(&fakeCatalogAdminRepository{}, domain.NewCatalogRegistry(), authority).WithCatalogAdminRepositoryV2(repoV2)
	valid := domain.CatalogRecord{Active: true, Values: map[string]domain.CatalogValue{
		"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}}, "code": {Text: "CANALIZACIONES"}, "name": {Text: "Canalizaciones"},
	}}
	if _, err := svc.Create(context.Background(), domain.KindFamily, valid); !errors.Is(err, ErrCatalogWriterUnavailable) {
		t.Fatalf("first Create error = %v, want ErrCatalogWriterUnavailable", err)
	}
	if _, err := svc.Create(context.Background(), domain.KindFamily, valid); !errors.Is(err, ErrCatalogWriterUnavailable) {
		t.Fatalf("second Create error = %v, want ErrCatalogWriterUnavailable (latched)", err)
	}
	if _, version := authority.Current(); version != 1 {
		t.Fatalf("authority version = %d, want 1 (nothing ever published)", version)
	}
}

func family(code, name string, active bool) domain.ResourceFamily {
	return domain.ResourceFamily{ClassCode: "MATERIAL", Code: code, Name: name, Active: active}
}

func familyRecord(id int64, code, name string, active bool) domain.CatalogRecord {
	return domain.CatalogRecord{ID: id, Kind: domain.KindFamily, Active: active, Values: map[string]domain.CatalogValue{
		"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}}, "code": {Text: code}, "name": {Text: name},
	}}
}

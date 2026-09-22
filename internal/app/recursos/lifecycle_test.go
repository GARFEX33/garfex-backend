package recursos

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

func TestServiceReactivateRejectsUnavailableCatalogBeforeRepositoryTransition(t *testing.T) {
	resource := domain.Resource{
		ID: 8, ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M",
		IdentityKey: "v1|resource", Active: false,
		Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE")},
	}
	repo := &fakeLifecycleRepo{byID: resource}
	service := NewService(repo, domain.SeedResourceCatalog())

	result, err := service.Reactivate(context.Background(), resource.ID)
	if !errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("Reactivate() error = %v, want catalog validation error", err)
	}
	if result.Resource.Active || repo.reactivateCalls != 0 {
		t.Fatalf("failed Reactivate() = %+v with %d repository transitions, want inactive and zero", result, repo.reactivateCalls)
	}
}

type fakeLifecycleRepo struct {
	fakeRepo
	byID            domain.Resource
	reactivateCalls int
}

func (f *fakeLifecycleRepo) GetByID(context.Context, int64) (domain.Resource, error) {
	return f.byID, nil
}

func (f *fakeLifecycleRepo) Reactivate(context.Context, int64, string) (domain.LifecycleResult, error) {
	f.reactivateCalls++
	return domain.LifecycleResult{}, nil
}

// fullReactivationResource builds via domain.NewResource so IdentityKey is
// the real computed identity-v1, not a hand-typed guess.
func fullReactivationResource(active bool) domain.Resource {
	resource, err := domain.NewResource(domain.SeedResourceCatalog(), domain.ResourceScope{
		ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE",
	}, "M", []domain.ResourceAttributeValue{
		domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"),
		domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V"),
	})
	if err != nil {
		panic(err)
	}
	resource.ID = 8
	resource.Active = active
	return resource
}

// fakeRevisionRepo is a fake domain.ResourceRepositoryV2, additive to
// fakeRepo/fakeLifecycleRepo (one value satisfies both interfaces, as in
// production).
type fakeRevisionRepo struct {
	fakeRepo
	byID    domain.Resource
	byIDErr error

	deactivateCalls       int
	gotDeactivateID       int64
	gotDeactivateRevision uint64
	deactivateResult      domain.Resource
	deactivateErr         error

	reactivateCalls       int
	gotReactivateID       int64
	gotReactivateIdentity string
	gotReactivateRevision uint64
	reactivateResult      domain.Resource
	reactivateErr         error

	updateCalls       int
	gotUpdateResource domain.Resource
	gotUpdateRevision uint64
	updateResult      domain.Resource
	updateErr         error
}

func (f *fakeRevisionRepo) GetByID(context.Context, int64) (domain.Resource, error) {
	return f.byID, f.byIDErr
}

func (f *fakeRevisionRepo) DeactivateRevision(_ context.Context, id int64, expectedRevision uint64) (domain.Resource, error) {
	f.deactivateCalls++
	f.gotDeactivateID, f.gotDeactivateRevision = id, expectedRevision
	return f.deactivateResult, f.deactivateErr
}

func (f *fakeRevisionRepo) ReactivateRevision(_ context.Context, id int64, identityKey string, expectedRevision uint64) (domain.Resource, error) {
	f.reactivateCalls++
	f.gotReactivateID, f.gotReactivateIdentity, f.gotReactivateRevision = id, identityKey, expectedRevision
	return f.reactivateResult, f.reactivateErr
}

func (f *fakeRevisionRepo) UpdateRevision(_ context.Context, resource domain.Resource, expectedRevision uint64) (domain.Resource, error) {
	f.updateCalls++
	f.gotUpdateResource, f.gotUpdateRevision = resource, expectedRevision
	return f.updateResult, f.updateErr
}

func TestServiceDeactivateRevisionRejectsInvalidArgumentsWithoutRepositoryCall(t *testing.T) {
	cases := []struct {
		name     string
		id       int64
		revision uint64
	}{
		{"zero id", 0, 1},
		{"zero revision", 8, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRevisionRepo{}
			_, err := NewService(repo, domain.SeedResourceCatalog()).DeactivateRevision(context.Background(), tt.id, tt.revision)
			if !errors.Is(err, ErrInvalidArgument) || repo.deactivateCalls != 0 {
				t.Fatalf("DeactivateRevision() error = %v, calls = %d, want ErrInvalidArgument and zero calls", err, repo.deactivateCalls)
			}
		})
	}
}

func TestServiceDeactivateRevisionAppliesCASAndClassifiesResult(t *testing.T) {
	generic := errors.New("connection lost")
	cases := []struct {
		name        string
		result      domain.Resource
		repoErr     error
		wantErr     error
		wantChanged bool
	}{
		{name: "real transition bumps revision", result: domain.Resource{ID: 8, Revision: 2, Active: false}, wantChanged: true},
		{name: "idempotent no-op keeps revision", result: domain.Resource{ID: 8, Revision: 1, Active: false}},
		{name: "absent row", repoErr: domain.ErrResourceNotFound, wantErr: domain.ErrResourceNotFound},
		{name: "stale revision", repoErr: domain.ErrResourceRevisionConflict, wantErr: domain.ErrResourceRevisionConflict},
		{name: "wraps unrelated error", repoErr: generic, wantErr: generic},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRevisionRepo{deactivateResult: tt.result, deactivateErr: tt.repoErr}
			result, err := NewService(repo, domain.SeedResourceCatalog()).DeactivateRevision(context.Background(), 8, 1)
			if !errors.Is(err, tt.wantErr) || repo.gotDeactivateID != 8 || repo.gotDeactivateRevision != 1 {
				t.Fatalf("DeactivateRevision() = %+v, %v, want error %v; repo called with id=%d rev=%d", result, err, tt.wantErr, repo.gotDeactivateID, repo.gotDeactivateRevision)
			}
			if tt.wantErr == nil && result.Changed != tt.wantChanged {
				t.Fatalf("DeactivateRevision() Changed = %v, want %v", result.Changed, tt.wantChanged)
			}
		})
	}
}

func TestServiceReactivateRevisionRejectsUnavailableCatalogBeforeRepositoryTransition(t *testing.T) {
	repo := &fakeRevisionRepo{byID: domain.Resource{
		ID: 8, ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M",
		IdentityKey: "v1|resource", Active: false,
		Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE")},
	}}
	result, err := NewService(repo, domain.SeedResourceCatalog()).ReactivateRevision(context.Background(), 8, 1)
	if !errors.Is(err, domain.ErrResourceValidation) || repo.reactivateCalls != 0 {
		t.Fatalf("ReactivateRevision() = %+v, %v, calls = %d; want validation error before any repository transition", result, err, repo.reactivateCalls)
	}
}

// TestServiceReactivateRevisionDistinguishesIdentityMismatchFromRepositoryConflict
// is the TRIANGULATE evidence for "identity mismatch classified distinctly
// from other reactivation failure" — the former is caught before any
// repository call, the latter requires one.
func TestServiceReactivateRevisionDistinguishesIdentityMismatchFromRepositoryConflict(t *testing.T) {
	t.Run("identity mismatch never reaches the repository", func(t *testing.T) {
		resource := fullReactivationResource(false)
		resource.IdentityKey = "v1|stale-identity-no-longer-matches-catalog"
		repo := &fakeRevisionRepo{byID: resource}
		_, err := NewService(repo, domain.SeedResourceCatalog()).ReactivateRevision(context.Background(), resource.ID, 3)
		if !errors.Is(err, domain.ErrResourceIntegrity) || repo.reactivateCalls != 0 {
			t.Fatalf("ReactivateRevision() error = %v, calls = %d; want ErrResourceIntegrity with zero repository calls", err, repo.reactivateCalls)
		}
	})
	t.Run("stale revision is a distinct repository-classified conflict", func(t *testing.T) {
		resource := fullReactivationResource(false)
		repo := &fakeRevisionRepo{byID: resource, reactivateErr: domain.ErrResourceRevisionConflict}
		_, err := NewService(repo, domain.SeedResourceCatalog()).ReactivateRevision(context.Background(), resource.ID, 3)
		if !errors.Is(err, domain.ErrResourceRevisionConflict) || errors.Is(err, domain.ErrResourceIntegrity) || repo.reactivateCalls != 1 {
			t.Fatalf("ReactivateRevision() error = %v, calls = %d; want distinct ErrResourceRevisionConflict after one repository call", err, repo.reactivateCalls)
		}
		if repo.gotReactivateID != resource.ID || repo.gotReactivateIdentity != resource.IdentityKey || repo.gotReactivateRevision != 3 {
			t.Fatalf("repository called with id=%d identity=%q revision=%d, want %d/%q/3", repo.gotReactivateID, repo.gotReactivateIdentity, repo.gotReactivateRevision, resource.ID, resource.IdentityKey)
		}
	})
}

func TestServiceReactivateRevisionAppliesCASAndReportsChanged(t *testing.T) {
	resource := fullReactivationResource(false)
	repo := &fakeRevisionRepo{byID: resource, reactivateResult: domain.Resource{ID: resource.ID, IdentityKey: resource.IdentityKey, Active: true, Revision: 4}}
	result, err := NewService(repo, domain.SeedResourceCatalog()).ReactivateRevision(context.Background(), resource.ID, 3)
	if err != nil || !result.Changed || !result.Resource.Active || result.Resource.Revision != 4 {
		t.Fatalf("ReactivateRevision() = %+v, %v; want changed active result at revision 4", result, err)
	}

	repo = &fakeRevisionRepo{byID: fullReactivationResource(true)}
	result, err = NewService(repo, domain.SeedResourceCatalog()).ReactivateRevision(context.Background(), resource.ID, 3)
	if err != nil || !result.Resource.Active || repo.reactivateCalls != 0 {
		t.Fatalf("ReactivateRevision() on already-active resource = %+v, %v, calls = %d; want no-op", result, err, repo.reactivateCalls)
	}
}

// validUpdateCommand mirrors fullReactivationResource's scope/attributes so
// catalog authority validation succeeds before any repository call.
func validUpdateCommand(id int64) domain.UpdateCommand {
	return domain.UpdateCommand{
		ID:          id,
		Scope:       domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"},
		NaturalUnit: "M",
		Attributes: []domain.ResourceAttributeValue{
			domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"),
			domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V"),
		},
	}
}

func TestServiceUpdateRevisionRejectsInvalidArgumentsWithoutRepositoryCall(t *testing.T) {
	cases := []struct {
		name     string
		id       int64
		revision uint64
	}{
		{"zero id", 0, 1},
		{"zero revision", 8, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRevisionRepo{}
			_, err := NewService(repo, domain.SeedResourceCatalog()).UpdateRevision(context.Background(), validUpdateCommand(tt.id), tt.revision)
			if !errors.Is(err, ErrInvalidArgument) || repo.updateCalls != 0 {
				t.Fatalf("UpdateRevision() error = %v, calls = %d, want ErrInvalidArgument and zero calls", err, repo.updateCalls)
			}
		})
	}
}

// catalog authority validation runs before any repository call.
func TestServiceUpdateRevisionRejectsUnavailableCatalogBeforeRepositoryTransition(t *testing.T) {
	command := domain.UpdateCommand{ID: 8, Scope: domain.ResourceScope{ClassCode: "NOPE", FamilyCode: "NOPE", TypeCode: "NOPE"}, NaturalUnit: "M"}
	repo := &fakeRevisionRepo{}
	_, err := NewService(repo, domain.SeedResourceCatalog()).UpdateRevision(context.Background(), command, 1)
	if !errors.Is(err, domain.ErrResourceValidation) || repo.updateCalls != 0 {
		t.Fatalf("UpdateRevision() error = %v, calls = %d; want catalog validation error before any repository call", err, repo.updateCalls)
	}
}

func TestServiceUpdateRevisionAppliesCASAndClassifiesResult(t *testing.T) {
	generic := errors.New("connection lost")
	cases := []struct {
		name    string
		result  domain.Resource
		repoErr error
		wantErr error
	}{
		{name: "successful CAS update returns replaced resource", result: domain.Resource{ID: 8, Revision: 2, Active: true}},
		{name: "stale revision", repoErr: domain.ErrResourceRevisionConflict, wantErr: domain.ErrResourceRevisionConflict},
		{name: "absent row", repoErr: domain.ErrResourceNotFound, wantErr: domain.ErrResourceNotFound},
		{name: "wraps unrelated error", repoErr: generic, wantErr: generic},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRevisionRepo{updateResult: tt.result, updateErr: tt.repoErr}
			command := validUpdateCommand(8)
			result, err := NewService(repo, domain.SeedResourceCatalog()).UpdateRevision(context.Background(), command, 3)
			if !errors.Is(err, tt.wantErr) || repo.updateCalls != 1 || repo.gotUpdateRevision != 3 || repo.gotUpdateResource.ID != 8 {
				t.Fatalf("UpdateRevision() = %+v, %v, want error %v; repo called %d times with id=%d rev=%d", result, err, tt.wantErr, repo.updateCalls, repo.gotUpdateResource.ID, repo.gotUpdateRevision)
			}
			if tt.wantErr == nil && !reflect.DeepEqual(result, tt.result) {
				t.Fatalf("UpdateRevision() = %+v, want repository result %+v", result, tt.result)
			}
		})
	}
}

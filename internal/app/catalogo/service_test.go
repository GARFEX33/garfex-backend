// Package catalogo provides the internal/app/catalogo.Service — the
// catalog-structure administration use cases (design §4, tasks 5.1/5.2).
//
// RED (task 5.1): Service/NewService do not exist yet — every reference
// below fails to compile until service.go's GREEN lands.
package catalogo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// --- fake domain.CatalogAdminRepository -----------------------------------

// fakeCatalogAdminRepository is a fixture-driven test double for
// domain.CatalogAdminRepository (testing-data-boundary.md — mocks/fakes
// isolate unit tests; no real Postgres involved in this PR). Each method
// delegates to an optional func field when set, otherwise returns a benign
// zero value; call counters let tests assert a persist method was (or was
// NOT) invoked.
type fakeCatalogAdminRepository struct {
	mu sync.Mutex

	listFn                  func(ctx context.Context, kind domain.CatalogKindCode, f domain.CatalogFilter) ([]domain.CatalogRecord, error)
	getFn                   func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error)
	insertFn                func(ctx context.Context, rec domain.CatalogRecord) (int64, error)
	updateFn                func(ctx context.Context, rec domain.CatalogRecord) error
	setActiveFn             func(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool) error
	deleteFn                func(ctx context.Context, kind domain.CatalogKindCode, id int64) error
	dependentsFn            func(ctx context.Context, kind domain.CatalogKindCode, id int64) ([]domain.CatalogDependency, error)
	referencedByResourcesFn func(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error)

	listCalls, getCalls, insertCalls, updateCalls, setActiveCalls, deleteCalls, dependentsCalls, referencedByResourcesCalls int
}

func (f *fakeCatalogAdminRepository) List(ctx context.Context, kind domain.CatalogKindCode, filter domain.CatalogFilter) ([]domain.CatalogRecord, error) {
	f.mu.Lock()
	f.listCalls++
	f.mu.Unlock()
	if f.listFn != nil {
		return f.listFn(ctx, kind, filter)
	}
	return nil, nil
}

func (f *fakeCatalogAdminRepository) Get(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
	f.mu.Lock()
	f.getCalls++
	f.mu.Unlock()
	if f.getFn != nil {
		return f.getFn(ctx, kind, id)
	}
	return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
}

func (f *fakeCatalogAdminRepository) Insert(ctx context.Context, rec domain.CatalogRecord) (int64, error) {
	f.mu.Lock()
	f.insertCalls++
	f.mu.Unlock()
	if f.insertFn != nil {
		return f.insertFn(ctx, rec)
	}
	return 1, nil
}

func (f *fakeCatalogAdminRepository) Update(ctx context.Context, rec domain.CatalogRecord) error {
	f.mu.Lock()
	f.updateCalls++
	f.mu.Unlock()
	if f.updateFn != nil {
		return f.updateFn(ctx, rec)
	}
	return nil
}

func (f *fakeCatalogAdminRepository) SetActive(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool) error {
	f.mu.Lock()
	f.setActiveCalls++
	f.mu.Unlock()
	if f.setActiveFn != nil {
		return f.setActiveFn(ctx, kind, id, active)
	}
	return nil
}

func (f *fakeCatalogAdminRepository) Delete(ctx context.Context, kind domain.CatalogKindCode, id int64) error {
	f.mu.Lock()
	f.deleteCalls++
	f.mu.Unlock()
	if f.deleteFn != nil {
		return f.deleteFn(ctx, kind, id)
	}
	return nil
}

func (f *fakeCatalogAdminRepository) Dependents(ctx context.Context, kind domain.CatalogKindCode, id int64) ([]domain.CatalogDependency, error) {
	f.mu.Lock()
	f.dependentsCalls++
	f.mu.Unlock()
	if f.dependentsFn != nil {
		return f.dependentsFn(ctx, kind, id)
	}
	return nil, nil
}

func (f *fakeCatalogAdminRepository) ReferencedByResources(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
	f.mu.Lock()
	f.referencedByResourcesCalls++
	f.mu.Unlock()
	if f.referencedByResourcesFn != nil {
		return f.referencedByResourcesFn(ctx, kind, id)
	}
	return false, nil
}

// --- fake domain.CatalogAdminRepositoryV2 (4C additive write port) --------

// fakeCatalogAdminRepositoryV2 is a fixture-driven test double for
// domain.CatalogAdminRepositoryV2, adapted alongside 4C's service methods —
// same optional-func-field/call-counter convention as
// fakeCatalogAdminRepository above.
type fakeCatalogAdminRepositoryV2 struct {
	mu sync.Mutex

	insertFn    func(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error)
	updateFn    func(ctx context.Context, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogWriteResult, error)
	setActiveFn func(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool, expectedRevision uint64) (domain.CatalogWriteResult, error)
	deleteFn    func(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) (domain.CatalogWriteResult, error)

	insertCalls, updateCalls, setActiveCalls, deleteCalls int
}

var _ domain.CatalogAdminRepositoryV2 = (*fakeCatalogAdminRepositoryV2)(nil)

func (f *fakeCatalogAdminRepositoryV2) Insert(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
	f.mu.Lock()
	f.insertCalls++
	f.mu.Unlock()
	if f.insertFn != nil {
		return f.insertFn(ctx, rec)
	}
	return domain.NewCatalogWriteResult(&rec, domain.ResourceCatalog{}), nil
}

func (f *fakeCatalogAdminRepositoryV2) Update(ctx context.Context, rec domain.CatalogRecord, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	f.mu.Lock()
	f.updateCalls++
	f.mu.Unlock()
	if f.updateFn != nil {
		return f.updateFn(ctx, rec, expectedRevision)
	}
	return domain.NewCatalogWriteResult(&rec, domain.ResourceCatalog{}), nil
}

func (f *fakeCatalogAdminRepositoryV2) SetActive(ctx context.Context, kind domain.CatalogKindCode, id int64, active bool, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	f.mu.Lock()
	f.setActiveCalls++
	f.mu.Unlock()
	if f.setActiveFn != nil {
		return f.setActiveFn(ctx, kind, id, active, expectedRevision)
	}
	rec := domain.CatalogRecord{Kind: kind, ID: id, Active: active, Revision: expectedRevision + 1}
	return domain.NewCatalogWriteResult(&rec, domain.ResourceCatalog{}), nil
}

func (f *fakeCatalogAdminRepositoryV2) Delete(ctx context.Context, kind domain.CatalogKindCode, id int64, expectedRevision uint64) (domain.CatalogWriteResult, error) {
	f.mu.Lock()
	f.deleteCalls++
	f.mu.Unlock()
	if f.deleteFn != nil {
		return f.deleteFn(ctx, kind, id, expectedRevision)
	}
	return domain.NewCatalogWriteResult(nil, domain.ResourceCatalog{}), nil
}

// --- fixtures --------------------------------------------------------------

// baseSnapshot mirrors catalog_mutation_test.go's baseMutationCatalog shape:
// one active Clase, one active Familia under it — enough to exercise
// referential validation without depending on domain.SeedResourceCatalog's
// larger fixture (avoids any accidental coupling to that literal's own
// landmine history).
func baseSnapshot() domain.ResourceCatalog {
	return domain.ResourceCatalog{
		Classes: []domain.ResourceClass{
			{Code: "MATERIAL", Name: "Material", Plural: "Materiales", Slug: "materiales", Order: 1, Active: true},
		},
		Families: []domain.ResourceFamily{
			{ClassCode: "MATERIAL", Code: "CONDUCTORES", Name: "Conductores", Active: true},
		},
		Units: []domain.UnitDefinition{
			{Code: "M", Name: "Metro", Symbol: "M", Dimension: "LENGTH", Active: true},
		},
	}
}

func newTestService(repo domain.CatalogAdminRepository) *Service {
	return NewService(repo, domain.NewCatalogRegistry(), baseSnapshot())
}

// --- Kinds/List/Get/Dependencies (read passthrough) ------------------------

func TestServiceKindsReturnsRegistry(t *testing.T) {
	svc := newTestService(&fakeCatalogAdminRepository{})
	got := svc.Kinds()
	want := domain.NewCatalogRegistry().Kinds()
	if len(got) != len(want) {
		t.Fatalf("Kinds() returned %d kinds, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Code != want[i].Code {
			t.Fatalf("Kinds()[%d].Code = %q, want %q", i, got[i].Code, want[i].Code)
		}
	}
}

// TestServiceListReadsFromRepository proves List is repository-backed, not
// snapshot-backed: domain.ResourceCatalog carries no ID field (design D10),
// so only the repository can return the ID-bearing records a caller needs
// for a later Update/Deactivate/Delete call — the in-memory snapshot exists
// solely to run ApplyCatalogMutation/Validate, never to serve reads.
func TestServiceListReadsFromRepository(t *testing.T) {
	want := []domain.CatalogRecord{{Kind: domain.KindClass, ID: 7, Active: true}}
	repo := &fakeCatalogAdminRepository{
		listFn: func(ctx context.Context, kind domain.CatalogKindCode, f domain.CatalogFilter) ([]domain.CatalogRecord, error) {
			if kind != domain.KindClass {
				t.Fatalf("List called with kind %q, want %q", kind, domain.KindClass)
			}
			return want, nil
		},
	}
	svc := newTestService(repo)

	got, err := svc.List(context.Background(), domain.KindClass, domain.CatalogFilter{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("List returned %+v, want %+v", got, want)
	}
	if repo.listCalls != 1 {
		t.Fatalf("repo.List called %d times, want 1", repo.listCalls)
	}
}

func TestServiceGetReadsFromRepository(t *testing.T) {
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return domain.CatalogRecord{Kind: kind, ID: id, Active: true}, nil
		},
	}
	svc := newTestService(repo)

	got, err := svc.Get(context.Background(), domain.KindClass, 3)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.ID != 3 {
		t.Fatalf("Get returned ID %d, want 3", got.ID)
	}
}

func TestServiceGetRejectsZeroID(t *testing.T) {
	svc := newTestService(&fakeCatalogAdminRepository{})
	if _, err := svc.Get(context.Background(), domain.KindClass, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Get(id=0) error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceGetPropagatesNotFound(t *testing.T) {
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return domain.CatalogRecord{}, domain.ErrCatalogRecordNotFound
		},
	}
	svc := newTestService(repo)
	if _, err := svc.Get(context.Background(), domain.KindClass, 99); !errors.Is(err, domain.ErrCatalogRecordNotFound) {
		t.Fatalf("Get error = %v, want ErrCatalogRecordNotFound", err)
	}
}

func TestServiceDependenciesReadsFromRepository(t *testing.T) {
	want := []domain.CatalogDependency{{Kind: domain.KindFamily, Count: 2, Blocking: true}}
	repo := &fakeCatalogAdminRepository{
		dependentsFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) ([]domain.CatalogDependency, error) {
			return want, nil
		},
	}
	svc := newTestService(repo)

	got, err := svc.Dependencies(context.Background(), domain.KindClass, 1)
	if err != nil {
		t.Fatalf("Dependencies returned error: %v", err)
	}
	if len(got) != 1 || got[0].Count != 2 {
		t.Fatalf("Dependencies returned %+v, want %+v", got, want)
	}
}

// TestServiceReferencedByResourcesReadsFromRepository proves
// ReferencedByResources (task 7.2/7.1's TUI-facing referencedness signal) is
// a direct passthrough to the repository probe of the same name.
func TestServiceReferencedByResourcesReadsFromRepository(t *testing.T) {
	repo := &fakeCatalogAdminRepository{
		referencedByResourcesFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
			return true, nil
		},
	}
	svc := newTestService(repo)

	got, err := svc.ReferencedByResources(context.Background(), domain.KindClass, 1)
	if err != nil {
		t.Fatalf("ReferencedByResources returned error: %v", err)
	}
	if !got {
		t.Fatal("ReferencedByResources = false, want true")
	}
	if repo.referencedByResourcesCalls != 1 {
		t.Fatalf("repo.ReferencedByResources called %d times, want 1", repo.referencedByResourcesCalls)
	}
}

// --- Create: validate-before-persist (design D9) ---------------------------

// TestServiceCreateRejectsInvalidMutationWithoutPersisting is the single
// most load-bearing behavior this service exists to guarantee (apply
// prompt's own framing): a mutation that would make the WHOLE catalog
// structurally invalid must be rejected, with NOTHING persisted and the
// service's own in-memory snapshot left completely unchanged.
func TestServiceCreateRejectsInvalidMutationWithoutPersisting(t *testing.T) {
	repo := &fakeCatalogAdminRepository{}
	svc := newTestService(repo)
	originalSnapshot := svc.snapshot

	// A Familia referencing a Clase the catalog does not define — caught by
	// ResourceCatalog.Validate's validateFamilies (ErrResourceReference).
	invalid := domain.CatalogRecord{
		Active: true,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "NO_EXISTE"}},
			"code":  {Text: "NUEVA"},
			"name":  {Text: "Nueva Familia"},
		},
	}

	_, err := svc.Create(context.Background(), domain.KindFamily, invalid)
	if err == nil {
		t.Fatal("Create returned nil error for a structurally invalid mutation")
	}
	if !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("Create error = %v, want it to wrap domain.ErrResourceReference", err)
	}
	if repo.insertCalls != 0 {
		t.Fatalf("repo.Insert called %d times, want 0 (nothing should persist on rejection)", repo.insertCalls)
	}
	if len(svc.snapshot.Families) != len(originalSnapshot.Families) {
		t.Fatalf("service snapshot Families changed after a rejected mutation: got %d, want %d", len(svc.snapshot.Families), len(originalSnapshot.Families))
	}
	if !reflectEqualCatalog(svc.snapshot, originalSnapshot) {
		t.Fatal("service snapshot mutated after a rejected mutation")
	}
}

// TestServiceCreateRejectsDuplicateClassCodeWithoutPersisting exercises the
// same guarantee via a different Validate() invariant (validateClasses'
// duplicate-code check), proving the rejection path is not special-cased to
// one invariant.
func TestServiceCreateRejectsDuplicateClassCodeWithoutPersisting(t *testing.T) {
	repo := &fakeCatalogAdminRepository{}
	svc := newTestService(repo)

	dup := domain.CatalogRecord{
		Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, // already seeded in baseSnapshot()
			"name": {Text: "Material Duplicado"}, "plural": {Text: "Materiales"}, "slug": {Text: "material-2"},
		},
	}
	_, err := svc.Create(context.Background(), domain.KindClass, dup)
	if !errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("Create error = %v, want it to wrap domain.ErrResourceValidation", err)
	}
	// 5A: the candidate catalog's own Validate() failure is additionally
	// classified ErrInvalidCatalog (design "Internal Core outcomes and
	// mapping") — never a collapse, both sentinels are reachable via
	// errors.Is on the same returned error.
	if !errors.Is(err, domain.ErrInvalidCatalog) {
		t.Fatalf("Create error = %v, want it to also wrap domain.ErrInvalidCatalog", err)
	}
	if repo.insertCalls != 0 {
		t.Fatalf("repo.Insert called %d times, want 0", repo.insertCalls)
	}
}

// TestLifecycleInvalidHardDeleteOfActiveTargetIsDistinctFromInvalidCatalog is
// the RED/TRIANGULATE evidence that a hard-delete-of-active rejection is
// ErrInvalidLifecycle, distinct from the candidate-catalog-invalid
// ErrInvalidCatalog outcome a later Validate() failure would produce.
func TestLifecycleInvalidHardDeleteOfActiveTargetIsDistinctFromInvalidCatalog(t *testing.T) {
	repo := &fakeCatalogAdminRepository{getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
		return familyRecord(5, "CONDUCTORES", "Conductores", true), nil
	}}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), domain.KindFamily, 5)
	if !errors.Is(err, domain.ErrInvalidLifecycle) {
		t.Fatalf("Delete() error = %v, want ErrInvalidLifecycle", err)
	}
	if errors.Is(err, domain.ErrInvalidCatalog) || errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("Delete() error = %v, must not also match ErrInvalidCatalog/ErrResourceValidation", err)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("repo.Delete called %d times, want 0", repo.deleteCalls)
	}
}

// TestServiceCreateCommitsSnapshotOnlyAfterPersist proves the success path:
// the snapshot is updated to the validated value, and only after Insert
// succeeds — the returned record carries the repository-assigned ID.
func TestServiceCreateCommitsSnapshotOnlyAfterPersist(t *testing.T) {
	repo := &fakeCatalogAdminRepository{
		insertFn: func(ctx context.Context, rec domain.CatalogRecord) (int64, error) { return 42, nil },
	}
	svc := newTestService(repo)

	valid := domain.CatalogRecord{
		Active: true,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}},
			"code":  {Text: "CANALIZACIONES"},
			"name":  {Text: "Canalizaciones"},
		},
	}
	got, err := svc.Create(context.Background(), domain.KindFamily, valid)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.ID != 42 {
		t.Fatalf("Create returned ID %d, want 42 (repository-assigned)", got.ID)
	}
	if repo.insertCalls != 1 {
		t.Fatalf("repo.Insert called %d times, want 1", repo.insertCalls)
	}
	if len(svc.snapshot.Families) != 2 {
		t.Fatalf("service snapshot Families = %d, want 2 (committed)", len(svc.snapshot.Families))
	}
}

// TestServiceCreateCanonicalizesCodeBeforePersisting proves the write path
// never lets a non-canonical "código" value reach the repository: Validate()
// already canonicalizes for comparison (resource_catalog_validate.go), but
// without this wiring the repository would still receive and persist the
// caller's raw, lowercase text.
func TestServiceCreateCanonicalizesCodeBeforePersisting(t *testing.T) {
	var persisted domain.CatalogRecord
	repo := &fakeCatalogAdminRepository{
		insertFn: func(ctx context.Context, rec domain.CatalogRecord) (int64, error) {
			persisted = rec
			return 42, nil
		},
	}
	svc := newTestService(repo)

	lowercase := domain.CatalogRecord{
		Active: true,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}},
			"code":  {Text: "  canalizaciones  "},
			"name":  {Text: "Canalizaciones"},
		},
	}
	got, err := svc.Create(context.Background(), domain.KindFamily, lowercase)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if persisted.Values["code"].Text != "CANALIZACIONES" {
		t.Fatalf("repository received code %q, want canonical CANALIZACIONES", persisted.Values["code"].Text)
	}
	if got.Values["code"].Text != "CANALIZACIONES" {
		t.Fatalf("Create returned code %q, want canonical CANALIZACIONES", got.Values["code"].Text)
	}
	if got.Values["name"].Text != "Canalizaciones" {
		t.Fatalf("non-código field must not be canonicalized: got %q", got.Values["name"].Text)
	}
}

// TestServiceCreatePropagatesDuplicateFromRepository confirms PR4's
// domain.ErrCatalogDuplicate (pgx 23505) mapping surfaces through the
// service unchanged (errors.Is), rather than being re-implemented or lost.
func TestServiceCreatePropagatesDuplicateFromRepository(t *testing.T) {
	repo := &fakeCatalogAdminRepository{
		insertFn: func(ctx context.Context, rec domain.CatalogRecord) (int64, error) {
			return 0, fmt.Errorf("%w: resource_classes_code_key", domain.ErrCatalogDuplicate)
		},
	}
	svc := newTestService(repo)

	valid := domain.CatalogRecord{
		Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "SERVICIO"}, "name": {Text: "Servicio"}, "plural": {Text: "Servicios"}, "slug": {Text: "servicio"},
		},
	}
	_, err := svc.Create(context.Background(), domain.KindClass, valid)
	if !errors.Is(err, domain.ErrCatalogDuplicate) {
		t.Fatalf("Create error = %v, want it to wrap domain.ErrCatalogDuplicate", err)
	}
	// The (already-validated) snapshot must NOT be committed when persist fails.
	if len(svc.snapshot.Classes) != 1 {
		t.Fatalf("service snapshot Classes = %d, want 1 (unchanged after failed persist)", len(svc.snapshot.Classes))
	}
}

// --- Update: validate-before-persist + Código immutability (D11) -----------

func TestServiceUpdateRejectsZeroID(t *testing.T) {
	svc := newTestService(&fakeCatalogAdminRepository{})
	_, err := svc.Update(context.Background(), domain.KindClass, domain.CatalogRecord{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Update(ID=0) error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceUpdateCommitsSnapshotOnPersist(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindClass, ID: 1, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"}, "order": {Int: 1},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	}
	svc := newTestService(repo)

	updated := current
	updated.Values = map[string]domain.CatalogValue{
		"code": {Text: "MATERIAL"}, "name": {Text: "Material (renombrado)"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"}, "order": {Int: 1},
	}
	got, err := svc.Update(context.Background(), domain.KindClass, updated)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got.Values["name"].Text != "Material (renombrado)" {
		t.Fatalf("Update returned %+v, want the new name applied", got)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("repo.Update called %d times, want 1", repo.updateCalls)
	}
	if svc.snapshot.Classes[0].Name != "Material (renombrado)" {
		t.Fatalf("service snapshot Classes[0].Name = %q, want committed rename", svc.snapshot.Classes[0].Name)
	}
}

// TestServiceUpdateCanonicalizesCodeBeforePersisting mirrors
// TestServiceCreateCanonicalizesCodeBeforePersisting for the Update path,
// and additionally proves a mere case variant of the current code ("material"
// vs persisted "MATERIAL") is never treated as a código change: the
// immutability guard must never fire, and repo.ReferencedByResources must
// never even be consulted.
func TestServiceUpdateCanonicalizesCodeBeforePersisting(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindClass, ID: 1, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
		},
	}
	var persisted domain.CatalogRecord
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
		updateFn: func(ctx context.Context, rec domain.CatalogRecord) error {
			persisted = rec
			return nil
		},
		referencedByResourcesFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
			t.Fatal("ReferencedByResources must not be consulted for a mere case variant of the same code")
			return false, nil
		},
	}
	svc := newTestService(repo)

	lowercase := current
	lowercase.Values = map[string]domain.CatalogValue{
		"code": {Text: "material"}, "name": {Text: "Material (renombrado)"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
	}
	got, err := svc.Update(context.Background(), domain.KindClass, lowercase)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if persisted.Values["code"].Text != "MATERIAL" {
		t.Fatalf("repository received code %q, want canonical MATERIAL", persisted.Values["code"].Text)
	}
	if got.Values["code"].Text != "MATERIAL" {
		t.Fatalf("Update returned code %q, want canonical MATERIAL", got.Values["code"].Text)
	}
}

// TestServiceUpdateRejectsCodeChangeWhenReferenced proves D11 layer 1: the
// service checks Código immutability BEFORE calling repo.Update at all.
func TestServiceUpdateRejectsCodeChangeWhenReferenced(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindClass, ID: 1, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
		referencedByResourcesFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
			return true, nil
		},
	}
	svc := newTestService(repo)

	changed := current
	changed.Values = map[string]domain.CatalogValue{
		"code": {Text: "MATERIAL_NUEVO"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
	}
	_, err := svc.Update(context.Background(), domain.KindClass, changed)
	if !errors.Is(err, domain.ErrCodeImmutable) {
		t.Fatalf("Update error = %v, want domain.ErrCodeImmutable", err)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("repo.Update called %d times, want 0 (rejected before persist)", repo.updateCalls)
	}
}

// TestServiceUpdateAllowsCodeChangeWhenNotReferenced proves D11 only blocks
// a código change once ≥1 resource references the record.
func TestServiceUpdateAllowsCodeChangeWhenNotReferenced(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindClass, ID: 1, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
		referencedByResourcesFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (bool, error) {
			return false, nil
		},
	}
	svc := newTestService(repo)
	svc.snapshot.Families = nil

	changed := current
	changed.Values = map[string]domain.CatalogValue{
		"code": {Text: "MATERIAL_NUEVO"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
	}
	if _, err := svc.Update(context.Background(), domain.KindClass, changed); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("repo.Update called %d times, want 1", repo.updateCalls)
	}
}

// --- Deactivate/Reactivate/Delete -------------------------------------------

func TestServiceDeactivateRejectsWhenActiveChildrenWouldDangle(t *testing.T) {
	// baseSnapshot's Familia CONDUCTORES is active under Clase MATERIAL —
	// deactivating MATERIAL without also deactivating CONDUCTORES violates
	// validateActiveHierarchy (design Risk#3).
	current := domain.CatalogRecord{
		Kind: domain.KindClass, ID: 1, Active: true,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	}
	svc := newTestService(repo)

	err := svc.Deactivate(context.Background(), domain.KindClass, 1)
	if !errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("Deactivate error = %v, want domain.ErrResourceValidation", err)
	}
	if repo.setActiveCalls != 0 {
		t.Fatalf("repo.SetActive called %d times, want 0 (rejected before persist)", repo.setActiveCalls)
	}
}

func TestServiceDeactivateCommitsOnPersist(t *testing.T) {
	// Deactivating the Familia (a leaf under MATERIAL) is structurally valid.
	current := domain.CatalogRecord{
		Kind: domain.KindFamily, ID: 5, Active: true,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}},
			"code":  {Text: "CONDUCTORES"}, "name": {Text: "Conductores"},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	}
	svc := newTestService(repo)

	if err := svc.Deactivate(context.Background(), domain.KindFamily, 5); err != nil {
		t.Fatalf("Deactivate returned error: %v", err)
	}
	if repo.setActiveCalls != 1 {
		t.Fatalf("repo.SetActive called %d times, want 1", repo.setActiveCalls)
	}
	if svc.snapshot.Families[0].Active {
		t.Fatal("service snapshot Families[0].Active still true after a committed Deactivate")
	}
}

func TestServiceDeactivateRejectsZeroID(t *testing.T) {
	svc := newTestService(&fakeCatalogAdminRepository{})
	if err := svc.Deactivate(context.Background(), domain.KindClass, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Deactivate(id=0) error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceDeactivateOnLifecycleCapableKindCommitsOnPersist(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindAttributeDefinition, ID: 9, Active: true,
		Values: map[string]domain.CatalogValue{"code": {Text: "COLOR"}, "name": {Text: "Color"}, "valueType": {Text: "CONTROLLED_TEXT"}},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	}
	snapshot := baseSnapshot()
	snapshot.Definitions = []domain.AttributeDefinition{{
		Code: "COLOR", Name: "Color", ValueType: domain.ValueTypeControlledText, Active: true,
	}}
	svc := NewService(repo, domain.NewCatalogRegistry(), snapshot)

	if err := svc.Deactivate(context.Background(), domain.KindAttributeDefinition, 9); err != nil {
		t.Fatalf("Deactivate returned error: %v", err)
	}
	if repo.setActiveCalls != 1 {
		t.Fatalf("repo.SetActive called %d times, want 1", repo.setActiveCalls)
	}
	if len(svc.snapshot.Definitions) != 1 || svc.snapshot.Definitions[0].Active {
		t.Fatalf("service snapshot Definitions = %#v, want inactive COLOR definition", svc.snapshot.Definitions)
	}
	published, _ := svc.authority.Current()
	if len(published.Definitions) != 1 || published.Definitions[0].Active {
		t.Fatalf("published Definitions = %#v, want inactive COLOR definition", published.Definitions)
	}
}

func TestServiceReactivateCommitsOnPersist(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindFamily, ID: 5, Active: false,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}},
			"code":  {Text: "CONDUCTORES"}, "name": {Text: "Conductores"},
		},
	}
	svc := newTestService(&fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	})
	svc.snapshot.Families[0].Active = false

	if err := svc.Reactivate(context.Background(), domain.KindFamily, 5); err != nil {
		t.Fatalf("Reactivate returned error: %v", err)
	}
	if !svc.snapshot.Families[0].Active {
		t.Fatal("service snapshot Families[0].Active still false after a committed Reactivate")
	}
}

func TestServiceDeleteRejectsWhenItWouldOrphanChildren(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindClass, ID: 1, Active: false,
		Values: map[string]domain.CatalogValue{
			"code": {Text: "MATERIAL"}, "name": {Text: "Material"}, "plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	}
	svc := newTestService(repo)

	err := svc.Delete(context.Background(), domain.KindClass, 1)
	if !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("Delete error = %v, want domain.ErrResourceReference (orphaned Familia)", err)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("repo.Delete called %d times, want 0 (rejected before persist)", repo.deleteCalls)
	}
}

func TestServiceDeleteCommitsOnPersist(t *testing.T) {
	current := domain.CatalogRecord{
		Kind: domain.KindFamily, ID: 5, Active: false,
		Values: map[string]domain.CatalogValue{
			"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}},
			"code":  {Text: "CONDUCTORES"}, "name": {Text: "Conductores"},
		},
	}
	repo := &fakeCatalogAdminRepository{
		getFn: func(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error) {
			return current, nil
		},
	}
	svc := newTestService(repo)

	if err := svc.Delete(context.Background(), domain.KindFamily, 5); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if repo.deleteCalls != 1 {
		t.Fatalf("repo.Delete called %d times, want 1", repo.deleteCalls)
	}
	if len(svc.snapshot.Families) != 0 {
		t.Fatalf("service snapshot Families = %d, want 0 (committed delete)", len(svc.snapshot.Families))
	}
}

func TestServiceDeleteRejectsZeroID(t *testing.T) {
	svc := newTestService(&fakeCatalogAdminRepository{})
	if err := svc.Delete(context.Background(), domain.KindClass, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Delete(id=0) error = %v, want ErrInvalidArgument", err)
	}
}

func TestServiceDeleteConservativeGuards(t *testing.T) {
	repoErr := errors.New("repository probe failed")
	cases := []struct {
		name           string
		kind           domain.CatalogKindCode
		id             int64
		current        domain.CatalogRecord
		getErr         error
		dependencies   []domain.CatalogDependency
		dependentsErr  error
		referenced     bool
		referencesErr  error
		deleteErr      error
		wantErr        error
		wantDeleteCall int
		wantCalls      []string
	}{
		{
			name:      "active target is rejected before dependency probes",
			kind:      domain.KindFamily,
			id:        5,
			current:   familyRecord(5, "CONDUCTORES", "Conductores", true),
			wantErr:   domain.ErrInvalidLifecycle,
			wantCalls: []string{"Get"},
		},
		{
			name: "inactive non-blocking dependency still blocks",
			kind: domain.KindFamily, id: 5,
			current: familyRecord(5, "CONDUCTORES", "Conductores", false),
			dependencies: []domain.CatalogDependency{{
				Kind: domain.KindPresentationField, Count: 1, Blocking: false,
			}},
			wantErr:   domain.ErrCatalogInUse,
			wantCalls: []string{"Get", "Dependents"},
		},
		{
			name: "historical dependency count still blocks",
			kind: domain.KindFamily, id: 5,
			current: familyRecord(5, "CONDUCTORES", "Conductores", false),
			dependencies: []domain.CatalogDependency{{
				Kind: domain.KindType, Count: 2, Blocking: false,
			}},
			wantErr:   domain.ErrCatalogInUse,
			wantCalls: []string{"Get", "Dependents"},
		},
		{
			name:       "active resource reference blocks",
			kind:       domain.KindFamily,
			id:         5,
			current:    familyRecord(5, "CONDUCTORES", "Conductores", false),
			referenced: true,
			wantErr:    domain.ErrCatalogInUse,
			wantCalls:  []string{"Get", "Dependents", "ReferencedByResources"},
		},
		{
			name:       "inactive resource reference blocks",
			kind:       domain.KindFamily,
			id:         5,
			current:    familyRecord(5, "CONDUCTORES", "Conductores", false),
			referenced: true,
			wantErr:    domain.ErrCatalogInUse,
			wantCalls:  []string{"Get", "Dependents", "ReferencedByResources"},
		},
		{
			name:       "historical resource reference blocks",
			kind:       domain.KindFamily,
			id:         5,
			current:    familyRecord(5, "CONDUCTORES", "Conductores", false),
			referenced: true,
			wantErr:    domain.ErrCatalogInUse,
			wantCalls:  []string{"Get", "Dependents", "ReferencedByResources"},
		},
		{
			name:      "stale repository candidate is rejected privately",
			kind:      domain.KindFamily,
			id:        5,
			current:   familyRecord(5, "STALE", "Stale", false),
			wantErr:   domain.ErrCatalogRecordNotFound,
			wantCalls: []string{"Get", "Dependents", "ReferencedByResources"},
		},
		{
			name: "invalid private candidate is rejected before delete",
			kind: domain.KindClass, id: 1,
			current: domain.CatalogRecord{
				Kind: domain.KindClass, ID: 1, Active: false,
				Values: map[string]domain.CatalogValue{
					"code": {Text: "MATERIAL"}, "name": {Text: "Material"},
					"plural": {Text: "Materiales"}, "slug": {Text: "materiales"},
				},
			},
			wantErr:   domain.ErrResourceReference,
			wantCalls: []string{"Get", "Dependents", "ReferencedByResources"},
		},
		{
			name:      "current state repository failure is invisible",
			kind:      domain.KindFamily,
			id:        5,
			getErr:    repoErr,
			wantErr:   repoErr,
			wantCalls: []string{"Get"},
		},
		{
			name:          "dependency repository failure is invisible",
			kind:          domain.KindFamily,
			id:            5,
			current:       familyRecord(5, "CONDUCTORES", "Conductores", false),
			dependentsErr: repoErr,
			wantErr:       repoErr,
			wantCalls:     []string{"Get", "Dependents"},
		},
		{
			name:          "resource reference repository failure is invisible",
			kind:          domain.KindFamily,
			id:            5,
			current:       familyRecord(5, "CONDUCTORES", "Conductores", false),
			referencesErr: repoErr,
			wantErr:       repoErr,
			wantCalls:     []string{"Get", "Dependents", "ReferencedByResources"},
		},
		{
			name:           "delete repository failure is invisible",
			kind:           domain.KindFamily,
			id:             5,
			current:        familyRecord(5, "CONDUCTORES", "Conductores", false),
			deleteErr:      repoErr,
			wantErr:        repoErr,
			wantDeleteCall: 1,
			wantCalls:      []string{"Get", "Dependents", "ReferencedByResources", "Delete"},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]string, 0, 4)
			repo := &fakeCatalogAdminRepository{
				getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
					calls = append(calls, "Get")
					return tt.current, tt.getErr
				},
				dependentsFn: func(context.Context, domain.CatalogKindCode, int64) ([]domain.CatalogDependency, error) {
					calls = append(calls, "Dependents")
					return tt.dependencies, tt.dependentsErr
				},
				referencedByResourcesFn: func(context.Context, domain.CatalogKindCode, int64) (bool, error) {
					calls = append(calls, "ReferencedByResources")
					return tt.referenced, tt.referencesErr
				},
				deleteFn: func(context.Context, domain.CatalogKindCode, int64) error {
					calls = append(calls, "Delete")
					return tt.deleteErr
				},
			}
			authority := domain.NewCatalogAuthority(baseSnapshot())
			svc := NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority)
			beforeSnapshot := svc.snapshot
			beforeAuthority, beforeVersion := authority.Current()

			err := svc.Delete(context.Background(), tt.kind, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Delete() error = %v, want %v", err, tt.wantErr)
			}
			if repo.deleteCalls != tt.wantDeleteCall {
				t.Fatalf("repo.Delete called %d times, want %d", repo.deleteCalls, tt.wantDeleteCall)
			}
			if !reflect.DeepEqual(svc.snapshot, beforeSnapshot) {
				t.Fatal("service snapshot changed after rejected hard delete")
			}
			gotAuthority, gotVersion := authority.Current()
			if gotVersion != beforeVersion || !reflect.DeepEqual(gotAuthority, beforeAuthority) {
				t.Fatalf("authority changed after rejected hard delete: version %d catalog=%+v", gotVersion, gotAuthority)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Fatalf("repository calls = %v, want %v", calls, tt.wantCalls)
			}
		})
	}
}

func TestServiceDeleteDoesNotTrustConsumerPreflight(t *testing.T) {
	checks := 0
	repo := &fakeCatalogAdminRepository{
		getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
			return familyRecord(5, "CONDUCTORES", "Conductores", false), nil
		},
		referencedByResourcesFn: func(context.Context, domain.CatalogKindCode, int64) (bool, error) {
			checks++
			return checks > 1, nil
		},
	}
	authority := domain.NewCatalogAuthority(baseSnapshot())
	svc := NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority)

	preflight, err := svc.ReferencedByResources(context.Background(), domain.KindFamily, 5)
	if err != nil || preflight {
		t.Fatalf("consumer preflight = %v, error=%v; want false, nil", preflight, err)
	}
	if err := svc.Delete(context.Background(), domain.KindFamily, 5); !errors.Is(err, domain.ErrCatalogInUse) {
		t.Fatalf("Delete() error = %v, want ErrCatalogInUse from authoritative recheck", err)
	}
	if checks != 2 {
		t.Fatalf("resource reference checks = %d, want consumer preflight plus service guard", checks)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("repo.Delete called %d times, want 0", repo.deleteCalls)
	}
	if _, version := authority.Current(); version != 1 {
		t.Fatalf("authority version = %d, want 1 after rejected delete", version)
	}
}

// --- helpers -----------------------------------------------------------

// reflectEqualCatalog compares two ResourceCatalog snapshots field by field
// on the slices this test package touches. ResourceClass carries
// Aliases/Keywords []string, which makes the whole struct non-comparable
// with ==, so element-wise comparison goes through reflect.DeepEqual.
func reflectEqualCatalog(a, b domain.ResourceCatalog) bool {
	if len(a.Classes) != len(b.Classes) || len(a.Families) != len(b.Families) {
		return false
	}
	for i := range a.Classes {
		if !reflect.DeepEqual(a.Classes[i], b.Classes[i]) {
			return false
		}
	}
	for i := range a.Families {
		if a.Families[i] != b.Families[i] {
			return false
		}
	}
	return true
}

func TestServiceUnitNameValidationAndDuplicateNames(t *testing.T) {
	repo := &fakeCatalogAdminRepository{}
	svc := newTestService(repo)
	unit := func(code, name, symbol string) domain.CatalogRecord {
		return domain.CatalogRecord{Active: true, Values: map[string]domain.CatalogValue{
			"code": {Text: code}, "name": {Text: name}, "symbol": {Text: symbol}, "dimension": {Text: "LENGTH"},
		}}
	}
	if _, err := svc.Create(context.Background(), domain.KindUnit, unit("TEST_BLANK_UNIT", "  ", "TBU")); !errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("blank unit name error = %v, want ErrResourceValidation", err)
	}
	if repo.insertCalls != 0 {
		t.Fatalf("repo.Insert called %d times after invalid name, want 0", repo.insertCalls)
	}
	if _, err := svc.Create(context.Background(), domain.KindUnit, unit("TEST_UNIT_A", "Unidad repetida", "TUA")); err != nil {
		t.Fatalf("first unit create error = %v", err)
	}
	if _, err := svc.Create(context.Background(), domain.KindUnit, unit("TEST_UNIT_B", "Unidad repetida", "TUB")); err != nil {
		t.Fatalf("duplicate unit name create error = %v, want nil", err)
	}
}

func TestServiceUnitNameUpdateRoundTrip(t *testing.T) {
	current := domain.CatalogRecord{Kind: domain.KindUnit, ID: 7, Active: true, Values: map[string]domain.CatalogValue{
		"code": {Text: "M"}, "name": {Text: "Metro"}, "symbol": {Text: "M"}, "dimension": {Text: "LENGTH"},
	}}
	repo := &fakeCatalogAdminRepository{getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
		return current, nil
	}}
	svc := newTestService(repo)
	updated := current
	updated.Values = map[string]domain.CatalogValue{
		"code": {Text: "M"}, "name": {Text: "Metro lineal"}, "symbol": {Text: "M"}, "dimension": {Text: "LENGTH"},
	}
	got, err := svc.Update(context.Background(), domain.KindUnit, updated)
	if err != nil || got.Values["name"].Text != "Metro lineal" {
		t.Fatalf("unit update = %+v, err=%v, want persisted name", got, err)
	}
}

func TestServiceUnitNameUpdateRejectsReferencedCodeChange(t *testing.T) {
	current := domain.CatalogRecord{Kind: domain.KindUnit, ID: 7, Active: true, Values: map[string]domain.CatalogValue{
		"code": {Text: "LEGACY_X"}, "name": {Text: "Provisional: Código LEGACY_X"}, "symbol": {Text: "lx"}, "dimension": {Text: "LENGTH"},
	}}
	repo := &fakeCatalogAdminRepository{
		getFn: func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
			return current, nil
		},
		referencedByResourcesFn: func(context.Context, domain.CatalogKindCode, int64) (bool, error) { return true, nil },
	}
	svc := newTestService(repo)
	updated := current
	updated.Values = map[string]domain.CatalogValue{
		"code": {Text: "LEGACY_RENAMED"}, "name": {Text: "Unidad corregida"}, "symbol": {Text: "lx"}, "dimension": {Text: "LENGTH"},
	}
	_, err := svc.Update(context.Background(), domain.KindUnit, updated)
	if !errors.Is(err, domain.ErrCodeImmutable) {
		t.Fatalf("Update() error = %v, want ErrCodeImmutable while references exist", err)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("repo.Update calls = %d, want 0 after rejected stable-code change", repo.updateCalls)
	}
}

// --- 4C: V2 additive path (design "Catalog transaction and coherent
// publication") ------------------------------------------------------------

// repoV2 is *fakeCatalogAdminRepositoryV2, not the domain.CatalogAdminRepositoryV2
// interface: comparing a nil *fakeCatalogAdminRepositoryV2 against nil after
// it were already boxed into the interface would always be false (typed-nil
// gotcha) and silently defeat the "no repoV2 configured" test row below.
func newV2TestService(repo domain.CatalogAdminRepository, repoV2 *fakeCatalogAdminRepositoryV2, authority *domain.CatalogAuthority) *Service {
	svc := NewServiceWithCatalogAuthority(repo, domain.NewCatalogRegistry(), authority)
	if repoV2 != nil {
		svc.WithCatalogAdminRepositoryV2(repoV2)
	}
	return svc
}

var v2FamilyCurrent = domain.CatalogRecord{Kind: domain.KindFamily, ID: 5, Active: false, Values: map[string]domain.CatalogValue{
	"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}}, "code": {Text: "CONDUCTORES"}, "name": {Text: "Conductores"},
}}

func v2GetFn(rec domain.CatalogRecord) func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) {
	return func(context.Context, domain.CatalogKindCode, int64) (domain.CatalogRecord, error) { return rec, nil }
}

func totalV2Calls(r *fakeCatalogAdminRepositoryV2) int {
	return r.insertCalls + r.updateCalls + r.setActiveCalls + r.deleteCalls
}

type v2WriteCase struct {
	name        string
	repo        *fakeCatalogAdminRepository
	repoV2      *fakeCatalogAdminRepositoryV2
	action      func(*Service) error
	wantErr     error
	wantV2Calls int
}

// TestServiceV2WritesPublishOnlyOnConfirmedCommit is the single table
// covering every no-publication case the design mandates: an unconfigured
// repoV2, an invalid private candidate, HardDeleteRevision's conservative
// guard chain (mirroring TestServiceDeleteConservativeGuards), every
// repository-classified failure category for UpdateRevision/
// HardDeleteRevision (stale CAS, SQL, reload, equivalence, commit — all
// routed through the SAME shared classifyV2WriteError path), and the
// idempotent no-op. In every row, the authority must stay at version 1.
func TestServiceV2WritesPublishOnlyOnConfirmedCommit(t *testing.T) {
	invalidFamily := domain.CatalogRecord{Active: true, Values: map[string]domain.CatalogValue{
		"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "NO_EXISTE"}}, "code": {Text: "NUEVA"}, "name": {Text: "Nueva Familia"},
	}}
	activeTarget := familyRecord(5, "CONDUCTORES", "Conductores", true)
	blockedDeps := []domain.CatalogDependency{{Kind: domain.KindType, Count: 1}}

	cases := []v2WriteCase{
		{"CreateV2 without repoV2 configured", &fakeCatalogAdminRepository{}, nil,
			func(s *Service) error {
				_, err := s.CreateV2(context.Background(), domain.KindUnit, domain.CatalogRecord{})
				return err
			},
			ErrCatalogAdminRepositoryV2Unavailable, 0},
		{"CreateV2 rejects invalid candidate", &fakeCatalogAdminRepository{}, &fakeCatalogAdminRepositoryV2{},
			func(s *Service) error {
				_, err := s.CreateV2(context.Background(), domain.KindFamily, invalidFamily)
				return err
			},
			domain.ErrResourceReference, 0},
		{"HardDeleteRevision rejects active target", &fakeCatalogAdminRepository{getFn: v2GetFn(activeTarget)}, &fakeCatalogAdminRepositoryV2{},
			func(s *Service) error { return s.HardDeleteRevision(context.Background(), domain.KindFamily, 5, 3) },
			domain.ErrInvalidLifecycle, 0},
		{"HardDeleteRevision rejects blocking dependency", &fakeCatalogAdminRepository{
			getFn: v2GetFn(v2FamilyCurrent),
			dependentsFn: func(context.Context, domain.CatalogKindCode, int64) ([]domain.CatalogDependency, error) {
				return blockedDeps, nil
			},
		}, &fakeCatalogAdminRepositoryV2{},
			func(s *Service) error { return s.HardDeleteRevision(context.Background(), domain.KindFamily, 5, 3) },
			domain.ErrCatalogInUse, 0},
		{"HardDeleteRevision rejects resource reference", &fakeCatalogAdminRepository{
			getFn:                   v2GetFn(v2FamilyCurrent),
			referencedByResourcesFn: func(context.Context, domain.CatalogKindCode, int64) (bool, error) { return true, nil },
		}, &fakeCatalogAdminRepositoryV2{},
			func(s *Service) error { return s.HardDeleteRevision(context.Background(), domain.KindFamily, 5, 3) },
			domain.ErrCatalogInUse, 0},
		{"ReactivateRevision on an already-active record is an idempotent no-op", &fakeCatalogAdminRepository{getFn: v2GetFn(activeTarget)}, &fakeCatalogAdminRepositoryV2{},
			func(s *Service) error { return s.ReactivateRevision(context.Background(), domain.KindFamily, 5, 7) },
			nil, 0},
	}
	for _, category := range []string{"stale CAS/conflict", "SQL failure", "reload failure", "equivalence mismatch", "commit failure"} {
		wantErr := errors.New(category)
		cases = append(cases,
			v2WriteCase{"UpdateRevision/" + category, &fakeCatalogAdminRepository{getFn: v2GetFn(v2FamilyCurrent)},
				&fakeCatalogAdminRepositoryV2{updateFn: func(context.Context, domain.CatalogRecord, uint64) (domain.CatalogWriteResult, error) {
					return domain.CatalogWriteResult{}, wantErr
				}},
				func(s *Service) error {
					_, err := s.UpdateRevision(context.Background(), domain.KindFamily, v2FamilyCurrent, 3)
					return err
				},
				wantErr, 1},
			v2WriteCase{"HardDeleteRevision/" + category, &fakeCatalogAdminRepository{getFn: v2GetFn(v2FamilyCurrent)},
				&fakeCatalogAdminRepositoryV2{deleteFn: func(context.Context, domain.CatalogKindCode, int64, uint64) (domain.CatalogWriteResult, error) {
					return domain.CatalogWriteResult{}, wantErr
				}},
				func(s *Service) error { return s.HardDeleteRevision(context.Background(), domain.KindFamily, 5, 3) },
				wantErr, 1},
		)
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			authority := domain.NewCatalogAuthority(baseSnapshot())
			svc := newV2TestService(tt.repo, tt.repoV2, authority)
			if err := tt.action(svc); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.repoV2 != nil && totalV2Calls(tt.repoV2) != tt.wantV2Calls {
				t.Fatalf("repoV2 calls = %d, want %d", totalV2Calls(tt.repoV2), tt.wantV2Calls)
			}
			if _, version := authority.Current(); version != 1 {
				t.Fatalf("authority version = %d, want 1 (nothing published)", version)
			}
		})
	}
}

// TestServiceV2CreatePublishesOnlyRepositoryCoherentResult proves the
// success path publishes ONLY the repository's own coherent
// CatalogWriteResult.Catalog — deliberately divergent (empty Families) from
// the hand-built local candidate — never the candidate itself.
func TestServiceV2CreatePublishesOnlyRepositoryCoherentResult(t *testing.T) {
	coherent := baseSnapshot()
	coherent.Families = nil
	repoV2 := &fakeCatalogAdminRepositoryV2{insertFn: func(ctx context.Context, rec domain.CatalogRecord) (domain.CatalogWriteResult, error) {
		rec.ID = 42
		return domain.NewCatalogWriteResult(&rec, coherent), nil
	}}
	authority := domain.NewCatalogAuthority(baseSnapshot())
	valid := domain.CatalogRecord{Active: true, Values: map[string]domain.CatalogValue{
		"class": {Ref: domain.CatalogRef{Kind: domain.KindClass, Code: "MATERIAL"}}, "code": {Text: "CANALIZACIONES"}, "name": {Text: "Canalizaciones"},
	}}
	got, err := newV2TestService(&fakeCatalogAdminRepository{}, repoV2, authority).CreateV2(context.Background(), domain.KindFamily, valid)
	if err != nil || got.ID != 42 {
		t.Fatalf("CreateV2 = %+v, err=%v, want ID 42, nil error", got, err)
	}
	published, version := authority.Current()
	if version != 2 || len(published.Families) != 0 {
		t.Fatalf("published = %+v at v%d, want the repository's coherent (empty-Families) result at v2", published.Families, version)
	}
}

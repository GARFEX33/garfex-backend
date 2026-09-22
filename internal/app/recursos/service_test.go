package recursos

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// fakeRepo is a fake domain.ResourceRepository. Get is class-scoped: it
// looks resources up by the exact (classCode, identityKey) pair it was
// seeded with, mirroring the real repository's `cl.code = $1 AND
// r.identity_key = $2` matching (design §3) rather than matching on
// identityKey alone. This is what lets TestServiceGetScopesByClassCode (the
// R1 carve-out) prove Service.Get forwards its first argument as the class
// scoping key instead of silently collapsing it.
type fakeRepo struct {
	byClassAndIdentity    map[[2]string]domain.Resource
	resource              domain.Resource
	gotClass, gotIdentity string
	err                   error

	gotCriteria domain.SearchCriteria
	resources   []domain.Resource
	searchErr   error
	page        domain.ResourcePage
	pageErr     error

	gotUpdate domain.Resource
	updateErr error

	gotSetActiveID    int64
	gotSetActiveValue bool
	setActiveErr      error

	gotCreate domain.Resource
	createErr error
}

func (f *fakeRepo) Create(_ context.Context, resource domain.Resource) error {
	f.gotCreate = resource
	return f.createErr
}

func (f *fakeRepo) Get(_ context.Context, classCode, identityKey string) (domain.Resource, error) {
	f.gotClass, f.gotIdentity = classCode, identityKey
	if f.byClassAndIdentity != nil {
		if resource, ok := f.byClassAndIdentity[[2]string{classCode, identityKey}]; ok {
			return resource, nil
		}
		return domain.Resource{}, domain.ErrResourceNotFound
	}
	return f.resource, f.err
}

func (f *fakeRepo) Search(_ context.Context, criteria domain.SearchCriteria) ([]domain.Resource, error) {
	f.gotCriteria = criteria
	return f.resources, f.searchErr
}
func (f *fakeRepo) SearchPage(_ context.Context, criteria domain.SearchCriteria) (domain.ResourcePage, error) {
	f.gotCriteria = criteria
	return f.page, f.pageErr
}

func (f *fakeRepo) Update(_ context.Context, resource domain.Resource) error {
	f.gotUpdate = resource
	return f.updateErr
}

func (f *fakeRepo) SetActive(_ context.Context, id int64, active bool) error {
	f.gotSetActiveID, f.gotSetActiveValue = id, active
	return f.setActiveErr
}

func (f *fakeRepo) Deactivate(_ context.Context, id int64) (domain.LifecycleResult, error) {
	f.gotSetActiveID, f.gotSetActiveValue = id, false
	return domain.LifecycleResult{}, f.setActiveErr
}

func (f *fakeRepo) Reactivate(_ context.Context, id int64, _ string) (domain.LifecycleResult, error) {
	f.gotSetActiveID, f.gotSetActiveValue = id, true
	return domain.LifecycleResult{}, f.setActiveErr
}

// TestServiceGetScopesByClassCode is the mandatory R1 carve-out (design
// §Open Risks R1, tasks 5.1): Service.Get(ctx, classCode, identityKey) keeps
// a (string, string) arity, so a call site accidentally passing a family
// code where a class code belongs compiles cleanly. This test seeds a fake
// repository with two resources sharing the exact same family+identity-key
// shape but scoped under different class codes, and proves Get threads its
// first argument through as the class scoping key rather than collapsing it
// — i.e. asking for the resource under class "MATERIAL" must never return
// the resource seeded under the family code "CONDUCTORES" used as a
// (wrong) class code, even though both share identity key
// "CONDUCTORES|CABLE|COBRE".
func TestServiceGetScopesByClassCode(t *testing.T) {
	const sharedIdentity = "CONDUCTORES|CABLE|COBRE"

	underMaterial := domain.Resource{ID: 1, ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", IdentityKey: sharedIdentity}
	underFamilyCodeMisusedAsClass := domain.Resource{ID: 2, ClassCode: "CONDUCTORES", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M", IdentityKey: sharedIdentity}

	repo := &fakeRepo{byClassAndIdentity: map[[2]string]domain.Resource{
		{"MATERIAL", sharedIdentity}:    underMaterial,
		{"CONDUCTORES", sharedIdentity}: underFamilyCodeMisusedAsClass,
	}}
	svc := NewService(repo, domain.SeedResourceCatalog())

	got, err := svc.Get(context.Background(), "MATERIAL", sharedIdentity)
	if err != nil {
		t.Fatalf("Get(MATERIAL, ...) error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, underMaterial) {
		t.Fatalf("Get(MATERIAL, ...) = %+v, want %+v", got, underMaterial)
	}

	got, err = svc.Get(context.Background(), "CONDUCTORES", sharedIdentity)
	if err != nil {
		t.Fatalf("Get(CONDUCTORES, ...) error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, underFamilyCodeMisusedAsClass) {
		t.Fatalf("Get(CONDUCTORES, ...) = %+v, want %+v — class scoping was collapsed", got, underFamilyCodeMisusedAsClass)
	}
	if got.ID == underMaterial.ID {
		t.Fatalf("Get(CONDUCTORES, ...) returned the MATERIAL-scoped resource — class code was ignored")
	}

	// A class code that was never seeded (not even as a misused family
	// code) must miss cleanly, proving the lookup key really is
	// (classCode, identityKey) and not identityKey alone.
	if _, err := svc.Get(context.Background(), "UNKNOWN_CLASS", sharedIdentity); !errors.Is(err, domain.ErrResourceNotFound) {
		t.Fatalf("Get(UNKNOWN_CLASS, ...) error = %v, want %v", err, domain.ErrResourceNotFound)
	}
}

func TestServiceGet(t *testing.T) {
	repositoryError := errors.New("connection lost")
	cases := []struct {
		name       string
		class      string
		identity   string
		resource   domain.Resource
		repoErr    error
		wantErr    error
		wantCalled bool
	}{
		{name: "returns technical resource", class: "MATERIAL", identity: "MATERIAL|CEMENT|kg", resource: domain.Resource{ClassCode: "MATERIAL", FamilyCode: "CEMENT", NaturalUnit: "kg", IdentityKey: "MATERIAL|CEMENT|kg"}, wantCalled: true},
		{name: "rejects missing class", identity: "MATERIAL|CEMENT|kg", wantErr: ErrInvalidArgument},
		{name: "rejects missing identity", class: "MATERIAL", wantErr: ErrInvalidArgument},
		{name: "propagates not found", class: "MATERIAL", identity: "missing", repoErr: domain.ErrResourceNotFound, wantErr: domain.ErrResourceNotFound, wantCalled: true},
		{name: "wraps repository error", class: "MATERIAL", identity: "MATERIAL|CEMENT|kg", repoErr: repositoryError, wantErr: repositoryError, wantCalled: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{resource: tt.resource, err: tt.repoErr}
			got, err := NewService(repo, domain.SeedResourceCatalog()).Get(context.Background(), tt.class, tt.identity)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Get() error = %v, want %v", err, tt.wantErr)
			}
			if !tt.wantCalled && (repo.gotClass != "" || repo.gotIdentity != "") {
				t.Fatalf("repository called with %q/%q", repo.gotClass, repo.gotIdentity)
			}
			if tt.wantErr == nil && !reflect.DeepEqual(got, tt.resource) {
				t.Fatalf("Get() = %+v, want %+v", got, tt.resource)
			}
		})
	}
}

func TestServiceSearch(t *testing.T) {
	repositoryError := errors.New("connection lost")
	cases := []struct {
		name      string
		criteria  domain.SearchCriteria
		resources []domain.Resource
		repoErr   error
		wantErr   error
	}{
		{name: "passes results through unchanged", criteria: domain.SearchCriteria{ClassCode: "MATERIAL", Text: "THW"}, resources: []domain.Resource{{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", NaturalUnit: "M", IdentityKey: "MATERIAL|CONDUCTORES|a"}, {ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", NaturalUnit: "M", IdentityKey: "MATERIAL|CONDUCTORES|b"}}},
		{name: "passes empty criteria through unchanged", criteria: domain.SearchCriteria{}, resources: nil},
		{name: "wraps repository error", criteria: domain.SearchCriteria{Text: "THW"}, repoErr: repositoryError, wantErr: repositoryError},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{resources: tt.resources, searchErr: tt.repoErr}
			got, err := NewService(repo, domain.SeedResourceCatalog()).Search(context.Background(), tt.criteria)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Search() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && !strings.Contains(err.Error(), "search resources:") {
				t.Fatalf("Search() error = %v, want prefix %q", err, "search resources:")
			}
			if !reflect.DeepEqual(repo.gotCriteria, tt.criteria) {
				t.Fatalf("repository called with %+v, want %+v", repo.gotCriteria, tt.criteria)
			}
			if tt.wantErr == nil && !reflect.DeepEqual(got, tt.resources) {
				t.Fatalf("Search() = %+v, want %+v", got, tt.resources)
			}
		})
	}
}

func TestServiceUpdate(t *testing.T) {
	repositoryError := errors.New("connection lost")
	command := domain.UpdateCommand{ID: 1, Scope: domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}, NaturalUnit: "M", Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")}}
	cases := []struct {
		name       string
		command    domain.UpdateCommand
		repoErr    error
		wantErr    error
		wantCalled bool
	}{
		{name: "happy path calls through", command: command, wantCalled: true},
		{name: "rejects zero id", command: func() domain.UpdateCommand { c := command; c.ID = 0; return c }(), wantErr: ErrInvalidArgument},
		{name: "propagates not found", command: command, repoErr: domain.ErrResourceNotFound, wantErr: domain.ErrResourceNotFound, wantCalled: true},
		{name: "propagates duplicate", command: command, repoErr: domain.ErrDuplicateResource, wantErr: domain.ErrDuplicateResource, wantCalled: true},
		{name: "propagates reference error", command: command, repoErr: domain.ErrResourceReference, wantErr: domain.ErrResourceReference, wantCalled: true},
		{name: "wraps repository error", command: command, repoErr: repositoryError, wantErr: repositoryError, wantCalled: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{updateErr: tt.repoErr}
			got, err := NewService(repo, domain.SeedResourceCatalog()).Update(context.Background(), tt.command)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Update() error = %v, want %v", err, tt.wantErr)
			}
			if !tt.wantCalled && repo.gotUpdate.ID != 0 {
				t.Fatalf("repository called with %+v", repo.gotUpdate)
			}
			if tt.wantCalled && !reflect.DeepEqual(repo.gotUpdate, got) {
				t.Fatalf("repository called with %+v, want %+v", repo.gotUpdate, got)
			}
			if tt.repoErr == repositoryError && !strings.Contains(err.Error(), "update resource") {
				t.Fatalf("Update() error = %v, want prefix %q", err, "update resource")
			}
		})
	}
}

func TestServiceCreate(t *testing.T) {
	repositoryError := errors.New("connection lost")
	command := domain.CreateCommand{Scope: domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}, NaturalUnit: "M", Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"), domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V")}}
	cases := []struct {
		name    string
		repoErr error
		wantErr error
	}{
		{name: "happy path calls through", repoErr: nil, wantErr: nil},
		{name: "propagates duplicate", repoErr: domain.ErrDuplicateResource, wantErr: domain.ErrDuplicateResource},
		{name: "propagates reference error", repoErr: domain.ErrResourceReference, wantErr: domain.ErrResourceReference},
		{name: "wraps repository error", repoErr: repositoryError, wantErr: repositoryError},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{createErr: tt.repoErr}
			got, err := NewService(repo, domain.SeedResourceCatalog()).Create(context.Background(), command)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(repo.gotCreate, got) {
				t.Fatalf("repository called with %+v, want %+v", repo.gotCreate, got)
			}
			if tt.repoErr == repositoryError && !strings.Contains(err.Error(), "create resource") {
				t.Fatalf("Create() error = %v, want prefix %q", err, "create resource")
			}
		})
	}
}

func TestServiceDelete(t *testing.T) {
	repositoryError := errors.New("connection lost")
	cases := []struct {
		name       string
		id         int64
		repoErr    error
		wantErr    error
		wantCalled bool
	}{
		{name: "happy path calls through", id: 1, wantCalled: true},
		{name: "rejects zero id", id: 0, wantErr: ErrInvalidArgument},
		{name: "propagates not found", id: 1, repoErr: domain.ErrResourceNotFound, wantErr: domain.ErrResourceNotFound, wantCalled: true},
		{name: "wraps repository error", id: 1, repoErr: repositoryError, wantErr: repositoryError, wantCalled: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{setActiveErr: tt.repoErr}
			err := NewService(repo, domain.SeedResourceCatalog()).Delete(context.Background(), tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Delete() error = %v, want %v", err, tt.wantErr)
			}
			if !tt.wantCalled && repo.gotSetActiveID != 0 {
				t.Fatalf("repository called with id %d", repo.gotSetActiveID)
			}
			if tt.wantCalled {
				if repo.gotSetActiveID != tt.id {
					t.Fatalf("repository called with id %d, want %d", repo.gotSetActiveID, tt.id)
				}
				if repo.gotSetActiveValue {
					t.Fatalf("repository called with active = %v, want false", repo.gotSetActiveValue)
				}
			}
			if tt.repoErr == repositoryError && !strings.Contains(err.Error(), "delete resource") {
				t.Fatalf("Delete() error = %v, want prefix %q", err, "delete resource")
			}
		})
	}
}

// TestIdentityConflictDistinctFromReactivationImpossibleOnReactivate is the
// RED/TRIANGULATE evidence for 5A: a reactivation candidate whose
// recomputed identity disagrees with the stored one is ErrIdentityConflict
// (never collapsed into the broader ErrReactivationImpossible/ErrResourceValidation
// outcome used for "cannot validate under the current catalog").
func TestIdentityConflictDistinctFromReactivationImpossibleOnReactivate(t *testing.T) {
	t.Run("identity mismatch is ErrIdentityConflict, not ErrReactivationImpossible", func(t *testing.T) {
		resource, err := domain.NewResource(domain.SeedResourceCatalog(), domain.ResourceScope{
			ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE",
		}, "M", []domain.ResourceAttributeValue{
			domain.OptionValue("conductor_material", "COBRE"), domain.OptionValue("gauge", "12 AWG"),
			domain.OptionValue("insulation", "THW"), domain.OptionValue("color", "NEGRO"), domain.OptionValue("voltage", "600 V"),
		})
		if err != nil {
			t.Fatalf("NewResource() error = %v", err)
		}
		resource.ID, resource.Active = 8, false
		resource.IdentityKey = "v1|stale-identity-no-longer-matches-catalog"
		repo := &fakeLifecycleRepo{byID: resource}

		_, gotErr := NewService(repo, domain.SeedResourceCatalog()).Reactivate(context.Background(), resource.ID)
		if !errors.Is(gotErr, domain.ErrIdentityConflict) {
			t.Fatalf("Reactivate() error = %v, want ErrIdentityConflict", gotErr)
		}
		if errors.Is(gotErr, domain.ErrReactivationImpossible) {
			t.Fatalf("Reactivate() error = %v, must not also match ErrReactivationImpossible", gotErr)
		}
		if repo.reactivateCalls != 0 {
			t.Fatalf("repo.reactivateCalls = %d, want 0 (identity mismatch caught before any repository call)", repo.reactivateCalls)
		}
	})

	t.Run("catalog-invalid retained record is ErrReactivationImpossible, not ErrIdentityConflict", func(t *testing.T) {
		resource := domain.Resource{
			ID: 8, ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE", NaturalUnit: "M",
			IdentityKey: "v1|resource", Active: false,
			Attributes: []domain.ResourceAttributeValue{domain.OptionValue("conductor_material", "COBRE")},
		}
		repo := &fakeLifecycleRepo{byID: resource}

		_, gotErr := NewService(repo, domain.SeedResourceCatalog()).Reactivate(context.Background(), resource.ID)
		if !errors.Is(gotErr, domain.ErrReactivationImpossible) {
			t.Fatalf("Reactivate() error = %v, want ErrReactivationImpossible", gotErr)
		}
		if !errors.Is(gotErr, domain.ErrResourceValidation) {
			t.Fatalf("Reactivate() error = %v, want it to still wrap the underlying ErrResourceValidation reason", gotErr)
		}
		if errors.Is(gotErr, domain.ErrIdentityConflict) {
			t.Fatalf("Reactivate() error = %v, must not also match ErrIdentityConflict", gotErr)
		}
		if repo.reactivateCalls != 0 {
			t.Fatalf("repo.reactivateCalls = %d, want 0", repo.reactivateCalls)
		}
	})
}

// fakeReactivateReferenceRepo is a domain.ResourceRepository whose Reactivate
// simulates the repository-level "cannot restore required reference"
// backstop (e.g. an inactive unit/class) — see
// internal/postgres/resource_repository_crud.go's setLifecycle.
type fakeReactivateReferenceRepo struct {
	fakeLifecycleRepo
	reactivateErr error
}

func (f *fakeReactivateReferenceRepo) Reactivate(_ context.Context, id int64, _ string) (domain.LifecycleResult, error) {
	f.reactivateCalls++
	return domain.LifecycleResult{}, f.reactivateErr
}

// TestReactivationImpossibleClassifiesRepositoryReferenceFailure proves the
// repository-level "cannot restore required reference" backstop (raw
// domain.ErrResourceReference) is classified ErrReactivationImpossible by
// the service, not left as the bare request-level sentinel.
func TestReactivationImpossibleClassifiesRepositoryReferenceFailure(t *testing.T) {
	resource := fullReactivationResource(false)
	repo := &fakeReactivateReferenceRepo{fakeLifecycleRepo: fakeLifecycleRepo{byID: resource}, reactivateErr: domain.ErrResourceReference}

	_, err := NewService(repo, domain.SeedResourceCatalog()).Reactivate(context.Background(), resource.ID)
	if !errors.Is(err, domain.ErrReactivationImpossible) || !errors.Is(err, domain.ErrResourceReference) {
		t.Fatalf("Reactivate() error = %v, want it to satisfy errors.Is for both ErrReactivationImpossible and ErrResourceReference", err)
	}
	if repo.reactivateCalls != 1 {
		t.Fatalf("repo.reactivateCalls = %d, want 1", repo.reactivateCalls)
	}
}

func TestServiceDescribeUsesCurrentCommittedCatalog(t *testing.T) {
	catalog := domain.SeedResourceCatalog()
	authority := domain.NewCatalogAuthority(catalog)
	service := NewServiceWithCatalogAuthority(nil, authority)
	resource := domain.Resource{
		ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE",
		Attributes: []domain.ResourceAttributeValue{domain.OptionValue("insulation", "THW-LS")},
	}

	before := service.Describe(resource)
	catalog.Types = append([]domain.ResourceType(nil), catalog.Types...)
	catalog.Types[0].Name = "Cable actualizado"
	authority.Publish(catalog)
	after := service.Describe(resource)
	fresh := NewService(nil, catalog).Describe(resource)

	if before == after || after != fresh {
		t.Fatalf("same-session presentation = %q, fresh presentation = %q, previous = %q", after, fresh, before)
	}
}

func TestServiceEffectiveAttributesUsesCurrentCommittedCatalog(t *testing.T) {
	catalog := domain.SeedResourceCatalog()
	authority := domain.NewCatalogAuthority(catalog)
	service := NewServiceWithCatalogAuthority(nil, authority)
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}

	attributes, err := service.EffectiveAttributes(scope, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attributes) == 0 {
		t.Fatalf("EffectiveAttributes() returned no attributes for CABLE")
	}
	if code := attributes[0].Attribute.Definition.Code; code != "insulation" {
		t.Fatalf("EffectiveAttributes()[0].Attribute.Definition.Code = %q, want %q", code, "insulation")
	}

	catalog.Attributes = append([]domain.ResourceAttribute(nil), catalog.Attributes...)
	catalog.Attributes[0].Mode = domain.ModeOptional
	authority.Publish(catalog)
	after, err := service.EffectiveAttributes(scope, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byCode := map[string]domain.EffectiveAttribute{}
	for _, a := range after {
		byCode[a.Attribute.Definition.Code] = a
	}
	if mode := byCode[catalog.Attributes[0].Definition.Code].EffectiveMode; mode != domain.ModeOptional {
		t.Fatalf("EffectiveAttributes() after Publish did not reflect updated catalog: mode = %q, want %q", mode, domain.ModeOptional)
	}
}

func TestServiceEffectiveAttributesPropagatesValueValidationError(t *testing.T) {
	authority := domain.NewCatalogAuthority(domain.SeedResourceCatalog())
	service := NewServiceWithCatalogAuthority(nil, authority)
	scope := domain.ResourceScope{ClassCode: "MATERIAL", FamilyCode: "CONDUCTORES", TypeCode: "CABLE"}

	mismatched := domain.ResourceAttributeValue{AttributeCode: "insulation", Type: domain.ValueTypeControlledText, Text: "DESNUDO"}
	_, err := service.EffectiveAttributes(scope, []domain.ResourceAttributeValue{mismatched})
	if !errors.Is(err, domain.ErrResourceValidation) {
		t.Fatalf("EffectiveAttributes(mismatched type) error = %v, want ErrResourceValidation", err)
	}
}

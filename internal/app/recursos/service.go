// Package recursos provides the application use cases for the unified
// Resource Master. It owns construction and validation before repository
// calls, and exposes class-scoped reads plus explicit lifecycle transitions.
package recursos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

// ErrInvalidArgument is returned when a resource lookup key is incomplete.
var ErrInvalidArgument = errors.New("resource lookup argument is required")

// Service implements Resource Master use cases.
type Service struct {
	repo      domain.ResourceRepository
	authority *domain.CatalogAuthority

	// orderStore is the optional per-Tipo visual-order capability, wired
	// additively via WithAttributeOrderStore. It is never consulted by
	// Describe/EffectiveAttributes/Create/etc. above, and ReadAttributeOrder/
	// WriteAttributeOrder never touch authority — see attribute_order.go.
	orderStore domain.AttributeOrderStore
}

// NewService returns a Service whose catalog authority constructs and validates
// write candidates before the domain repository is called.
func NewService(repo domain.ResourceRepository, catalog domain.ResourceCatalog) *Service {
	return NewServiceWithCatalogAuthority(repo, domain.NewCatalogAuthority(catalog))
}

// NewServiceWithCatalogAuthority resolves catalog-controlled behavior from the
// current committed version.
func NewServiceWithCatalogAuthority(repo domain.ResourceRepository, authority *domain.CatalogAuthority) *Service {
	return &Service{repo: repo, authority: authority}
}

// Get returns a resource by its owning class code and deterministic
// identity key. classCode is the resource's ResourceClass.Code (design R1)
// — IdentityKey already encodes class|family|type, so class+key is the
// minimal complete lookup; a family code here will not match.
func (s *Service) Get(ctx context.Context, classCode, identityKey string) (domain.Resource, error) {
	if strings.TrimSpace(classCode) == "" || strings.TrimSpace(identityKey) == "" {
		return domain.Resource{}, ErrInvalidArgument
	}
	r, err := s.repo.Get(ctx, classCode, identityKey)
	if err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) {
			return domain.Resource{}, domain.ErrResourceNotFound
		}
		return domain.Resource{}, fmt.Errorf("get resource %s/%s: %w", classCode, identityKey, err)
	}
	return r, nil
}

func (s *Service) Search(ctx context.Context, criteria domain.SearchCriteria) ([]domain.Resource, error) {
	resources, err := s.repo.Search(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("search resources: %w", err)
	}
	return resources, nil
}

func (s *Service) SearchPage(ctx context.Context, criteria domain.SearchCriteria) (domain.ResourcePage, error) {
	normalized, err := criteria.Normalize()
	if err != nil {
		return domain.ResourcePage{}, err
	}
	paged, ok := s.repo.(domain.ResourcePageRepository)
	if !ok {
		return domain.ResourcePage{}, errors.New("resource repository does not support paged search")
	}
	page, err := paged.SearchPage(ctx, normalized)
	if err != nil {
		return domain.ResourcePage{}, fmt.Errorf("search resources: %w", err)
	}
	page.Criteria = normalized
	return page, nil
}

// Describe resolves the canonical presentation of resource using the
// catalog-controlled configuration owned by its ResourceType.
func (s *Service) Describe(resource domain.Resource) string {
	catalog, _ := s.authority.Current()
	return catalog.Describe(resource)
}

// EffectiveAttributes resolves scope's applicable attributes against the
// current committed catalog (domain.ResourceCatalog.EffectiveAttributesFor),
// combining inheritance, presentation order, and rule evaluation into one
// UI-ready view. current is the caller's already-known values (nil for a
// static, pre-input baseline); passing values reuses the exact same
// conditional-rule evaluation and value validation NewResource applies
// internally, so a malformed value (wrong type, unknown or repeated
// attribute) is rejected the same way here.
func (s *Service) EffectiveAttributes(scope domain.ResourceScope, current []domain.ResourceAttributeValue) ([]domain.EffectiveAttribute, error) {
	catalog, _ := s.authority.Current()
	return catalog.EffectiveAttributesFor(scope, current)
}

// Create constructs and validates a canonical resource before persistence.
func (s *Service) Create(ctx context.Context, command domain.CreateCommand) (domain.Resource, error) {
	catalog, _ := s.authority.Current()
	resource, err := domain.NewResource(catalog, command.Scope, command.NaturalUnit, command.Attributes)
	if err != nil {
		return domain.Resource{}, err
	}
	if err := s.repo.Create(ctx, resource); err != nil {
		if errors.Is(err, domain.ErrDuplicateResource) {
			return resource, domain.ErrDuplicateResource
		}
		if errors.Is(err, domain.ErrResourceReference) {
			return resource, domain.ErrResourceReference
		}
		if errors.Is(err, domain.ErrResourceIntegrity) {
			return resource, domain.ErrResourceIntegrity
		}
		return resource, fmt.Errorf("create resource: %w", err)
	}
	return resource, nil
}

// Update constructs a canonical resource from intent and attaches only the
// stable persistence ID supplied by the caller.
func (s *Service) Update(ctx context.Context, command domain.UpdateCommand) (domain.Resource, error) {
	if command.ID <= 0 {
		return domain.Resource{}, ErrInvalidArgument
	}
	catalog, _ := s.authority.Current()
	resource, err := domain.NewResource(catalog, command.Scope, command.NaturalUnit, command.Attributes)
	if err != nil {
		return domain.Resource{}, err
	}
	resource.ID = command.ID
	if err := s.repo.Update(ctx, resource); err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) {
			return resource, domain.ErrResourceNotFound
		}
		if errors.Is(err, domain.ErrDuplicateResource) {
			return resource, domain.ErrDuplicateResource
		}
		if errors.Is(err, domain.ErrResourceReference) {
			return resource, domain.ErrResourceReference
		}
		if errors.Is(err, domain.ErrResourceIntegrity) {
			return resource, domain.ErrResourceIntegrity
		}
		return resource, fmt.Errorf("update resource %d: %w", resource.ID, err)
	}
	return resource, nil
}

// Deactivate marks a resource inactive without removing its identity or data.
func (s *Service) Deactivate(ctx context.Context, id int64) (domain.LifecycleResult, error) {
	if id <= 0 {
		return domain.LifecycleResult{}, ErrInvalidArgument
	}
	result, err := s.repo.Deactivate(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) {
			return domain.LifecycleResult{}, domain.ErrResourceNotFound
		}
		return domain.LifecycleResult{}, fmt.Errorf("deactivate resource %d: %w", id, err)
	}
	return result, nil
}

// Delete is a compatibility alias for Deactivate. It never physically removes
// a resource; new callers should use the explicit lifecycle method.
func (s *Service) Delete(ctx context.Context, id int64) error {
	_, err := s.Deactivate(ctx, id)
	if err == nil || errors.Is(err, ErrInvalidArgument) || errors.Is(err, domain.ErrResourceNotFound) {
		return err
	}
	return fmt.Errorf("delete resource %d: %w", id, err)
}

// Reactivate validates the current active catalog and identity before asking
// the repository to perform its transactional guarded transition.
func (s *Service) Reactivate(ctx context.Context, id int64) (domain.LifecycleResult, error) {
	if id <= 0 {
		return domain.LifecycleResult{}, ErrInvalidArgument
	}
	finder, ok := s.repo.(interface {
		GetByID(context.Context, int64) (domain.Resource, error)
	})
	if !ok {
		return domain.LifecycleResult{}, errors.New("resource repository cannot load lifecycle identity")
	}
	current, err := finder.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) {
			return domain.LifecycleResult{}, domain.ErrResourceNotFound
		}
		return domain.LifecycleResult{}, fmt.Errorf("load resource %d for reactivation: %w", id, err)
	}
	if current.Active {
		return domain.LifecycleResult{Resource: current}, nil
	}
	candidate, err := s.reactivationCandidate(current)
	if err != nil {
		return domain.LifecycleResult{Resource: current}, err
	}
	result, err := s.repo.Reactivate(ctx, id, candidate.IdentityKey)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrResourceNotFound), errors.Is(err, domain.ErrDuplicateResource):
			return domain.LifecycleResult{Resource: current}, err
		case errors.Is(err, domain.ErrResourceIntegrity):
			return domain.LifecycleResult{Resource: current}, domain.ErrIdentityConflict
		case errors.Is(err, domain.ErrResourceReference):
			return domain.LifecycleResult{Resource: current}, domain.WrapReactivationImpossible(err)
		default:
			return domain.LifecycleResult{Resource: current}, fmt.Errorf("reactivate resource %d: %w", id, err)
		}
	}
	return result, nil
}

// reactivationCandidate rebuilds and authoritatively re-validates current
// against the current committed catalog, requiring the rebuilt identity-v1
// to equal current's stored one exactly. Shared by Reactivate and
// ReactivateRevision so both validate identically before any repository call.
func (s *Service) reactivationCandidate(current domain.Resource) (domain.Resource, error) {
	catalog, _ := s.authority.Current()
	values := make([]domain.ResourceAttributeValue, 0, len(current.Attributes))
	for _, value := range current.Attributes {
		if value.Text != domain.NotApplicableText {
			values = append(values, value)
		}
	}
	candidate, err := domain.NewResource(catalog, domain.ResourceScope{
		ClassCode: current.ClassCode, FamilyCode: current.FamilyCode, TypeCode: current.TypeCode,
	}, current.NaturalUnit, values)
	if err != nil {
		return domain.Resource{}, domain.WrapReactivationImpossible(err)
	}
	if candidate.IdentityKey != current.IdentityKey {
		return domain.Resource{}, domain.ErrIdentityConflict
	}
	return candidate, nil
}

// DeactivateRevision is the additive CAS-aware sibling of Deactivate:
// expectedRevision must be non-zero and match the persisted revision, or
// the call is rejected as domain.ErrResourceRevisionConflict. It coexists
// with Deactivate; the legacy path is unaffected.
func (s *Service) DeactivateRevision(ctx context.Context, id int64, expectedRevision uint64) (domain.LifecycleResult, error) {
	if id <= 0 || expectedRevision == 0 {
		return domain.LifecycleResult{}, ErrInvalidArgument
	}
	v2, ok := s.repo.(domain.ResourceRepositoryV2)
	if !ok {
		return domain.LifecycleResult{}, errors.New("resource repository does not support revision-aware lifecycle")
	}
	result, err := v2.DeactivateRevision(ctx, id, expectedRevision)
	if err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) || errors.Is(err, domain.ErrResourceRevisionConflict) {
			return domain.LifecycleResult{}, err
		}
		return domain.LifecycleResult{}, fmt.Errorf("deactivate resource %d: %w", id, err)
	}
	return domain.LifecycleResult{Resource: result, Changed: result.Revision != expectedRevision}, nil
}

// ReactivateRevision is the additive CAS-aware sibling of Reactivate: same
// authoritative validation (reactivationCandidate), then a
// domain.ResourceRepositoryV2 CAS transition instead of Reactivate's
// identity-only guard. It coexists with Reactivate.
func (s *Service) ReactivateRevision(ctx context.Context, id int64, expectedRevision uint64) (domain.LifecycleResult, error) {
	if id <= 0 || expectedRevision == 0 {
		return domain.LifecycleResult{}, ErrInvalidArgument
	}
	finder, ok := s.repo.(interface {
		GetByID(context.Context, int64) (domain.Resource, error)
	})
	if !ok {
		return domain.LifecycleResult{}, errors.New("resource repository cannot load lifecycle identity")
	}
	current, err := finder.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) {
			return domain.LifecycleResult{}, domain.ErrResourceNotFound
		}
		return domain.LifecycleResult{}, fmt.Errorf("load resource %d for reactivation: %w", id, err)
	}
	if current.Active {
		return domain.LifecycleResult{Resource: current}, nil
	}
	candidate, err := s.reactivationCandidate(current)
	if err != nil {
		return domain.LifecycleResult{Resource: current}, err
	}
	v2, ok := s.repo.(domain.ResourceRepositoryV2)
	if !ok {
		return domain.LifecycleResult{Resource: current}, errors.New("resource repository does not support revision-aware lifecycle")
	}
	result, err := v2.ReactivateRevision(ctx, id, candidate.IdentityKey, expectedRevision)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrResourceRevisionConflict), errors.Is(err, domain.ErrResourceNotFound):
			return domain.LifecycleResult{Resource: current}, err
		case errors.Is(err, domain.ErrResourceIntegrity):
			return domain.LifecycleResult{Resource: current}, domain.ErrIdentityConflict
		case errors.Is(err, domain.ErrResourceReference):
			return domain.LifecycleResult{Resource: current}, domain.WrapReactivationImpossible(err)
		default:
			return domain.LifecycleResult{Resource: current}, fmt.Errorf("reactivate resource %d: %w", id, err)
		}
	}
	return domain.LifecycleResult{Resource: result, Changed: result.Revision != expectedRevision}, nil
}

// UpdateRevision is Update's CAS-aware sibling: same authoritative validation, then ResourceRepositoryV2's atomic CAS.
func (s *Service) UpdateRevision(ctx context.Context, command domain.UpdateCommand, expectedRevision uint64) (domain.Resource, error) {
	if command.ID <= 0 || expectedRevision == 0 {
		return domain.Resource{}, ErrInvalidArgument
	}
	catalog, _ := s.authority.Current()
	resource, err := domain.NewResource(catalog, command.Scope, command.NaturalUnit, command.Attributes)
	if err != nil {
		return domain.Resource{}, err
	}
	resource.ID = command.ID
	v2, ok := s.repo.(domain.ResourceRepositoryV2)
	if !ok {
		return resource, errors.New("resource repository does not support revision-aware update")
	}
	result, err := v2.UpdateRevision(ctx, resource, expectedRevision)
	if err != nil {
		if errors.Is(err, domain.ErrResourceNotFound) || errors.Is(err, domain.ErrDuplicateResource) || errors.Is(err, domain.ErrResourceReference) || errors.Is(err, domain.ErrResourceIntegrity) || errors.Is(err, domain.ErrResourceRevisionConflict) {
			return resource, err
		}
		return resource, fmt.Errorf("update resource %d: %w", resource.ID, err)
	}
	return result, nil
}

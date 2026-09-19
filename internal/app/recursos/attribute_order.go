package recursos

import (
	"context"
	"fmt"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/core"
	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
)

// ErrAttributeOrderStoreUnavailable is returned by ReadAttributeOrder and
// WriteAttributeOrder when the Service was never wired via
// WithAttributeOrderStore — the capability is optional, mirroring
// catalogo.ErrCatalogAdminRepositoryV2Unavailable.
var ErrAttributeOrderStoreUnavailable = fmt.Errorf("%w: attribute order store is not configured", core.ErrUnavailable)

// WithAttributeOrderStore additively wires store into s and returns s,
// enabling ReadAttributeOrder/WriteAttributeOrder without touching any
// existing method's behavior.
func (s *Service) WithAttributeOrderStore(store domain.AttributeOrderStore) *Service {
	s.orderStore = store
	return s
}

// AttributeOrderStoreConfigured reports whether store is wired, so a
// DB-less composition test can assert that a production/integration
// composition root owns the attribute-order capability without a real
// database.
func (s *Service) AttributeOrderStoreConfigured() bool {
	return s.orderStore != nil
}

// ReadAttributeOrder is a thin pass-through to the wired
// domain.AttributeOrderStore. It deliberately never touches s.authority:
// the store already loads its own DB-backed coherent ResourceCatalog
// snapshot internally, independent of the process-local catalog authority
// cache (see the domain package's AttributeOrderReader doc comment).
func (s *Service) ReadAttributeOrder(ctx context.Context, scope domain.ResourceScope) (domain.AttributeOrderReadResult, error) {
	if s.orderStore == nil {
		return domain.AttributeOrderReadResult{}, ErrAttributeOrderStoreUnavailable
	}
	return s.orderStore.ReadAttributeOrder(ctx, scope)
}

// WriteAttributeOrder is a thin pass-through to the wired
// domain.AttributeOrderStore. Like ReadAttributeOrder, it never touches
// s.authority: the store's own locked CAS transaction owns validation and
// revision semantics.
func (s *Service) WriteAttributeOrder(ctx context.Context, req domain.AttributeOrderWriteRequest) (domain.AttributeOrderReadResult, error) {
	if s.orderStore == nil {
		return domain.AttributeOrderReadResult{}, ErrAttributeOrderStoreUnavailable
	}
	return s.orderStore.WriteAttributeOrder(ctx, req)
}

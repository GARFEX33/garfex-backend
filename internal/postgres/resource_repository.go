// Package postgres provides PostgreSQL-backed implementations of domain ports.
package postgres

import "github.com/GARFEX33/garfex-backend/internal/domain"

// resourceRepository additionally satisfies domain.ResourceRepositoryV2
// (DeactivateRevision/ReactivateRevision, resource_repository_crud.go) —
// real and callable. A caller type-asserts for it like
// recursos.Service.SearchPage already does for ResourcePageRepository.
var _ domain.ResourceRepositoryV2 = (*resourceRepository)(nil)

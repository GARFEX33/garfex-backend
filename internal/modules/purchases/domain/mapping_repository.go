package domain

import (
	"context"
	"errors"
)

var (
	ErrResourceNotFound = errors.New("purchase master resource not found")
	ErrResourceInactive = errors.New("purchase master resource is inactive")
	ErrCommitAmbiguous  = errors.New("purchase master commit outcome is ambiguous")
)

// ConfirmMappingCommand confirms an unresolved SupplierProduct mapping.
type ConfirmMappingCommand struct {
	SupplierProductID int64
	ResourceID        int64
	ExpectedRevision  MappingRevision
	Decision          MappingDecisionMetadata
}

// CorrectMappingCommand replaces a confirmed mapping after checking both
// optimistic revision and the expected current Resource.
type CorrectMappingCommand struct {
	SupplierProductID       int64
	ExpectedCurrentResource int64
	ResourceID              int64
	ExpectedRevision        MappingRevision
	Decision                MappingDecisionMetadata
}

// ExceptionalUnlinkCommand removes a confirmed mapping as an explicit
// exceptional correction.
type ExceptionalUnlinkCommand struct {
	SupplierProductID       int64
	ExpectedCurrentResource int64
	ExpectedRevision        MappingRevision
	Decision                MappingDecisionMetadata
}

// ReportIdentityConflictCommand marks the current confirmed identity as
// conflicting without discarding its retained Resource mapping.
type ReportIdentityConflictCommand struct {
	SupplierProductID       int64
	ExpectedCurrentResource int64
	ExpectedRevision        MappingRevision
	Decision                MappingDecisionMetadata
}

// ResolveIdentityConflictCommand clears an identity conflict and confirms its
// replacement Resource in one semantic transition.
type ResolveIdentityConflictCommand struct {
	SupplierProductID       int64
	ExpectedCurrentResource int64
	ResourceID              int64
	ExpectedRevision        MappingRevision
	Decision                MappingDecisionMetadata
}

// MappingRepository persists semantic mapping transitions and their audit.
type MappingRepository interface {
	ConfirmMapping(context.Context, ConfirmMappingCommand) (SupplierProduct, error)
	CorrectMapping(context.Context, CorrectMappingCommand) (SupplierProduct, error)
	ExceptionalUnlink(context.Context, ExceptionalUnlinkCommand) (SupplierProduct, error)
	ReportIdentityConflict(context.Context, ReportIdentityConflictCommand) (SupplierProduct, error)
	ResolveIdentityConflict(context.Context, ResolveIdentityConflictCommand) (SupplierProduct, error)
	ListMappingAudit(context.Context, int64, ListCriteria) ([]MappingAuditEntry, error)
}

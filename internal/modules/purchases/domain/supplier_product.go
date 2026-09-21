package domain

import (
	"fmt"
	"strings"
	"time"
)

// SupplierProduct is the aggregate root for reusable supplier SKU knowledge.
// CurrentMapping contains the only current mapping authority. Mapping audit
// entries are returned by changed transitions for persistence to append; the
// aggregate does not own the audit history.
type SupplierProduct struct {
	ID          int64
	SupplierID  int64
	SupplierSKU string
	// Description is the last-seen description for this SKU. It is
	// informational only and never authoritative: each PurchaseLine keeps
	// its own original description regardless of this value.
	Description     string
	CurrentMapping  SupplierProductMapping
	MappingRevision MappingRevision
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewSupplierProductIdentity validates the identity fields required to
// create or reuse a SupplierProduct during import. SKU identity uses only
// TrimSpace; case, punctuation, hyphens, and leading zeroes remain exact.
func NewSupplierProductIdentity(supplierID int64, sku, description string) (int64, string, string, error) {
	sku = strings.TrimSpace(sku)
	description = strings.TrimSpace(description)
	if supplierID <= 0 {
		return 0, "", "", NewValidationError("supplier_id", "must be positive")
	}
	if sku == "" {
		return 0, "", "", NewValidationError("supplier_sku", "is required")
	}
	return supplierID, sku, description, nil
}

// NewSupplierProduct creates an aggregate with unresolved mapping knowledge.
func NewSupplierProduct(supplierID int64, sku, description string) (SupplierProduct, error) {
	supplierID, sku, description, err := NewSupplierProductIdentity(supplierID, sku, description)
	if err != nil {
		return SupplierProduct{}, err
	}
	return SupplierProduct{
		SupplierID:     supplierID,
		SupplierSKU:    sku,
		Description:    description,
		CurrentMapping: NewUnresolvedSupplierProductMapping(),
	}, nil
}

// UpdateDescription changes informational last-seen data only. It never
// changes mapping revision or mapping audit.
func (sp SupplierProduct) UpdateDescription(description string) SupplierProduct {
	sp.Description = strings.TrimSpace(description)
	return sp
}

// ConfirmMapping records a first confirmed target. Reconfirming the same
// target is an idempotent no-op; changing an existing target requires
// CorrectMapping.
func (sp *SupplierProduct) ConfirmMapping(resourceID int64, expected MappingRevision, decision MappingDecisionMetadata) (MappingTransition, error) {
	if resourceID <= 0 {
		return MappingTransition{}, NewValidationError("resource_id", "must be positive")
	}
	next := NewConfirmedSupplierProductMapping(resourceID)
	if sp.CurrentMapping.KnowledgeState() == MappingStateConfirmed {
		if sp.CurrentMapping.equal(next) {
			if err := decision.validate(false); err != nil {
				return MappingTransition{}, err
			}
			if sp.MappingRevision != expected {
				return MappingTransition{}, ErrStaleMappingRevision
			}
			return MappingTransition{}, nil
		}
		return MappingTransition{}, ErrInvalidMappingTransition
	}
	if sp.CurrentMapping.KnowledgeState() != MappingStateUnresolved {
		return MappingTransition{}, ErrInvalidMappingTransition
	}
	return sp.transition(MappingOperationConfirm, next, expected, decision)
}

func expectedResourceError(expected int64) error {
	return fmt.Errorf("%w: expected current resource %d", ErrInvalidMappingTransition, expected)
}

// CorrectMapping replaces the current confirmed target after checking the
// expected current ResourceID. New ResourceID is intentionally separate from
// that optimistic semantic guard.
func (sp *SupplierProduct) CorrectMapping(expectedResourceID, resourceID int64, expected MappingRevision, decision MappingDecisionMetadata) (MappingTransition, error) {
	if resourceID <= 0 {
		return MappingTransition{}, NewValidationError("resource_id", "must be positive")
	}
	if sp.CurrentMapping.KnowledgeState() != MappingStateConfirmed || sp.CurrentMapping.ResourceID == nil || *sp.CurrentMapping.ResourceID == resourceID {
		return MappingTransition{}, ErrInvalidMappingTransition
	}
	if expectedResourceID <= 0 || *sp.CurrentMapping.ResourceID != expectedResourceID {
		return MappingTransition{}, expectedResourceError(expectedResourceID)
	}
	return sp.transition(MappingOperationCorrect, NewConfirmedSupplierProductMapping(resourceID), expected, decision)
}

// ExceptionalUnlink removes a confirmed target after checking the expected
// current ResourceID.
func (sp *SupplierProduct) ExceptionalUnlink(expectedResourceID int64, expected MappingRevision, decision MappingDecisionMetadata) (MappingTransition, error) {
	if sp.CurrentMapping.KnowledgeState() != MappingStateConfirmed || sp.CurrentMapping.ResourceID == nil {
		return MappingTransition{}, ErrInvalidMappingTransition
	}
	if expectedResourceID <= 0 || *sp.CurrentMapping.ResourceID != expectedResourceID {
		return MappingTransition{}, expectedResourceError(expectedResourceID)
	}
	return sp.transition(MappingOperationExceptionalUnlink, NewUnresolvedSupplierProductMapping(), expected, decision)
}

// ReportIdentityConflict stores an identity conflict without discarding the
// previously known target, after checking the expected current ResourceID.
func (sp *SupplierProduct) ReportIdentityConflict(expectedResourceID int64, expected MappingRevision, decision MappingDecisionMetadata) (MappingTransition, error) {
	if sp.CurrentMapping.KnowledgeState() != MappingStateConfirmed || sp.CurrentMapping.ResourceID == nil {
		return MappingTransition{}, ErrInvalidMappingTransition
	}
	if expectedResourceID <= 0 || *sp.CurrentMapping.ResourceID != expectedResourceID {
		return MappingTransition{}, expectedResourceError(expectedResourceID)
	}
	next := sp.CurrentMapping.copy()
	next.IdentityConflict = true
	return sp.transition(MappingOperationReportIdentityConflict, next, expected, decision)
}

// ResolveIdentityConflict confirms the resolved target after checking the
// ResourceID retained under conflict separately from the resolved target.
func (sp *SupplierProduct) ResolveIdentityConflict(expectedResourceID, resourceID int64, expected MappingRevision, decision MappingDecisionMetadata) (MappingTransition, error) {
	if resourceID <= 0 {
		return MappingTransition{}, NewValidationError("resource_id", "must be positive")
	}
	if sp.CurrentMapping.KnowledgeState() != MappingStateIdentityConflict || sp.CurrentMapping.ResourceID == nil {
		return MappingTransition{}, ErrInvalidMappingTransition
	}
	if expectedResourceID <= 0 || *sp.CurrentMapping.ResourceID != expectedResourceID {
		return MappingTransition{}, expectedResourceError(expectedResourceID)
	}
	return sp.transition(MappingOperationResolveIdentityConflict, NewConfirmedSupplierProductMapping(resourceID), expected, decision)
}

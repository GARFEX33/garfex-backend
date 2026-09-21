package domain

import (
	"errors"
	"fmt"
)

var (
	ErrValidation              = errors.New("purchase master validation failed")
	ErrNotFound                = errors.New("purchase master record not found")
	ErrConflict                = errors.New("purchase master conflict")
	ErrPurchaseNotFound        = fmt.Errorf("%w: purchase", ErrNotFound)
	ErrPurchaseLineNotFound    = fmt.Errorf("%w: purchase line", ErrNotFound)
	ErrSupplierProductNotFound = fmt.Errorf("%w: supplier product", ErrNotFound)
	// ErrPurchaseConflict is returned when an incoming document shares the
	// fiscal UUID of an already-registered purchase but its relevant content
	// does not match. The existing purchase is never overwritten.
	ErrPurchaseConflict = fmt.Errorf("%w: purchase uuid already registered with different content", ErrConflict)

	// Mapping errors distinguish a rejected business transition, an optimistic
	// concurrency failure, and malformed decision metadata.
	ErrInvalidMappingTransition = errors.New("purchase master invalid mapping transition")
	ErrStaleMappingRevision     = errors.New("purchase master stale mapping revision")
	ErrInvalidDecisionMetadata  = errors.New("purchase master invalid decision metadata")

	// Line-centered resolution errors remain distinct so delivery adapters can
	// map each business outcome without parsing messages.
	ErrCommercialSupplierSKURequired  = errors.New("purchase master commercial supplier sku is required")
	ErrCommercialSupplierSKUForbidden = errors.New("purchase master commercial supplier sku is forbidden")
	ErrPurchaseLineStateConflict      = errors.New("purchase master purchase line state conflict")
	ErrStaleResolutionRevision        = errors.New("purchase master stale resolution revision")
	ErrMappingTargetConflict          = errors.New("purchase master mapping target conflict")
	ErrPurchaseIntegrityConflict      = errors.New("purchase master integrity conflict")

	// ErrMappingRevisionConflict is retained as the domain vocabulary for
	// callers that describe a stale mapping revision as a conflict.
	ErrMappingRevisionConflict = ErrStaleMappingRevision
	ErrInvalidMappingDecision  = ErrInvalidDecisionMetadata
)

type ValidationError struct {
	Field   string
	Message string
}

func NewValidationError(field, message string) ValidationError {
	return ValidationError{Field: field, Message: message}
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (ValidationError) Unwrap() error { return ErrValidation }

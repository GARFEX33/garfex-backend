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

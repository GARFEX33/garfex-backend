// Package core provides the internal neutral GARFEX error boundary: stable
// categories, an exhaustive typed mapper, and the opaque diagnostic seam.
// Nothing here parses error strings, SQLSTATE values, constraint names, or
// driver messages.
package core

import (
	"context"
	"errors"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/domain"
)

// ErrorCode is one of the fifteen stable GARFEX outcome categories.
type ErrorCode string

const (
	InvalidArgument        ErrorCode = "INVALID_ARGUMENT"
	NotFound               ErrorCode = "NOT_FOUND"
	Duplicate              ErrorCode = "DUPLICATE"
	InvalidReference       ErrorCode = "INVALID_REFERENCE"
	Validation             ErrorCode = "VALIDATION"
	Integrity              ErrorCode = "INTEGRITY"
	IdentityConflict       ErrorCode = "IDENTITY_CONFLICT"
	InvalidLifecycle       ErrorCode = "INVALID_LIFECYCLE"
	ReactivationImpossible ErrorCode = "REACTIVATION_IMPOSSIBLE"
	InvalidCatalog         ErrorCode = "INVALID_CATALOG"
	InUse                  ErrorCode = "IN_USE"
	ImmutableCode          ErrorCode = "IMMUTABLE_CODE"
	Conflict               ErrorCode = "CONFLICT"
	Unavailable            ErrorCode = "UNAVAILABLE"
	Internal               ErrorCode = "INTERNAL"
)

// ErrInvalidArgument and ErrUnavailable are internal classifier sentinels.
// Application boundaries may return or wrap these values so Map recognizes
// them without importing the application package.
var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrUnavailable     = errors.New("service unavailable")
)

// Error is a neutral GARFEX error. It carries only a stable code and a safe
// message; it does not implement Unwrap or expose a cause.
type Error struct {
	code    ErrorCode
	message string
}

func (e Error) Error() string   { return e.message }
func (e Error) Code() ErrorCode { return e.code }

// New returns a neutral GARFEX error.
func New(code ErrorCode, message string) Error {
	return Error{code: code, message: message}
}

// Map translates any error into a neutral GARFEX Error using only errors.Is
// and typed classification. The original cause is not retained.
func Map(err error) Error {
	if err == nil {
		return Error{}
	}
	var e Error
	if errors.As(err, &e) {
		return e
	}

	// Explicit precedence: revision, identity, reactivation, invalid catalog,
	// lifecycle, immutable code, in-use, integrity, then broader categories.
	switch {
	case errors.Is(err, domain.ErrRevisionConflict):
		return New(Conflict, messages[Conflict])
	case errors.Is(err, domain.ErrAttributeOrderRevisionConflict):
		return New(Conflict, messages[Conflict])
	case errors.Is(err, domain.ErrIdentityConflict):
		return New(IdentityConflict, messages[IdentityConflict])
	case errors.Is(err, domain.ErrReactivationImpossible):
		return New(ReactivationImpossible, messages[ReactivationImpossible])
	case errors.Is(err, domain.ErrInvalidCatalog):
		return New(InvalidCatalog, messages[InvalidCatalog])
	case errors.Is(err, domain.ErrInvalidLifecycle):
		return New(InvalidLifecycle, messages[InvalidLifecycle])
	case errors.Is(err, domain.ErrCodeImmutable):
		return New(ImmutableCode, messages[ImmutableCode])
	case errors.Is(err, domain.ErrCatalogInUse):
		return New(InUse, messages[InUse])
	case errors.Is(err, domain.ErrResourceIntegrity):
		return New(Integrity, messages[Integrity])
	case errors.Is(err, domain.ErrResourceValidation):
		return New(Validation, messages[Validation])
	case errors.Is(err, domain.ErrCatalogDuplicate), errors.Is(err, domain.ErrDuplicateResource):
		return New(Duplicate, messages[Duplicate])
	case errors.Is(err, domain.ErrCatalogReference), errors.Is(err, domain.ErrResourceReference):
		return New(InvalidReference, messages[InvalidReference])
	case errors.Is(err, domain.ErrCatalogRecordNotFound), errors.Is(err, domain.ErrResourceNotFound):
		return New(NotFound, messages[NotFound])
	case errors.Is(err, ErrInvalidArgument):
		return New(InvalidArgument, messages[InvalidArgument])
	case errors.Is(err, domain.ErrAttributeOrderUnavailable):
		return New(Unavailable, messages[Unavailable])
	case errors.Is(err, ErrUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return New(Unavailable, messages[Unavailable])
	default:
		return New(Internal, messages[Internal])
	}
}

// Code returns the stable GARFEX category for err.
func Code(err error) ErrorCode { return Map(err).Code() }

// IsCode reports whether err maps to the given stable category.
func IsCode(err error, code ErrorCode) bool { return Code(err) == code }

var messages = map[ErrorCode]string{
	InvalidArgument:        "the request argument is invalid",
	NotFound:               "the requested record was not found",
	Duplicate:              "the record already exists",
	InvalidReference:       "the referenced record is invalid",
	Validation:             "the request failed validation",
	Integrity:              "a persistence integrity failure occurred",
	IdentityConflict:       "the resource identity conflicts with the canonical identity",
	InvalidLifecycle:       "the operation is forbidden by the current lifecycle state",
	ReactivationImpossible: "the retained record cannot be reactivated under the current authority",
	InvalidCatalog:         "the catalog authority is invalid",
	InUse:                  "the record is still in use",
	ImmutableCode:          "the code cannot be changed",
	Conflict:               "the expected revision conflicts with the current state",
	Unavailable:            "the service is temporarily unavailable",
	Internal:               "an internal error occurred",
}

package purchasecore

import "errors"

// ErrorCode is a stable machine-readable Purchase Core outcome. Generic
// categories remain available for existing operations; line-resolution
// commands use the specific codes below so adapters never inspect messages.
type ErrorCode string

const (
	NotFound        ErrorCode = "NOT_FOUND"
	Validation      ErrorCode = "VALIDATION"
	Conflict        ErrorCode = "CONFLICT"
	InvalidArgument ErrorCode = "INVALID_ARGUMENT"
	Internal        ErrorCode = "INTERNAL"

	PurchaseLineNotFound           ErrorCode = "PURCHASE_LINE_NOT_FOUND"
	ResourceNotFound               ErrorCode = "RESOURCE_NOT_FOUND"
	ResourceInactive               ErrorCode = "RESOURCE_INACTIVE"
	CommercialSupplierSKURequired  ErrorCode = "COMMERCIAL_SUPPLIER_SKU_REQUIRED"
	CommercialSupplierSKUForbidden ErrorCode = "COMMERCIAL_SUPPLIER_SKU_FORBIDDEN"
	PurchaseLineStateConflict      ErrorCode = "PURCHASE_LINE_STATE_CONFLICT"
	StaleResolutionRevision        ErrorCode = "STALE_RESOLUTION_REVISION"
	StaleMappingRevision           ErrorCode = "STALE_MAPPING_REVISION"
	SupplierProductTargetConflict  ErrorCode = "SUPPLIER_PRODUCT_TARGET_CONFLICT"
	InvalidMappingTransition       ErrorCode = "INVALID_MAPPING_TRANSITION"
	IntegrityConflict              ErrorCode = "INTEGRITY_CONFLICT"
)

// Error is the only error type this package returns.
type Error struct {
	code    ErrorCode
	message string
}

func (e Error) Error() string   { return e.message }
func (e Error) Code() ErrorCode { return e.code }

// NewError returns an Error carrying code and message.
func NewError(code ErrorCode, message string) Error { return Error{code: code, message: message} }

// Code extracts the ErrorCode from err, returning Internal for any
// unclassified error and "" for a nil err.
func Code(err error) ErrorCode {
	if err == nil {
		return ""
	}
	var e Error
	if errors.As(err, &e) {
		return e.code
	}
	return Internal
}

// IsCode reports whether err classifies as code.
func IsCode(err error, code ErrorCode) bool { return Code(err) == code }

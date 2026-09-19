package purchasecore

import "errors"

// ErrorCode is one of five stable failure categories returned by this
// package, matching suppliercore's taxonomy.
type ErrorCode string

const (
	NotFound        ErrorCode = "NOT_FOUND"
	Validation      ErrorCode = "VALIDATION"
	Conflict        ErrorCode = "CONFLICT"
	InvalidArgument ErrorCode = "INVALID_ARGUMENT"
	Internal        ErrorCode = "INTERNAL"
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

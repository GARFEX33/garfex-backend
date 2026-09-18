package cfdicore

import "errors"

// ErrorCode is one of the stable failure categories returned by this package.
type ErrorCode string

const (
	// InvalidXML means the input is not well-formed XML.
	InvalidXML ErrorCode = "INVALID_XML"
	// NotCFDI means the document is well-formed XML but not a CFDI 4.x voucher.
	NotCFDI ErrorCode = "NOT_CFDI"
	// UnsupportedVersion means the voucher declares a CFDI version other than 4.0.
	UnsupportedVersion ErrorCode = "UNSUPPORTED_VERSION"
	// InvalidCFDI means the voucher is structurally a CFDI 4.0 but its content
	// is unusable, such as a missing issuer RFC or a malformed date.
	InvalidCFDI ErrorCode = "INVALID_CFDI"
	// Internal is returned by Code for any error this package did not classify.
	Internal ErrorCode = "INTERNAL"
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

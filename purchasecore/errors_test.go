package purchasecore

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewError_CarriesCodeAndMessage(t *testing.T) {
	err := NewError(NotFound, "purchase not found")
	if err.Code() != NotFound {
		t.Fatalf("Code() = %q, want %q", err.Code(), NotFound)
	}
	if err.Error() != "purchase not found" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "purchase not found")
	}
}

func TestCode_AllFiveCategoriesRoundTrip(t *testing.T) {
	codes := []ErrorCode{NotFound, Validation, Conflict, InvalidArgument, Internal}
	for _, code := range codes {
		err := NewError(code, "message")
		if got := Code(err); got != code {
			t.Errorf("Code(NewError(%q, ...)) = %q, want %q", code, got, code)
		}
		if !IsCode(err, code) {
			t.Errorf("IsCode(NewError(%q, ...), %q) = false, want true", code, code)
		}
	}
}

func TestCode_NilErrorReturnsEmptyCode(t *testing.T) {
	if got := Code(nil); got != "" {
		t.Fatalf("Code(nil) = %q, want empty", got)
	}
}

func TestCode_UnclassifiedErrorReturnsInternal(t *testing.T) {
	err := errors.New("some raw error")
	if got := Code(err); got != Internal {
		t.Fatalf("Code(raw error) = %q, want %q", got, Internal)
	}
}

func TestCode_WrappedErrorStillClassifies(t *testing.T) {
	base := NewError(Conflict, "conflict")
	wrapped := fmt.Errorf("operation failed: %w", base)
	if got := Code(wrapped); got != Conflict {
		t.Fatalf("Code(wrapped) = %q, want %q", got, Conflict)
	}
}

package postgres

import (
	"errors"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

func TestEncodeAttributeValuePreservesTypedPayloads(t *testing.T) {
	integer := int64(42)
	precise := decimal.RequireFromString("123.4500")
	boolean := true
	tests := []struct {
		name  string
		value domain.ResourceAttributeValue
		want  valuePayload
	}{{name: "option", value: domain.OptionValue("color", "NEGRO"), want: valuePayload{state: "SET", option: "NEGRO"}}, {name: "integer", value: domain.ResourceAttributeValue{AttributeCode: "count", Type: domain.ValueTypeInteger, Integer: &integer}, want: valuePayload{state: "SET", integer: &integer}}, {name: "decimal", value: domain.ResourceAttributeValue{AttributeCode: "ratio", Type: domain.ValueTypeDecimal, Decimal: &precise}, want: valuePayload{state: "SET", decimal: &precise}}, {name: "quantity", value: domain.QuantityValue("voltage", "600", "V"), want: valuePayload{state: "SET", quantity: decimal.RequireFromString("600"), quantityUnit: "V"}}, {name: "boolean", value: domain.ResourceAttributeValue{AttributeCode: "enabled", Type: domain.ValueTypeBoolean, Boolean: &boolean}, want: valuePayload{state: "SET", boolean: &boolean}}, {name: "text", value: domain.ControlledTextValue("label", "approved"), want: valuePayload{state: "SET", text: "approved"}}, {name: "not applicable", value: domain.ResourceAttributeValue{AttributeCode: "color", Type: domain.ValueTypeControlledOption, Text: notApplicableState}, want: valuePayload{state: notApplicableState}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeValue(tt.value)
			if err != nil {
				t.Fatalf("encodeValue() error = %v", err)
			}
			if got.state != tt.want.state || got.option != tt.want.option || got.integer != tt.want.integer || got.decimal == nil != (tt.want.decimal == nil) || (got.decimal != nil && !got.decimal.Equal(*tt.want.decimal)) || !got.quantity.Equal(tt.want.quantity) || got.quantityUnit != tt.want.quantityUnit || got.boolean != tt.want.boolean || got.text != tt.want.text {
				t.Fatalf("encodeValue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEncodeAttributeValueRejectsNotApplicableWithPayload(t *testing.T) {
	value := domain.ResourceAttributeValue{AttributeCode: "color", Type: domain.ValueTypeControlledOption, OptionCode: "NEGRO", Text: notApplicableState}
	if _, err := encodeValue(value); err == nil {
		t.Fatal("encodeValue() error = nil, want error for NOT_APPLICABLE payload")
	}
}

func TestMapRepositoryError(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want error
	}{{name: "duplicate", code: "23505", want: domain.ErrDuplicateResource}, {name: "attribute cardinality", code: "23505", want: domain.ErrResourceIntegrity}, {name: "foreign key", code: "23503", want: domain.ErrResourceReference}, {name: "check violation", code: "23514", want: domain.ErrResourceReference}} {
		t.Run(tt.name, func(t *testing.T) {
			constraint := "resources_test"
			message := "generic violation"
			if tt.code == "23505" {
				constraint = "recursos_class_id_identity_key"
				if tt.name == "attribute cardinality" {
					constraint = "resource_attribute_values_resource_id_resource_attribute_id_key"
				}
			}
			if tt.code == "23514" {
				message = "SET CONTROLLED_OPTION values require an official option"
			}
			err := mapRepositoryError(&pgconn.PgError{Code: tt.code, ConstraintName: constraint, Message: message})
			if !errors.Is(err, tt.want) {
				t.Fatalf("mapRepositoryError() = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestIdentityConflictNeverMatchesGenuineCardinalityCorruption is 5A's
// TRIANGULATE non-collapse evidence: a genuine persisted-cardinality
// corruption (the 23505 attribute-uniqueness constraint) keeps returning
// bare domain.ErrResourceIntegrity and is never accidentally reclassified as
// domain.ErrIdentityConflict — that dedicated outcome is reserved for a
// reactivation candidate's identity-v1 mismatch (internal/app/recursos),
// never for real PostgreSQL-level persistence corruption.
func TestIdentityConflictNeverMatchesGenuineCardinalityCorruption(t *testing.T) {
	err := mapRepositoryError(&pgconn.PgError{Code: "23505", ConstraintName: "resource_attribute_values_resource_id_resource_attribute_id_key", Message: "duplicate attribute row"})
	if !errors.Is(err, domain.ErrResourceIntegrity) {
		t.Fatalf("mapRepositoryError() = %v, want ErrResourceIntegrity", err)
	}
	if errors.Is(err, domain.ErrIdentityConflict) {
		t.Fatalf("mapRepositoryError() = %v, must not also match ErrIdentityConflict", err)
	}
}

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/core"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

const notApplicableState = "NOT_APPLICABLE"

type valuePayload struct {
	state        string
	option       string
	integer      *int64
	decimal      *decimal.Decimal
	quantity     decimal.Decimal
	quantityUnit string
	boolean      *bool
	text         string
}

func encodeValue(value domain.ResourceAttributeValue) (valuePayload, error) {
	if value.Text == notApplicableState {
		if hasPayload(value) {
			return valuePayload{}, fmt.Errorf("%w: NOT_APPLICABLE attribute %q has a payload", domain.ErrResourceReference, value.AttributeCode)
		}
		return valuePayload{state: notApplicableState}, nil
	}
	p := valuePayload{state: "SET"}
	switch value.Type {
	case domain.ValueTypeControlledOption:
		p.option = value.OptionCode
	case domain.ValueTypeInteger:
		p.integer = value.Integer
	case domain.ValueTypeDecimal:
		p.decimal = value.Decimal
	case domain.ValueTypeQuantity:
		if value.Quantity == nil {
			return valuePayload{}, errors.New("quantity payload is missing")
		}
		p.quantity, p.quantityUnit = value.Quantity.Value, value.Quantity.UnitCode
	case domain.ValueTypeBoolean:
		p.boolean = value.Boolean
	case domain.ValueTypeControlledText:
		p.text = value.Text
	default:
		return valuePayload{}, fmt.Errorf("unsupported value type %q", value.Type)
	}
	return p, nil
}

func hasPayload(value domain.ResourceAttributeValue) bool {
	return value.OptionCode != "" || value.Integer != nil || value.Decimal != nil || value.Quantity != nil || value.Boolean != nil || (value.Type == domain.ValueTypeControlledText && value.Text != notApplicableState)
}

func nullableQuantity(p valuePayload) any {
	if p.quantityUnit == "" {
		return nil
	}
	return p.quantity
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func decodeValue(code string, valueType domain.AttributeValueType, state string, option *string, integer *int64, dec, quantity, unit *string, boolean *bool, text *string) (domain.ResourceAttributeValue, error) {
	if state == notApplicableState {
		return domain.ResourceAttributeValue{AttributeCode: code, Type: valueType, Text: notApplicableState}, nil
	}
	value := domain.ResourceAttributeValue{AttributeCode: code, Type: valueType}
	switch valueType {
	case domain.ValueTypeControlledOption:
		value.OptionCode = dereference(option)
	case domain.ValueTypeInteger:
		value.Integer = integer
	case domain.ValueTypeDecimal:
		parsed, err := decimal.NewFromString(dereference(dec))
		if err != nil {
			return value, fmt.Errorf("decode decimal %q: %w", code, err)
		}
		value.Decimal = &parsed
	case domain.ValueTypeQuantity:
		parsed, err := decimal.NewFromString(dereference(quantity))
		if err != nil {
			return value, fmt.Errorf("decode quantity %q: %w", code, err)
		}
		value.Quantity = &domain.Quantity{Value: parsed, UnitCode: dereference(unit)}
	case domain.ValueTypeBoolean:
		value.Boolean = boolean
	case domain.ValueTypeControlledText:
		value.Text = dereference(text)
	default:
		return value, fmt.Errorf("decode unsupported value type %q", valueType)
	}
	return value, nil
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mapRepositoryError(err error) error {
	core.Record(context.Background(), core.Operation("resource.write"), "", 0, err)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			if pgErr.ConstraintName == "resource_attribute_values_resource_id_resource_attribute_id_key" {
				return fmt.Errorf("%w: %s", domain.ErrResourceIntegrity, pgErr.ConstraintName)
			}
			return fmt.Errorf("%w: %s", domain.ErrDuplicateResource, pgErr.ConstraintName)
		case "23503", "23514":
			return fmt.Errorf("%w: %s", domain.ErrResourceReference, pgErr.Message)
		}
	}
	return err
}

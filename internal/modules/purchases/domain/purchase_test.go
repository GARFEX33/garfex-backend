package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func validDraft() PurchaseDraft {
	return PurchaseDraft{
		Supplier:    PurchaseParty{SupplierID: 1},
		CFDIUUID:    "abc-123",
		Currency:    "mxn",
		IssuedAt:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		IssuerTaxID: "abc010101aaa",
		Total:       decimal.NewFromInt(100),
		Subtotal:    decimal.NewFromInt(100),
		XML: XMLDocument{
			Content: []byte("<xml/>"),
			Hash:    strings.Repeat("0", 64),
		},
		Lines: []PurchaseLineDraft{
			{LineNumber: 1, Description: "cable", Quantity: decimal.NewFromInt(1), UnitPrice: decimal.NewFromInt(100), Amount: decimal.NewFromInt(100)},
		},
	}
}

func TestNewPurchaseDraft(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d PurchaseDraft) PurchaseDraft
		wantErr bool
		field   string
	}{
		{name: "valid draft succeeds", mutate: func(d PurchaseDraft) PurchaseDraft { return d }},
		{name: "missing supplier id", mutate: func(d PurchaseDraft) PurchaseDraft { d.Supplier.SupplierID = 0; return d }, wantErr: true, field: "supplier_id"},
		{name: "negative branch id", mutate: func(d PurchaseDraft) PurchaseDraft { id := int64(-1); d.Supplier.BranchID = &id; return d }, wantErr: true, field: "branch_id"},
		{name: "blank uuid", mutate: func(d PurchaseDraft) PurchaseDraft { d.CFDIUUID = "  "; return d }, wantErr: true, field: "cfdi_uuid"},
		{name: "zero issued at", mutate: func(d PurchaseDraft) PurchaseDraft { d.IssuedAt = time.Time{}; return d }, wantErr: true, field: "issued_at"},
		{name: "blank currency", mutate: func(d PurchaseDraft) PurchaseDraft { d.Currency = ""; return d }, wantErr: true, field: "currency"},
		{name: "blank issuer tax id", mutate: func(d PurchaseDraft) PurchaseDraft { d.IssuerTaxID = " "; return d }, wantErr: true, field: "issuer_tax_id"},
		{name: "empty xml content", mutate: func(d PurchaseDraft) PurchaseDraft { d.XML.Content = nil; return d }, wantErr: true, field: "xml_content"},
		{name: "short xml hash", mutate: func(d PurchaseDraft) PurchaseDraft { d.XML.Hash = "abc"; return d }, wantErr: true, field: "xml_hash"},
		{name: "negative subtotal", mutate: func(d PurchaseDraft) PurchaseDraft { d.Subtotal = decimal.NewFromInt(-1); return d }, wantErr: true, field: "subtotal"},
		{name: "negative total", mutate: func(d PurchaseDraft) PurchaseDraft { d.Total = decimal.NewFromInt(-1); return d }, wantErr: true, field: "total"},
		{name: "no lines", mutate: func(d PurchaseDraft) PurchaseDraft { d.Lines = nil; return d }, wantErr: true, field: "lines"},
		{name: "invalid line propagates", mutate: func(d PurchaseDraft) PurchaseDraft { d.Lines[0].Description = ""; return d }, wantErr: true, field: "lines.description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPurchaseDraft(tt.mutate(validDraft()))
			if tt.wantErr {
				var verr ValidationError
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !isValidationError(err, &verr) {
					t.Fatalf("error = %v, want ValidationError", err)
				}
				if verr.Field != tt.field {
					t.Fatalf("field = %q, want %q", verr.Field, tt.field)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewPurchaseDraftNormalizesUUIDAndCurrency(t *testing.T) {
	draft, err := NewPurchaseDraft(validDraft())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if draft.CFDIUUID != "ABC-123" {
		t.Fatalf("cfdi uuid = %q, want normalized upper case", draft.CFDIUUID)
	}
	if draft.Currency != "MXN" {
		t.Fatalf("currency = %q, want normalized upper case", draft.Currency)
	}
}

func TestPurchaseSameRelevantContent(t *testing.T) {
	draft, err := NewPurchaseDraft(validDraft())
	if err != nil {
		t.Fatalf("build draft: %v", err)
	}
	existing := Purchase{
		Series: draft.Series, Folio: draft.Folio, Currency: draft.Currency, IssuerTaxID: draft.IssuerTaxID,
		IssuedAt: draft.IssuedAt, Subtotal: draft.Subtotal, Discount: draft.Discount,
		TaxTransferred: draft.TaxTransferred, TaxWithheld: draft.TaxWithheld, Total: draft.Total,
	}
	if !existing.SameRelevantContent(draft) {
		t.Fatal("expected identical content to match")
	}
	existing.Total = existing.Total.Add(decimal.NewFromInt(1))
	if existing.SameRelevantContent(draft) {
		t.Fatal("expected different total to not match")
	}
}

func TestSameLines(t *testing.T) {
	draft, err := NewPurchaseDraft(validDraft())
	if err != nil {
		t.Fatalf("build draft: %v", err)
	}
	existing := []PurchaseLine{{
		LineNumber: draft.Lines[0].LineNumber, Description: draft.Lines[0].Description, SupplierSKU: draft.Lines[0].SupplierSKU,
		Quantity: draft.Lines[0].Quantity, UnitPrice: draft.Lines[0].UnitPrice, Amount: draft.Lines[0].Amount,
	}}
	if !SameLines(existing, draft.Lines) {
		t.Fatal("expected identical lines to match")
	}
	if SameLines(existing, nil) {
		t.Fatal("expected mismatched line count to not match")
	}
	existing[0].Amount = existing[0].Amount.Add(decimal.NewFromInt(1))
	if SameLines(existing, draft.Lines) {
		t.Fatal("expected different amount to not match")
	}
}

func isValidationError(err error, target *ValidationError) bool {
	verr, ok := err.(ValidationError)
	if !ok {
		return false
	}
	*target = verr
	return true
}

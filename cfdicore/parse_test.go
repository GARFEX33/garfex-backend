package cfdicore

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestParseExtractsEveryField(t *testing.T) {
	got, err := Parse(readFixture(t, "invoice.xml"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := Invoice{
		Version:           "4.0",
		Series:            "AB",
		Folio:             "1234",
		IssuedAt:          time.Date(2026, 3, 5, 9, 30, 15, 0, time.UTC),
		Seal:              "SELLO-CFD",
		PaymentForm:       "03",
		CertificateNumber: "00001000000700000001",
		Certificate:       "CERT-BASE64",
		PaymentConditions: "Contado",
		Subtotal:          "1200.50",
		Discount:          "200.50",
		Currency:          "MXN",
		ExchangeRate:      "1",
		Total:             "1160.00",
		VoucherType:       "I",
		Export:            "01",
		PaymentMethod:     "PUE",
		IssuePlace:        "64000",
		Related: []RelatedCFDIs{{
			RelationType: "04",
			UUIDs:        []string{"11111111-2222-3333-4444-555555555555", "66666666-7777-8888-9999-000000000000"},
		}},
		Issuer: Issuer{TaxID: "ABC010101AA1", Name: "PROVEEDOR DE PRUEBA SA DE CV", TaxRegime: "601"},
		Receiver: Receiver{
			TaxID: "XAXX010101000", Name: "CLIENTE DE PRUEBA",
			PostalCode: "64010", TaxRegime: "612", CFDIUse: "G03",
		},
		Concepts: []Concept{
			{
				ProductServiceCode: "26121600", ItemNumber: "CAB-001",
				Quantity: "100.00", UnitCode: "MTR", Unit: "mts",
				Description: "CABLE THW CAL. 12 NEGRO",
				UnitPrice:   "10.00", Amount: "1000.00", Discount: "100.00", TaxObject: "02",
				Taxes: ConceptTaxes{
					Transferred: []Tax{{Base: "900.00", Tax: "002", FactorType: "Tasa", Rate: "0.160000", Amount: "144.00"}},
					Withheld:    []Tax{{Base: "900.00", Tax: "001", FactorType: "Tasa", Rate: "0.100000", Amount: "90.00"}},
				},
			},
			{
				ProductServiceCode: "39121700", Quantity: "2", UnitCode: "H87",
				Description: "CONECTOR", UnitPrice: "100.25", Amount: "200.50", TaxObject: "01",
			},
		},
		Taxes: Taxes{
			TotalTransferred: "144.00",
			TotalWithheld:    "90.00",
			Transferred:      []Tax{{Base: "900.00", Tax: "002", FactorType: "Tasa", Rate: "0.160000", Amount: "144.00"}},
			Withheld:         []Tax{{Tax: "001", Amount: "90.00"}},
		},
		Stamp: &Stamp{
			Version:              "1.1",
			UUID:                 "ABCDEF12-3456-7890-ABCD-EF1234567890",
			StampedAt:            time.Date(2026, 3, 5, 9, 30, 20, 0, time.UTC),
			ProviderTaxID:        "SPR190613I52",
			CFDSeal:              "SELLO-CFD",
			SATCertificateNumber: "00001000000700000002",
			SATSeal:              "SELLO-SAT",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestParseToleratesBOMPrefixAndAttributeOrder(t *testing.T) {
	doc := "\xef\xbb\xbf" + `<?xml version="1.0" encoding="utf-8"?>` +
		`<c:Comprobante xmlns:c="http://www.sat.gob.mx/cfd/4" Total="10.00" Version="4.0" SubTotal="10.00">` +
		`<c:Emisor Nombre="ACME" Rfc="acm010101aa1" RegimenFiscal="601"/>` +
		`<c:Receptor Rfc="XAXX010101000"/>` +
		`</c:Comprobante>`

	got, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Issuer.TaxID != "ACM010101AA1" || got.Issuer.Name != "ACME" || got.Total != "10.00" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if got.Stamp != nil {
		t.Fatalf("Stamp = %#v, want nil when the complement is absent", got.Stamp)
	}
	if len(got.Concepts) != 0 {
		t.Fatalf("Concepts = %#v, want empty", got.Concepts)
	}
}

func TestParseRejects(t *testing.T) {
	const ns = `xmlns:cfdi="http://www.sat.gob.mx/cfd/4"`
	tests := []struct {
		name string
		doc  string
		code ErrorCode
	}{
		{"empty input", ``, InvalidXML},
		{"not xml", `this is not xml`, InvalidXML},
		{"truncated xml", `<cfdi:Comprobante ` + ns + ` Version="4.0"><cfdi:Emisor`, InvalidXML},
		{"other root element", `<invoice/>`, NotCFDI},
		{"wrong namespace", `<cfdi:Comprobante xmlns:cfdi="http://example.com/x" Version="4.0"/>`, NotCFDI},
		{"cfdi 3.3", `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/3" Version="3.3"/>`, NotCFDI},
		{"unsupported version", `<cfdi:Comprobante ` + ns + ` Version="5.0"/>`, UnsupportedVersion},
		{"missing version", `<cfdi:Comprobante ` + ns + `/>`, UnsupportedVersion},
		{"missing issuer", `<cfdi:Comprobante ` + ns + ` Version="4.0"><cfdi:Receptor Rfc="X"/></cfdi:Comprobante>`, InvalidCFDI},
		{"blank issuer tax id", `<cfdi:Comprobante ` + ns + ` Version="4.0"><cfdi:Emisor Rfc="  " Nombre="A"/></cfdi:Comprobante>`, InvalidCFDI},
		{"bad issue date", `<cfdi:Comprobante ` + ns + ` Version="4.0" Fecha="yesterday"><cfdi:Emisor Rfc="A"/></cfdi:Comprobante>`, InvalidCFDI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.doc))
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if got := Code(err); got != tt.code {
				t.Fatalf("Code = %q (%v), want %q", got, err, tt.code)
			}
			if strings.Contains(err.Error(), "\n") {
				t.Fatalf("error message must be single-line: %q", err)
			}
		})
	}
}

func TestParseDoesNotMutateInput(t *testing.T) {
	data := readFixture(t, "invoice.xml")
	before := string(data)
	if _, err := Parse(data); err != nil {
		t.Fatal(err)
	}
	if string(data) != before {
		t.Fatal("Parse mutated its input")
	}
}

func TestCodeClassification(t *testing.T) {
	if Code(nil) != "" {
		t.Fatal("Code(nil) must be empty")
	}
	if !IsCode(NewError(NotCFDI, "x"), NotCFDI) {
		t.Fatal("IsCode must match")
	}
	if Code(os.ErrNotExist) != Internal {
		t.Fatalf("unclassified errors must classify as %q", Internal)
	}
}

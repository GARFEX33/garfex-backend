package cfdicore_test

import (
	"bytes"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GARFEX33/garfex-costos-unitarios/cfdicore"
)

// samplesGlob points at real supplier invoices kept out of git; the test
// skips when they are not present on the machine.
const samplesGlob = "../docs/ejemplo xlm/*.xml"

func rat(t *testing.T, s string) *big.Rat {
	t.Helper()
	if s == "" {
		return new(big.Rat)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatalf("not a decimal: %q", s)
	}
	return r
}

func TestParseRealSamples(t *testing.T) {
	files, err := filepath.Glob(samplesGlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no sample CFDI files present")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			inv, err := cfdicore.Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if want := bytes.Count(data, []byte("<cfdi:Concepto ")); len(inv.Concepts) != want || want == 0 {
				t.Fatalf("concepts = %d, want %d (>0)", len(inv.Concepts), want)
			}
			if inv.Issuer.TaxID == "" || inv.Issuer.Name == "" || inv.Issuer.TaxRegime == "" {
				t.Fatalf("issuer incomplete: %#v", inv.Issuer)
			}
			if inv.Issuer.TaxID != strings.ToUpper(inv.Issuer.TaxID) {
				t.Fatalf("issuer tax id not normalized: %q", inv.Issuer.TaxID)
			}
			if inv.Stamp == nil || inv.Stamp.UUID != strings.ToUpper(inv.Stamp.UUID) || inv.Stamp.UUID == "" {
				t.Fatalf("stamp missing or UUID not normalized: %#v", inv.Stamp)
			}
			if !strings.Contains(strings.ToUpper(filepath.Base(file)), inv.Stamp.UUID) {
				t.Fatalf("stamp UUID %q does not match file name %q", inv.Stamp.UUID, filepath.Base(file))
			}
			if inv.IssuedAt.IsZero() || inv.Currency == "" || inv.Total == "" {
				t.Fatalf("voucher header incomplete: %#v", inv)
			}

			// Accounting invariants, computed exactly with rationals.
			sum := new(big.Rat)
			for _, c := range inv.Concepts {
				if c.Description == "" || c.Quantity == "" || c.UnitPrice == "" || c.Amount == "" {
					t.Fatalf("concept incomplete: %#v", c)
				}
				sum.Add(sum, rat(t, c.Amount))
			}
			if sum.Cmp(rat(t, inv.Subtotal)) != 0 {
				t.Fatalf("sum of concept amounts %s != subtotal %s", sum.FloatString(2), inv.Subtotal)
			}
			total := new(big.Rat).Sub(rat(t, inv.Subtotal), rat(t, inv.Discount))
			total.Add(total, rat(t, inv.Taxes.TotalTransferred))
			total.Sub(total, rat(t, inv.Taxes.TotalWithheld))
			if total.Cmp(rat(t, inv.Total)) != 0 {
				t.Fatalf("subtotal-discount+taxes = %s != total %s", total.FloatString(2), inv.Total)
			}
		})
	}
}

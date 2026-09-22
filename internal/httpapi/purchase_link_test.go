package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/purchasecore"
)

func TestSupplierProductMappingCommands(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		body      string
		configure func(*purchaseWriterFuncs, *purchasecore.MappingDecisionMetadata)
	}{
		{
			name: "confirm",
			path: "/v1/supplier-products/9/mapping/confirm",
			body: `{"actor":"tester","reason":"initial review","resourceId":"42","expectedRevision":"3"}`,
			configure: func(writer *purchaseWriterFuncs, got *purchasecore.MappingDecisionMetadata) {
				writer.confirmMapping = func(_ context.Context, req purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
					*got = req.Decision
					if req.SupplierProductID != 9 || req.ResourceID != 42 || req.ExpectedRevision != 3 {
						t.Errorf("request = %#v", req)
					}
					return sampleSupplierProduct, nil
				}
			},
		},
		{
			name: "correct",
			path: "/v1/supplier-products/9/mapping/correct",
			body: `{"actor":"tester","reason":"correction","expectedCurrentResourceId":"41","resourceId":"42","expectedRevision":"3"}`,
			configure: func(writer *purchaseWriterFuncs, got *purchasecore.MappingDecisionMetadata) {
				writer.correctMapping = func(_ context.Context, req purchasecore.CorrectMappingRequest) (purchasecore.SupplierProduct, error) {
					*got = req.Decision
					if req.SupplierProductID != 9 || req.ExpectedCurrentResourceID != 41 || req.ResourceID != 42 || req.ExpectedRevision != 3 {
						t.Errorf("request = %#v", req)
					}
					return sampleSupplierProduct, nil
				}
			},
		},
		{
			name: "retire",
			path: "/v1/supplier-products/9/mapping/retire",
			body: `{"actor":"tester","reason":"retire mapping","expectedCurrentResourceId":"42","expectedRevision":"3"}`,
			configure: func(writer *purchaseWriterFuncs, got *purchasecore.MappingDecisionMetadata) {
				writer.exceptionalUnlink = func(_ context.Context, req purchasecore.ExceptionalUnlinkRequest) (purchasecore.SupplierProduct, error) {
					*got = req.Decision
					if req.SupplierProductID != 9 || req.ExpectedCurrentResourceID != 42 || req.ExpectedRevision != 3 {
						t.Errorf("request = %#v", req)
					}
					return sampleSupplierProduct, nil
				}
			},
		},
		{
			name: "report conflict",
			path: "/v1/supplier-products/9/mapping/report-conflict",
			body: `{"actor":"tester","reason":"duplicate identity","expectedCurrentResourceId":"42","expectedRevision":"3"}`,
			configure: func(writer *purchaseWriterFuncs, got *purchasecore.MappingDecisionMetadata) {
				writer.reportIdentityConflict = func(_ context.Context, req purchasecore.ReportIdentityConflictRequest) (purchasecore.SupplierProduct, error) {
					*got = req.Decision
					if req.SupplierProductID != 9 || req.ExpectedCurrentResourceID != 42 || req.ExpectedRevision != 3 {
						t.Errorf("request = %#v", req)
					}
					return sampleSupplierProduct, nil
				}
			},
		},
		{
			name: "resolve conflict",
			path: "/v1/supplier-products/9/mapping/resolve-conflict",
			body: `{"actor":"tester","reason":"selected canonical","expectedCurrentResourceId":"42","resourceId":"43","expectedRevision":"3"}`,
			configure: func(writer *purchaseWriterFuncs, got *purchasecore.MappingDecisionMetadata) {
				writer.resolveIdentityConflict = func(_ context.Context, req purchasecore.ResolveIdentityConflictRequest) (purchasecore.SupplierProduct, error) {
					*got = req.Decision
					if req.SupplierProductID != 9 || req.ExpectedCurrentResourceID != 42 || req.ResourceID != 43 || req.ExpectedRevision != 3 {
						t.Errorf("request = %#v", req)
					}
					return sampleSupplierProduct, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var writer purchaseWriterFuncs
			var decision purchasecore.MappingDecisionMetadata
			tt.configure(&writer, &decision)
			h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			if decision.Actor != "tester" || decision.Reason == "" || decision.Origin != purchasecore.MappingOriginManual || decision.At.IsZero() || decision.At.Location() != time.UTC {
				t.Fatalf("decision = %#v", decision)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["id"] != "9" {
				t.Fatalf("body = %#v", body)
			}
		})
	}
}

func TestSupplierProductMappingRejectsInvalidRevision(t *testing.T) {
	called := false
	writer := purchaseWriterFuncs{confirmMapping: func(context.Context, purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
		called = true
		return sampleSupplierProduct, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/supplier-products/9/mapping/confirm", strings.NewReader(`{"actor":"tester","reason":"review","resourceId":"42","expectedRevision":"bad"}`)))
	if rec.Code != http.StatusBadRequest || called {
		t.Fatalf("status/called = %d/%t, want %d/false, body = %s", rec.Code, called, http.StatusBadRequest, rec.Body)
	}
}

func TestSupplierProductMappingPropagatesCoreErrorCode(t *testing.T) {
	writer := purchaseWriterFuncs{confirmMapping: func(context.Context, purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
		return purchasecore.SupplierProduct{}, purchasecore.NewError(purchasecore.StaleMappingRevision, "mapping revision is stale")
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/supplier-products/9/mapping/confirm", strings.NewReader(`{"actor":"tester","reason":"review","resourceId":"42","expectedRevision":"3"}`)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != string(purchasecore.StaleMappingRevision) {
		t.Fatalf("body = %#v", body)
	}
}

func TestResolvePurchaseLineSupportsNullableAndExistingSnapshots(t *testing.T) {
	productID := int64(9)
	mappingRevision := purchasecore.MappingRevision(7)
	writer := purchaseWriterFuncs{resolvePurchaseLine: func(_ context.Context, req purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error) {
		if req.LineID != 11 || req.ResourceID != 42 || req.ExpectedResolutionRevision != 4 || req.Actor != "tester" || req.Reason != "resolve" {
			t.Errorf("request = %#v", req)
		}
		return purchasecore.ResolvePurchaseLineResult{
			Line: purchasecore.PurchaseLine{ID: req.LineID, SupplierProductID: &productID, ResolutionRevision: 5, EffectiveStatus: purchasecore.LinkLinked},
			SupplierProduct: purchasecore.SupplierProduct{
				ID: 9, SupplierID: 3, SupplierSKU: "COMM-9", MappingRevision: mappingRevision,
				CurrentMapping: purchasecore.SupplierProductMapping{ResourceID: &req.ResourceID},
			},
			CommercialIdentityDisposition: purchasecore.CommercialIdentityAlreadyMapped,
		}, nil
	}}
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "null snapshot", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":null,"expectedMappingRevision":null,"expectedResolutionRevision":"4","commercialSupplierSku":"COMM-9"}`},
		{name: "existing snapshot", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":"9","expectedMappingRevision":"7","expectedResolutionRevision":"4","commercialSupplierSku":""}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got purchasecore.ResolvePurchaseLineRequest
			checkWriter := writer
			checkWriter.resolvePurchaseLine = func(ctx context.Context, req purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error) {
				got = req
				return writer.resolvePurchaseLine(ctx, req)
			}
			h := NewRouter(nil, nil, nil, nil, nil, nil, nil, checkWriter)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines/11/resolve", strings.NewReader(test.body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			if test.name == "null snapshot" && (got.ExpectedSupplierProductID != nil || got.ExpectedMappingRevision != nil) {
				t.Fatalf("nullable snapshot = %#v", got)
			}
			if test.name == "existing snapshot" && (got.ExpectedSupplierProductID == nil || *got.ExpectedSupplierProductID != productID || got.ExpectedMappingRevision == nil || *got.ExpectedMappingRevision != mappingRevision) {
				t.Fatalf("existing snapshot = %#v", got)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			identity := body["commercialIdentity"].(map[string]any)
			if identity["disposition"] != string(purchasecore.CommercialIdentityAlreadyMapped) || identity["supplierProductId"] != "9" || identity["supplierId"] != "3" || identity["commercialSupplierSku"] != "COMM-9" || identity["mappingRevision"] != "7" || identity["resourceId"] != "42" {
				t.Fatalf("commercial identity = %#v", identity)
			}
		})
	}
}

func TestResolvePurchaseLinePropagatesCoreError(t *testing.T) {
	writer := purchaseWriterFuncs{resolvePurchaseLine: func(context.Context, purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error) {
		return purchasecore.ResolvePurchaseLineResult{}, purchasecore.NewError(purchasecore.PurchaseLineStateConflict, "line is not resolvable")
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	body := `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":null,"expectedMappingRevision":null,"expectedResolutionRevision":"4","commercialSupplierSku":"COMM-9"}`
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines/11/resolve", strings.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body)
	}
}

func TestSetResolutionOverride(t *testing.T) {
	var got purchasecore.SetResolutionOverrideRequest
	writer := purchaseWriterFuncs{setResolutionOverride: func(_ context.Context, req purchasecore.SetResolutionOverrideRequest) (purchasecore.PurchaseLine, error) {
		got = req
		return purchasecore.PurchaseLine{ID: req.LineID, ResolutionRevision: req.ExpectedRevision + 1, ResolutionOverride: req.Override, EffectiveStatus: req.Override}, nil
	}}
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, writer)
	rec := httptest.NewRecorder()
	body := `{"actor":"tester","reason":"manual exception","override":"NO_APLICA","expectedRevision":"8"}`
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines/11/resolution-override", strings.NewReader(body)))
	if rec.Code != http.StatusOK || got.LineID != 11 || got.Override != purchasecore.LinkNotApplicable || got.ExpectedRevision != 8 || got.Actor != "tester" || got.Reason != "manual exception" {
		t.Fatalf("status = %d, request = %#v, body = %s", rec.Code, got, rec.Body)
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["resolutionOverride"] != "NO_APLICA" || response["effectiveStatus"] != "NO_APLICA" {
		t.Fatalf("response = %#v", response)
	}
}

func TestPurchaseMutationRoutesRejectWrongMethods(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	paths := []string{
		"/v1/supplier-products/9/mapping/confirm",
		"/v1/supplier-products/9/mapping/correct",
		"/v1/supplier-products/9/mapping/retire",
		"/v1/supplier-products/9/mapping/report-conflict",
		"/v1/supplier-products/9/mapping/resolve-conflict",
		"/v1/purchase-lines/11/resolve",
		"/v1/purchase-lines/11/resolution-override",
	}
	for _, path := range paths {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestLegacyPurchaseMutationRoutesNotFound(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, purchaseWriterFuncs{})
	for _, path := range []string{
		"/v1/supplier-products/9/link",
		"/v1/supplier-products/9/unlink",
		"/v1/purchase-lines/11/link-status",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestPurchaseMutationOpenAPISchemasMatchRuntimeContracts(t *testing.T) {
	line := map[string]any{
		"id": "11", "purchaseId": "7", "lineNumber": 1, "description": "CABLE", "supplierSku": "XML-1",
		"satProductCode": "3912", "quantity": "3", "unitCode": "H87", "unit": "PIEZA", "unitPrice": "10.00", "amount": "30.00",
		"discount": "0", "taxTransferred": "4.80", "taxWithheld": "0", "taxObject": "02", "supplierProductId": "9",
		"resolutionRevision": "5", "resolutionOverride": "NONE", "effectiveStatus": "VINCULADO", "effectiveCause": "NONE",
	}
	product := map[string]any{
		"id": "9", "supplierId": "3", "supplierSku": "COMM-9", "description": "CABLE", "resourceId": "42", "mappingRevision": "7",
		"resourceActive": true, "mappingState": "CONFIRMED", "mappingCause": "NONE", "notes": "", "createdAt": "2026-03-05T10:00:00Z", "updatedAt": "2026-03-05T10:00:00Z",
	}
	for name, value := range map[string]any{
		"ConfirmMappingRequest":         map[string]any{"actor": "tester", "reason": "confirm", "resourceId": "42", "expectedRevision": "3"},
		"CorrectMappingRequest":         map[string]any{"actor": "tester", "reason": "correct", "expectedCurrentResourceId": "41", "resourceId": "42", "expectedRevision": "3"},
		"RetireMappingRequest":          map[string]any{"actor": "tester", "reason": "retire", "expectedCurrentResourceId": "42", "expectedRevision": "3"},
		"ReportMappingConflictRequest":  map[string]any{"actor": "tester", "reason": "conflict", "expectedCurrentResourceId": "42", "expectedRevision": "3"},
		"ResolveMappingConflictRequest": map[string]any{"actor": "tester", "reason": "resolve", "expectedCurrentResourceId": "42", "resourceId": "43", "expectedRevision": "3"},
		"ResolvePurchaseLineRequest":    map[string]any{"actor": "tester", "reason": "resolve", "resourceId": "42", "expectedSupplierProductId": nil, "expectedMappingRevision": nil, "expectedResolutionRevision": "4", "commercialSupplierSku": "COMM-9"},
		"ResolutionOverrideRequest":     map[string]any{"actor": "tester", "reason": "exception", "override": "CONFLICTO", "expectedRevision": "8"},
		"SupplierProduct":               product,
		"PurchaseLine":                  line,
		"ResolvePurchaseLineResponse":   map[string]any{"line": line, "supplierProduct": product, "commercialIdentity": map[string]any{"supplierProductId": "9", "supplierId": "3", "commercialSupplierSku": "COMM-9", "disposition": "ALREADY_MAPPED", "mappingRevision": "7", "resourceId": "42"}},
	} {
		t.Run(name, func(t *testing.T) {
			validateSchemaJSON(t, catalogSchema(t, name), value)
		})
	}
	if err := catalogSchema(t, "ConfirmMappingRequest").VisitJSON(map[string]any{"actor": "tester", "reason": "confirm", "resourceId": "42", "expectedRevision": "3", "extra": true}); err == nil {
		t.Fatal("ConfirmMappingRequest accepted an extra property")
	}
}

func TestPurchaseMutationRoutesRejectMissingWriter(t *testing.T) {
	h := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil)
	tests := []struct {
		path string
		body string
	}{
		{"/v1/supplier-products/9/mapping/confirm", `{"actor":"tester","reason":"x","resourceId":"1","expectedRevision":"0"}`},
		{"/v1/supplier-products/9/mapping/correct", `{"actor":"tester","reason":"x","expectedCurrentResourceId":"1","resourceId":"2","expectedRevision":"0"}`},
		{"/v1/supplier-products/9/mapping/retire", `{"actor":"tester","reason":"x","expectedCurrentResourceId":"1","expectedRevision":"0"}`},
		{"/v1/supplier-products/9/mapping/report-conflict", `{"actor":"tester","reason":"x","expectedCurrentResourceId":"1","expectedRevision":"0"}`},
		{"/v1/supplier-products/9/mapping/resolve-conflict", `{"actor":"tester","reason":"x","expectedCurrentResourceId":"1","resourceId":"2","expectedRevision":"0"}`},
		{"/v1/purchase-lines/11/resolve", `{"actor":"tester","reason":"x","resourceId":"1","expectedResolutionRevision":"0","commercialSupplierSku":"SKU"}`},
		{"/v1/purchase-lines/11/resolution-override", `{"actor":"tester","reason":"x","override":"NONE","expectedRevision":"0"}`},
	}
	for _, test := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want %d, body = %s", test.path, rec.Code, http.StatusInternalServerError, rec.Body)
		}
	}
}

func TestPurchaseMutationRequestsRejectClosedShapeViolations(t *testing.T) {
	resolveCalls := 0
	writer := purchaseWriterFuncs{
		confirmMapping: func(context.Context, purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
			t.Fatal("confirm writer called for invalid request")
			return purchasecore.SupplierProduct{}, nil
		},
		resolvePurchaseLine: func(context.Context, purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error) {
			resolveCalls++
			return purchasecore.ResolvePurchaseLineResult{}, nil
		},
	}
	validResolve := `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":null,"expectedMappingRevision":null,"expectedResolutionRevision":"4","commercialSupplierSku":"COMM-9"}`
	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{name: "unknown field", path: "/v1/supplier-products/9/mapping/confirm", body: `{"actor":"tester","reason":"confirm","resourceId":"42","expectedRevision":"3","expectedCurrentResourceId":"41"}`, want: http.StatusBadRequest},
		{name: "action inapplicable field", path: "/v1/supplier-products/9/mapping/confirm", body: `{"actor":"tester","reason":"confirm","resourceId":"42","expectedRevision":"3","override":"NONE"}`, want: http.StatusBadRequest},
		{name: "trailing JSON", path: "/v1/supplier-products/9/mapping/confirm", body: `{"actor":"tester","reason":"confirm","resourceId":"42","expectedRevision":"3"}{}`, want: http.StatusBadRequest},
		{name: "omitted resolve snapshots", path: "/v1/purchase-lines/11/resolve", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedResolutionRevision":"4","commercialSupplierSku":"COMM-9"}`, want: http.StatusBadRequest},
		{name: "only one resolve snapshot null", path: "/v1/purchase-lines/11/resolve", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":null,"expectedMappingRevision":"7","expectedResolutionRevision":"4","commercialSupplierSku":"COMM-9"}`, want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewRouter(nil, nil, nil, nil, nil, nil, nil, writer).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
			if rec.Code != test.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, test.want, rec.Body)
			}
		})
	}

	rec := httptest.NewRecorder()
	NewRouter(nil, nil, nil, nil, nil, nil, nil, writer).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/purchase-lines/11/resolve", strings.NewReader(validResolve)))
	if rec.Code != http.StatusOK || resolveCalls != 1 {
		t.Fatalf("explicit null snapshots status/calls = %d/%d, want %d/1", rec.Code, resolveCalls, http.StatusOK)
	}
}

func TestPurchaseMutationRejectsInvalidDecimalLexemes(t *testing.T) {
	called := false
	writer := purchaseWriterFuncs{confirmMapping: func(context.Context, purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
		called = true
		return sampleSupplierProduct, nil
	}}
	for _, value := range []string{"01", "+1", "18446744073709551616"} {
		t.Run(value, func(t *testing.T) {
			called = false
			rec := httptest.NewRecorder()
			body := `{"actor":"tester","reason":"confirm","resourceId":"42","expectedRevision":"` + value + `"}`
			NewRouter(nil, nil, nil, nil, nil, nil, nil, writer).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/supplier-products/9/mapping/confirm", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest || called {
				t.Fatalf("status/called = %d/%t, want %d/false, body = %s", rec.Code, called, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestPurchaseMutationOpenAPISchemasRejectInvalidNumericLexemes(t *testing.T) {
	invalid := []struct {
		name   string
		schema string
		value  map[string]any
	}{
		{name: "positive resource id zero", schema: "ConfirmMappingRequest", value: map[string]any{"actor": "tester", "reason": "confirm", "resourceId": "0", "expectedRevision": "0"}},
		{name: "positive resource id overflow", schema: "ConfirmMappingRequest", value: map[string]any{"actor": "tester", "reason": "confirm", "resourceId": "92233720368547758080", "expectedRevision": "0"}},
		{name: "revision leading zero", schema: "ConfirmMappingRequest", value: map[string]any{"actor": "tester", "reason": "confirm", "resourceId": "42", "expectedRevision": "01"}},
		{name: "commercial identity revision overflow", schema: "CommercialIdentity", value: map[string]any{"supplierProductId": "9", "supplierId": "3", "commercialSupplierSku": "COMM-9", "disposition": "ALREADY_MAPPED", "mappingRevision": "184467440737095516160", "resourceId": "42"}},
		{name: "resolve snapshot id zero", schema: "ResolvePurchaseLineRequest", value: map[string]any{"actor": "tester", "reason": "resolve", "resourceId": "42", "expectedSupplierProductId": "0", "expectedMappingRevision": "7", "expectedResolutionRevision": "4", "commercialSupplierSku": ""}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := catalogSchema(t, test.schema).VisitJSON(test.value); err == nil {
				t.Fatalf("%s accepted invalid numeric lexeme", test.schema)
			}
		})
	}
}

func TestPurchaseMutationRejectsExactNumericBoundaryOverflow(t *testing.T) {
	const (
		signedInt64Overflow   = "9223372036854775808"
		unsignedInt64Overflow = "18446744073709551616"
	)
	validResolve := `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":null,"expectedMappingRevision":null,"expectedResolutionRevision":"0","commercialSupplierSku":"COMM-9"}`
	writer := purchaseWriterFuncs{
		confirmMapping: func(context.Context, purchasecore.ConfirmMappingRequest) (purchasecore.SupplierProduct, error) {
			t.Fatal("confirm mapping writer called for an overflowing value")
			return purchasecore.SupplierProduct{}, nil
		},
		correctMapping: func(context.Context, purchasecore.CorrectMappingRequest) (purchasecore.SupplierProduct, error) {
			t.Fatal("correct mapping writer called for an overflowing value")
			return purchasecore.SupplierProduct{}, nil
		},
		exceptionalUnlink: func(context.Context, purchasecore.ExceptionalUnlinkRequest) (purchasecore.SupplierProduct, error) {
			t.Fatal("retire mapping writer called for an overflowing value")
			return purchasecore.SupplierProduct{}, nil
		},
		reportIdentityConflict: func(context.Context, purchasecore.ReportIdentityConflictRequest) (purchasecore.SupplierProduct, error) {
			t.Fatal("report conflict writer called for an overflowing value")
			return purchasecore.SupplierProduct{}, nil
		},
		resolveIdentityConflict: func(context.Context, purchasecore.ResolveIdentityConflictRequest) (purchasecore.SupplierProduct, error) {
			t.Fatal("resolve conflict writer called for an overflowing value")
			return purchasecore.SupplierProduct{}, nil
		},
		resolvePurchaseLine: func(context.Context, purchasecore.ResolvePurchaseLineRequest) (purchasecore.ResolvePurchaseLineResult, error) {
			t.Fatal("resolve purchase line writer called for an overflowing value")
			return purchasecore.ResolvePurchaseLineResult{}, nil
		},
		setResolutionOverride: func(context.Context, purchasecore.SetResolutionOverrideRequest) (purchasecore.PurchaseLine, error) {
			t.Fatal("resolution override writer called for an overflowing value")
			return purchasecore.PurchaseLine{}, nil
		},
	}
	cases := []struct {
		name string
		path string
		body string
	}{
		{name: "mapping path id", path: "/v1/supplier-products/" + signedInt64Overflow + "/mapping/confirm", body: `{"actor":"tester","reason":"confirm","resourceId":"42","expectedRevision":"0"}`},
		{name: "confirm resource id", path: "/v1/supplier-products/9/mapping/confirm", body: `{"actor":"tester","reason":"confirm","resourceId":"` + signedInt64Overflow + `","expectedRevision":"0"}`},
		{name: "confirm mapping revision", path: "/v1/supplier-products/9/mapping/confirm", body: `{"actor":"tester","reason":"confirm","resourceId":"42","expectedRevision":"` + unsignedInt64Overflow + `"}`},
		{name: "correct current resource id", path: "/v1/supplier-products/9/mapping/correct", body: `{"actor":"tester","reason":"correct","expectedCurrentResourceId":"` + signedInt64Overflow + `","resourceId":"42","expectedRevision":"0"}`},
		{name: "correct resource id", path: "/v1/supplier-products/9/mapping/correct", body: `{"actor":"tester","reason":"correct","expectedCurrentResourceId":"42","resourceId":"` + signedInt64Overflow + `","expectedRevision":"0"}`},
		{name: "correct mapping revision", path: "/v1/supplier-products/9/mapping/correct", body: `{"actor":"tester","reason":"correct","expectedCurrentResourceId":"42","resourceId":"43","expectedRevision":"` + unsignedInt64Overflow + `"}`},
		{name: "retire current resource id", path: "/v1/supplier-products/9/mapping/retire", body: `{"actor":"tester","reason":"retire","expectedCurrentResourceId":"` + signedInt64Overflow + `","expectedRevision":"0"}`},
		{name: "retire mapping revision", path: "/v1/supplier-products/9/mapping/retire", body: `{"actor":"tester","reason":"retire","expectedCurrentResourceId":"42","expectedRevision":"` + unsignedInt64Overflow + `"}`},
		{name: "report current resource id", path: "/v1/supplier-products/9/mapping/report-conflict", body: `{"actor":"tester","reason":"report","expectedCurrentResourceId":"` + signedInt64Overflow + `","expectedRevision":"0"}`},
		{name: "report mapping revision", path: "/v1/supplier-products/9/mapping/report-conflict", body: `{"actor":"tester","reason":"report","expectedCurrentResourceId":"42","expectedRevision":"` + unsignedInt64Overflow + `"}`},
		{name: "resolve conflict current resource id", path: "/v1/supplier-products/9/mapping/resolve-conflict", body: `{"actor":"tester","reason":"resolve","expectedCurrentResourceId":"` + signedInt64Overflow + `","resourceId":"43","expectedRevision":"0"}`},
		{name: "resolve conflict resource id", path: "/v1/supplier-products/9/mapping/resolve-conflict", body: `{"actor":"tester","reason":"resolve","expectedCurrentResourceId":"42","resourceId":"` + signedInt64Overflow + `","expectedRevision":"0"}`},
		{name: "resolve conflict mapping revision", path: "/v1/supplier-products/9/mapping/resolve-conflict", body: `{"actor":"tester","reason":"resolve","expectedCurrentResourceId":"42","resourceId":"43","expectedRevision":"` + unsignedInt64Overflow + `"}`},
		{name: "resolve path id", path: "/v1/purchase-lines/" + signedInt64Overflow + "/resolve", body: validResolve},
		{name: "resolve resource id", path: "/v1/purchase-lines/11/resolve", body: `{"actor":"tester","reason":"resolve","resourceId":"` + signedInt64Overflow + `","expectedSupplierProductId":null,"expectedMappingRevision":null,"expectedResolutionRevision":"0","commercialSupplierSku":"COMM-9"}`},
		{name: "resolve supplier product snapshot id", path: "/v1/purchase-lines/11/resolve", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":"` + signedInt64Overflow + `","expectedMappingRevision":"0","expectedResolutionRevision":"0","commercialSupplierSku":""}`},
		{name: "resolve mapping revision", path: "/v1/purchase-lines/11/resolve", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":"9","expectedMappingRevision":"` + unsignedInt64Overflow + `","expectedResolutionRevision":"0","commercialSupplierSku":""}`},
		{name: "resolve resolution revision", path: "/v1/purchase-lines/11/resolve", body: `{"actor":"tester","reason":"resolve","resourceId":"42","expectedSupplierProductId":null,"expectedMappingRevision":null,"expectedResolutionRevision":"` + unsignedInt64Overflow + `","commercialSupplierSku":"COMM-9"}`},
		{name: "override path id", path: "/v1/purchase-lines/" + signedInt64Overflow + "/resolution-override", body: `{"actor":"tester","reason":"override","override":"NONE","expectedRevision":"0"}`},
		{name: "override resolution revision", path: "/v1/purchase-lines/11/resolution-override", body: `{"actor":"tester","reason":"override","override":"NONE","expectedRevision":"` + unsignedInt64Overflow + `"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewRouter(nil, nil, nil, nil, nil, nil, nil, writer).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestPurchaseMutationOpenAPINumericBoundaryPolicy(t *testing.T) {
	const (
		signedInt64Overflow   = "9223372036854775808"
		unsignedInt64Overflow = "18446744073709551616"
	)
	fields := []struct {
		schema   string
		field    string
		overflow string
		limit    string
	}{
		{"ConfirmMappingRequest", "resourceId", signedInt64Overflow, "9223372036854775807"},
		{"CorrectMappingRequest", "expectedCurrentResourceId", signedInt64Overflow, "9223372036854775807"},
		{"CorrectMappingRequest", "resourceId", signedInt64Overflow, "9223372036854775807"},
		{"RetireMappingRequest", "expectedCurrentResourceId", signedInt64Overflow, "9223372036854775807"},
		{"ReportMappingConflictRequest", "expectedCurrentResourceId", signedInt64Overflow, "9223372036854775807"},
		{"ResolveMappingConflictRequest", "expectedCurrentResourceId", signedInt64Overflow, "9223372036854775807"},
		{"ResolveMappingConflictRequest", "resourceId", signedInt64Overflow, "9223372036854775807"},
		{"ResolvePurchaseLineRequest", "resourceId", signedInt64Overflow, "9223372036854775807"},
		{"ResolvePurchaseLineRequest", "expectedSupplierProductId", signedInt64Overflow, "9223372036854775807"},
		{"MappingDecisionFields", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"ConfirmMappingRequest", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"CorrectMappingRequest", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"RetireMappingRequest", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"ReportMappingConflictRequest", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"ResolveMappingConflictRequest", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"ResolvePurchaseLineRequest", "expectedMappingRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"ResolvePurchaseLineRequest", "expectedResolutionRevision", unsignedInt64Overflow, "18446744073709551615"},
		{"ResolutionOverrideRequest", "expectedRevision", unsignedInt64Overflow, "18446744073709551615"},
	}
	for _, test := range fields {
		t.Run(test.schema+"/"+test.field, func(t *testing.T) {
			property := catalogSchema(t, test.schema).Properties[test.field]
			if property == nil || property.Value == nil {
				t.Fatalf("%s.%s schema is missing", test.schema, test.field)
			}
			if err := property.Value.VisitJSON(test.overflow); err != nil {
				t.Fatalf("schema rejected runtime boundary+1 %q: %v", test.overflow, err)
			}
			if !strings.Contains(property.Value.Description, test.limit) {
				t.Fatalf("description = %q, want runtime boundary %s", property.Value.Description, test.limit)
			}
		})
	}
}

func TestPurchaseSpecificErrorCodesStatusMatrix(t *testing.T) {
	tests := []struct {
		code   purchasecore.ErrorCode
		status int
	}{
		{purchasecore.PurchaseLineNotFound, http.StatusNotFound},
		{purchasecore.ResourceNotFound, http.StatusNotFound},
		{purchasecore.ResourceInactive, http.StatusUnprocessableEntity},
		{purchasecore.CommercialSupplierSKURequired, http.StatusUnprocessableEntity},
		{purchasecore.CommercialSupplierSKUForbidden, http.StatusUnprocessableEntity},
		{purchasecore.PurchaseLineStateConflict, http.StatusConflict},
		{purchasecore.StaleResolutionRevision, http.StatusConflict},
		{purchasecore.StaleMappingRevision, http.StatusConflict},
		{purchasecore.SupplierProductTargetConflict, http.StatusConflict},
		{purchasecore.InvalidMappingTransition, http.StatusConflict},
		{purchasecore.IntegrityConflict, http.StatusConflict},
	}
	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			rec := httptest.NewRecorder()
			writePurchaseError(rec, purchasecore.NewError(test.code, "internal detail"))
			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d", rec.Code, test.status)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != string(test.code) {
				t.Fatalf("code = %#v, want %q", body["code"], test.code)
			}
		})
	}
}

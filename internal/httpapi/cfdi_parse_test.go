package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cfdiSampleXML = `<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0" Serie="AB" Folio="7" Fecha="2026-03-05T09:30:15" SubTotal="100.00" Total="116.00" Moneda="MXN" TipoDeComprobante="I">
  <cfdi:Emisor Rfc="abc010101aa1" Nombre="PROVEEDOR SA DE CV" RegimenFiscal="601"/>
  <cfdi:Receptor Rfc="XAXX010101000" Nombre="CLIENTE" UsoCFDI="G03"/>
  <cfdi:Conceptos>
    <cfdi:Concepto ClaveProdServ="26121600" NoIdentificacion="CAB-1" Cantidad="10" ClaveUnidad="MTR" Descripcion="CABLE" ValorUnitario="10.00" Importe="100.00" ObjetoImp="02">
      <cfdi:Impuestos><cfdi:Traslados><cfdi:Traslado Base="100.00" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="16.00"/></cfdi:Traslados></cfdi:Impuestos>
    </cfdi:Concepto>
  </cfdi:Conceptos>
  <cfdi:Impuestos TotalImpuestosTrasladados="16.00"><cfdi:Traslados><cfdi:Traslado Base="100.00" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="16.00"/></cfdi:Traslados></cfdi:Impuestos>
  <cfdi:Complemento><tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="1.1" UUID="abcdef12-3456-7890-abcd-ef1234567890" FechaTimbrado="2026-03-05T09:30:20" RfcProvCertif="SPR190613I52" SelloCFD="S1" NoCertificadoSAT="0001" SelloSAT="S2"/></cfdi:Complemento>
</cfdi:Comprobante>`

func cfdiMultipartRequest(t *testing.T, field, filename, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/cfdi/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func serveCFDI(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	NewRouter(nil, nil, nil, nil, nil, nil, nil, nil).ServeHTTP(rec, req)
	return rec
}

func TestCFDIParseMultipartReturnsInvoiceAndSupplierDraft(t *testing.T) {
	rec := serveCFDI(t, cfdiMultipartRequest(t, "file", "factura.xml", cfdiSampleXML))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	validateSchemaJSON(t, catalogSchema(t, "CFDIParseResponse"), body)

	draft := body["supplierDraft"].(map[string]any)
	if draft["taxIdentifier"] != "ABC010101AA1" || draft["legalName"] != "PROVEEDOR SA DE CV" || draft["taxRegime"] != "601" {
		t.Fatalf("supplierDraft = %#v", draft)
	}
	inv := body["invoice"].(map[string]any)
	if inv["folio"] != "7" || inv["issuedAt"] != "2026-03-05T09:30:15" || inv["total"] != "116.00" {
		t.Fatalf("invoice header = %#v", inv)
	}
	concepts := inv["concepts"].([]any)
	if len(concepts) != 1 {
		t.Fatalf("concepts = %d, want 1", len(concepts))
	}
	first := concepts[0].(map[string]any)
	if first["description"] != "CABLE" || first["itemNumber"] != "CAB-1" || first["amount"] != "100.00" {
		t.Fatalf("concept = %#v", first)
	}
	stamp := inv["stamp"].(map[string]any)
	if stamp["uuid"] != "ABCDEF12-3456-7890-ABCD-EF1234567890" {
		t.Fatalf("stamp = %#v", stamp)
	}
}

func TestCFDIParseAcceptsRawXMLBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/cfdi/parse", strings.NewReader(cfdiSampleXML))
	req.Header.Set("Content-Type", "application/xml")

	rec := serveCFDI(t, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["supplierDraft"].(map[string]any)["taxIdentifier"] != "ABC010101AA1" {
		t.Fatalf("body = %#v", body)
	}
}

func TestCFDIParseEmitsEmptyArraysAndNullStamp(t *testing.T) {
	const minimal = `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0"><cfdi:Emisor Rfc="ABC010101AA1"/></cfdi:Comprobante>`
	req := httptest.NewRequest(http.MethodPost, "/v1/cfdi/parse", strings.NewReader(minimal))

	rec := serveCFDI(t, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	validateSchemaJSON(t, catalogSchema(t, "CFDIParseResponse"), body)
	inv := body["invoice"].(map[string]any)
	if got, ok := inv["concepts"].([]any); !ok || len(got) != 0 {
		t.Fatalf("concepts = %#v, want []", inv["concepts"])
	}
	if v, ok := inv["stamp"]; !ok || v != nil {
		t.Fatalf("stamp = %#v, want explicit null", inv["stamp"])
	}
	if inv["issuedAt"] != "" {
		t.Fatalf("issuedAt = %#v, want empty string when absent", inv["issuedAt"])
	}
}

func TestCFDIParseRejections(t *testing.T) {
	tests := []struct {
		name     string
		req      func(t *testing.T) *http.Request
		status   int
		errText  string
		wantCode string
	}{
		{
			name:    "multipart without file field",
			req:     func(t *testing.T) *http.Request { return cfdiMultipartRequest(t, "other", "f.xml", cfdiSampleXML) },
			status:  http.StatusBadRequest,
			errText: "invalid request",
		},
		{
			name: "empty body",
			req: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodPost, "/v1/cfdi/parse", strings.NewReader(""))
			},
			status:   http.StatusUnprocessableEntity,
			errText:  "unprocessable cfdi",
			wantCode: "INVALID_XML",
		},
		{
			name: "not xml",
			req: func(t *testing.T) *http.Request {
				return cfdiMultipartRequest(t, "file", "f.xml", "hello world")
			},
			status:   http.StatusUnprocessableEntity,
			errText:  "unprocessable cfdi",
			wantCode: "INVALID_XML",
		},
		{
			name: "xml that is not a cfdi",
			req: func(t *testing.T) *http.Request {
				return cfdiMultipartRequest(t, "file", "f.xml", `<invoice/>`)
			},
			status:   http.StatusUnprocessableEntity,
			errText:  "unprocessable cfdi",
			wantCode: "NOT_CFDI",
		},
		{
			name: "unsupported version",
			req: func(t *testing.T) *http.Request {
				return cfdiMultipartRequest(t, "file", "f.xml", `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="5.0"/>`)
			},
			status:   http.StatusUnprocessableEntity,
			errText:  "unprocessable cfdi",
			wantCode: "UNSUPPORTED_VERSION",
		},
		{
			name: "missing issuer rfc",
			req: func(t *testing.T) *http.Request {
				return cfdiMultipartRequest(t, "file", "f.xml", `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0"/>`)
			},
			status:   http.StatusUnprocessableEntity,
			errText:  "unprocessable cfdi",
			wantCode: "INVALID_CFDI",
		},
		{
			name: "body over the router limit",
			req: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodPost, "/v1/cfdi/parse", strings.NewReader(strings.Repeat("a", maxRequestBodyBytes+1)))
			},
			status:  http.StatusBadRequest,
			errText: "invalid request",
		},
		{
			name: "malformed multipart",
			req: func(t *testing.T) *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/v1/cfdi/parse", strings.NewReader("garbage"))
				req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
				return req
			},
			status:  http.StatusBadRequest,
			errText: "invalid request",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveCFDI(t, tt.req(t))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.status, rec.Body)
			}
			var got errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v (%s)", err, rec.Body)
			}
			if got.Error != tt.errText || got.Code != tt.wantCode {
				t.Fatalf("error = %#v, want error=%q code=%q", got, tt.errText, tt.wantCode)
			}
			if strings.Contains(rec.Body.String(), "hello world") {
				t.Fatal("response must not echo request data")
			}
		})
	}
}

func TestCFDIParseRejectsOtherMethods(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := serveCFDI(t, httptest.NewRequest(method, "/v1/cfdi/parse", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", method, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodPost {
			t.Fatalf("%s Allow = %q, want POST", method, got)
		}
	}
}

func TestCFDIParseRequestOpenAPIShape(t *testing.T) {
	rec := serveCFDI(t, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	body := rec.Body.String()
	for _, want := range []string{"  /v1/cfdi/parse:\n", "multipart/form-data", "application/xml", "format: binary"} {
		if !strings.Contains(body, want) {
			t.Errorf("OpenAPI document does not contain %q", want)
		}
	}
}

// cfdiSamplesGlob points at real supplier invoices that live in the Core
// checkout (outside git); the test skips when they are not on this machine.
const cfdiSamplesGlob = "../../../garfex-costos-unitarios-workspace/docs/ejemplo xlm/*.xml"

func TestCFDIParseRealSamplesMatchOpenAPISchema(t *testing.T) {
	files, err := filepath.Glob(cfdiSamplesGlob)
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
			rec := serveCFDI(t, cfdiMultipartRequest(t, "file", filepath.Base(file), string(data)))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %.300s", rec.Code, rec.Body)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			validateSchemaJSON(t, catalogSchema(t, "CFDIParseResponse"), body)
			if body["supplierDraft"].(map[string]any)["taxIdentifier"] == "" {
				t.Fatal("supplierDraft.taxIdentifier is empty")
			}
		})
	}
}

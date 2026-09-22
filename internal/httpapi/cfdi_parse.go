package httpapi

import (
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"github.com/GARFEX33/garfex-backend/cfdicore"
)

// cfdiDateLayout mirrors the SAT xs:dateTime shape. CFDI dates carry no zone,
// so they are emitted as written instead of pretending to be UTC.
const cfdiDateLayout = "2006-01-02T15:04:05"

type cfdiParseResponse struct {
	Invoice       cfdiInvoiceResponse       `json:"invoice"`
	SupplierDraft cfdiSupplierDraftResponse `json:"supplierDraft"`
}

// cfdiSupplierDraftResponse is the Emisor shaped like a supplier create
// request, so a UI can prefill the form. TaxRegime has no supplier field and
// is informational only.
type cfdiSupplierDraftResponse struct {
	TaxIdentifier string `json:"taxIdentifier"`
	LegalName     string `json:"legalName"`
	TaxRegime     string `json:"taxRegime"`
}

type cfdiInvoiceResponse struct {
	Version           string                `json:"version"`
	Series            string                `json:"series"`
	Folio             string                `json:"folio"`
	IssuedAt          string                `json:"issuedAt"`
	Seal              string                `json:"seal"`
	PaymentForm       string                `json:"paymentForm"`
	CertificateNumber string                `json:"certificateNumber"`
	Certificate       string                `json:"certificate"`
	PaymentConditions string                `json:"paymentConditions"`
	Subtotal          string                `json:"subtotal"`
	Discount          string                `json:"discount"`
	Currency          string                `json:"currency"`
	ExchangeRate      string                `json:"exchangeRate"`
	Total             string                `json:"total"`
	VoucherType       string                `json:"voucherType"`
	Export            string                `json:"export"`
	PaymentMethod     string                `json:"paymentMethod"`
	IssuePlace        string                `json:"issuePlace"`
	Confirmation      string                `json:"confirmation"`
	Related           []cfdiRelatedResponse `json:"related"`
	Issuer            cfdiIssuerResponse    `json:"issuer"`
	Receiver          cfdiReceiverResponse  `json:"receiver"`
	Concepts          []cfdiConceptResponse `json:"concepts"`
	Taxes             cfdiTaxesResponse     `json:"taxes"`
	Stamp             *cfdiStampResponse    `json:"stamp"`
}

type cfdiRelatedResponse struct {
	RelationType string   `json:"relationType"`
	UUIDs        []string `json:"uuids"`
}

type cfdiIssuerResponse struct {
	TaxIdentifier string `json:"taxIdentifier"`
	Name          string `json:"name"`
	TaxRegime     string `json:"taxRegime"`
}

type cfdiReceiverResponse struct {
	TaxIdentifier        string `json:"taxIdentifier"`
	Name                 string `json:"name"`
	PostalCode           string `json:"postalCode"`
	TaxRegime            string `json:"taxRegime"`
	CFDIUse              string `json:"cfdiUse"`
	ForeignResidence     string `json:"foreignResidence"`
	ForeignTaxIdentifier string `json:"foreignTaxIdentifier"`
}

type cfdiConceptResponse struct {
	ProductServiceCode string                   `json:"productServiceCode"`
	ItemNumber         string                   `json:"itemNumber"`
	Quantity           string                   `json:"quantity"`
	UnitCode           string                   `json:"unitCode"`
	Unit               string                   `json:"unit"`
	Description        string                   `json:"description"`
	UnitPrice          string                   `json:"unitPrice"`
	Amount             string                   `json:"amount"`
	Discount           string                   `json:"discount"`
	TaxObject          string                   `json:"taxObject"`
	Taxes              cfdiConceptTaxesResponse `json:"taxes"`
}

type cfdiConceptTaxesResponse struct {
	Transferred []cfdiTaxResponse `json:"transferred"`
	Withheld    []cfdiTaxResponse `json:"withheld"`
}

type cfdiTaxesResponse struct {
	TotalTransferred string            `json:"totalTransferred"`
	TotalWithheld    string            `json:"totalWithheld"`
	Transferred      []cfdiTaxResponse `json:"transferred"`
	Withheld         []cfdiTaxResponse `json:"withheld"`
}

type cfdiTaxResponse struct {
	Base       string `json:"base"`
	Tax        string `json:"tax"`
	FactorType string `json:"factorType"`
	Rate       string `json:"rate"`
	Amount     string `json:"amount"`
}

type cfdiStampResponse struct {
	Version               string `json:"version"`
	UUID                  string `json:"uuid"`
	StampedAt             string `json:"stampedAt"`
	ProviderTaxIdentifier string `json:"providerTaxIdentifier"`
	Legend                string `json:"legend"`
	CFDSeal               string `json:"cfdSeal"`
	SATCertificateNumber  string `json:"satCertificateNumber"`
	SATSeal               string `json:"satSeal"`
}

// serveCFDIParse reads one CFDI 4.0 XML, sent either as a multipart "file"
// field or as the raw request body, and returns its data. It is stateless:
// nothing is stored and Core's database is never touched.
func serveCFDIParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	data, ok := readCFDIUpload(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	invoice, err := cfdicore.Parse(data)
	if err != nil {
		writeCFDIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapCFDIParse(invoice))
}

// readCFDIUpload extracts the XML bytes. The router already bounds the body
// to maxRequestBodyBytes, so every read below is bounded too.
func readCFDIUpload(r *http.Request) ([]byte, bool) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		data, err := io.ReadAll(r.Body)
		return data, err == nil
	}
	if err := r.ParseMultipartForm(maxRequestBodyBytes); err != nil {
		return nil, false
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	return data, err == nil
}

func writeCFDIError(w http.ResponseWriter, err error) {
	code := cfdicore.Code(err)
	// Only the code is logged: parser messages may echo document content.
	log.Printf("cfdi parse error: code=%s", code)

	detail, known := cfdiErrorDetails[code]
	if !known {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
		return
	}
	writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
		Error:  "unprocessable cfdi",
		Code:   string(code),
		Detail: detail,
	})
}

// cfdiErrorDetails holds developer-authored static messages, safe to expose.
var cfdiErrorDetails = map[cfdicore.ErrorCode]string{
	cfdicore.InvalidXML:         "the file is not well-formed XML",
	cfdicore.NotCFDI:            "the document is not a CFDI 4.0 voucher",
	cfdicore.UnsupportedVersion: "only CFDI version 4.0 is supported",
	cfdicore.InvalidCFDI:        "the voucher is missing required data or has invalid values",
}

func mapCFDIParse(inv cfdicore.Invoice) cfdiParseResponse {
	out := cfdiInvoiceResponse{
		Version:           inv.Version,
		Series:            inv.Series,
		Folio:             inv.Folio,
		IssuedAt:          formatCFDIDate(inv.IssuedAt),
		Seal:              inv.Seal,
		PaymentForm:       inv.PaymentForm,
		CertificateNumber: inv.CertificateNumber,
		Certificate:       inv.Certificate,
		PaymentConditions: inv.PaymentConditions,
		Subtotal:          inv.Subtotal,
		Discount:          inv.Discount,
		Currency:          inv.Currency,
		ExchangeRate:      inv.ExchangeRate,
		Total:             inv.Total,
		VoucherType:       inv.VoucherType,
		Export:            inv.Export,
		PaymentMethod:     inv.PaymentMethod,
		IssuePlace:        inv.IssuePlace,
		Confirmation:      inv.Confirmation,
		Related:           make([]cfdiRelatedResponse, 0, len(inv.Related)),
		Issuer: cfdiIssuerResponse{
			TaxIdentifier: inv.Issuer.TaxID,
			Name:          inv.Issuer.Name,
			TaxRegime:     inv.Issuer.TaxRegime,
		},
		Receiver: cfdiReceiverResponse{
			TaxIdentifier:        inv.Receiver.TaxID,
			Name:                 inv.Receiver.Name,
			PostalCode:           inv.Receiver.PostalCode,
			TaxRegime:            inv.Receiver.TaxRegime,
			CFDIUse:              inv.Receiver.CFDIUse,
			ForeignResidence:     inv.Receiver.ForeignResidence,
			ForeignTaxIdentifier: inv.Receiver.ForeignTaxID,
		},
		Concepts: make([]cfdiConceptResponse, 0, len(inv.Concepts)),
		Taxes: cfdiTaxesResponse{
			TotalTransferred: inv.Taxes.TotalTransferred,
			TotalWithheld:    inv.Taxes.TotalWithheld,
			Transferred:      mapCFDITaxes(inv.Taxes.Transferred),
			Withheld:         mapCFDITaxes(inv.Taxes.Withheld),
		},
	}
	for _, rel := range inv.Related {
		uuids := rel.UUIDs
		if uuids == nil {
			uuids = []string{}
		}
		out.Related = append(out.Related, cfdiRelatedResponse{RelationType: rel.RelationType, UUIDs: uuids})
	}
	for _, c := range inv.Concepts {
		out.Concepts = append(out.Concepts, cfdiConceptResponse{
			ProductServiceCode: c.ProductServiceCode,
			ItemNumber:         c.ItemNumber,
			Quantity:           c.Quantity,
			UnitCode:           c.UnitCode,
			Unit:               c.Unit,
			Description:        c.Description,
			UnitPrice:          c.UnitPrice,
			Amount:             c.Amount,
			Discount:           c.Discount,
			TaxObject:          c.TaxObject,
			Taxes: cfdiConceptTaxesResponse{
				Transferred: mapCFDITaxes(c.Taxes.Transferred),
				Withheld:    mapCFDITaxes(c.Taxes.Withheld),
			},
		})
	}
	if s := inv.Stamp; s != nil {
		out.Stamp = &cfdiStampResponse{
			Version:               s.Version,
			UUID:                  s.UUID,
			StampedAt:             formatCFDIDate(s.StampedAt),
			ProviderTaxIdentifier: s.ProviderTaxID,
			Legend:                s.Legend,
			CFDSeal:               s.CFDSeal,
			SATCertificateNumber:  s.SATCertificateNumber,
			SATSeal:               s.SATSeal,
		}
	}
	return cfdiParseResponse{
		Invoice:       out,
		SupplierDraft: cfdiSupplierDraft(inv),
	}
}

func cfdiSupplierDraft(inv cfdicore.Invoice) cfdiSupplierDraftResponse {
	return cfdiSupplierDraftResponse{
		TaxIdentifier: inv.Issuer.TaxID,
		LegalName:     inv.Issuer.Name,
		TaxRegime:     inv.Issuer.TaxRegime,
	}
}

func mapCFDITaxes(in []cfdicore.Tax) []cfdiTaxResponse {
	out := make([]cfdiTaxResponse, len(in))
	for i, t := range in {
		out[i] = cfdiTaxResponse{Base: t.Base, Tax: t.Tax, FactorType: t.FactorType, Rate: t.Rate, Amount: t.Amount}
	}
	return out
}

func formatCFDIDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(cfdiDateLayout)
}

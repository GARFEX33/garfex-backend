package cfdicore

import "time"

// Invoice is the data extracted from one CFDI 4.0 voucher.
//
// Monetary amounts, quantities and rates are exact decimal strings copied
// from the XML, never floats. Fields the document does not carry are empty.
type Invoice struct {
	Version           string
	Series            string
	Folio             string
	IssuedAt          time.Time
	Seal              string
	PaymentForm       string
	CertificateNumber string
	Certificate       string
	PaymentConditions string
	Subtotal          string
	Discount          string
	Currency          string
	ExchangeRate      string
	Total             string
	VoucherType       string
	Export            string
	PaymentMethod     string
	IssuePlace        string
	Confirmation      string

	Related  []RelatedCFDIs
	Issuer   Issuer
	Receiver Receiver
	Concepts []Concept
	Taxes    Taxes
	// Stamp is nil when the voucher carries no TimbreFiscalDigital complement.
	Stamp *Stamp
}

// RelatedCFDIs is one CfdiRelacionados group.
type RelatedCFDIs struct {
	RelationType string
	UUIDs        []string
}

// Issuer is the Emisor. TaxID is trimmed and upper-cased.
type Issuer struct {
	TaxID     string
	Name      string
	TaxRegime string
}

// Receiver is the Receptor. TaxID is trimmed and upper-cased.
type Receiver struct {
	TaxID            string
	Name             string
	PostalCode       string
	TaxRegime        string
	CFDIUse          string
	ForeignResidence string
	ForeignTaxID     string
}

// Concept is one line item of the voucher.
type Concept struct {
	ProductServiceCode string
	ItemNumber         string
	Quantity           string
	UnitCode           string
	Unit               string
	Description        string
	UnitPrice          string
	Amount             string
	Discount           string
	TaxObject          string
	Taxes              ConceptTaxes
}

// ConceptTaxes are the taxes declared on a single concept.
type ConceptTaxes struct {
	Transferred []Tax
	Withheld    []Tax
}

// Taxes are the voucher-level tax totals and breakdown.
type Taxes struct {
	TotalTransferred string
	TotalWithheld    string
	Transferred      []Tax
	Withheld         []Tax
}

// Tax is one transferred or withheld tax line. Voucher-level withholdings
// only carry Tax and Amount.
type Tax struct {
	Base       string
	Tax        string
	FactorType string
	Rate       string
	Amount     string
}

// Stamp is the SAT TimbreFiscalDigital complement. UUID is upper-cased.
type Stamp struct {
	Version              string
	UUID                 string
	StampedAt            time.Time
	ProviderTaxID        string
	Legend               string
	CFDSeal              string
	SATCertificateNumber string
	SATSeal              string
}

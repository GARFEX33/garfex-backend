package cfdicore

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	nsCFDI = "http://www.sat.gob.mx/cfd/4"

	supportedVersion = "4.0"
	// dateLayout is the SAT xs:dateTime shape, which carries no zone.
	dateLayout = "2006-01-02T15:04:05"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Parse extracts the data of one CFDI 4.0 voucher from its XML bytes.
//
// Elements are matched by namespace URI, so any prefix works, and attribute
// order is irrelevant. A leading UTF-8 byte order mark is ignored. Dates
// have no zone in the XML and are returned in UTC. data is never modified.
func Parse(data []byte) (Invoice, error) {
	data = bytes.TrimLeft(bytes.TrimPrefix(data, utf8BOM), " \t\r\n")

	dec := xml.NewDecoder(bytes.NewReader(data))
	root, err := firstElement(dec)
	if err != nil {
		return Invoice{}, err
	}
	if root.Name.Space != nsCFDI || root.Name.Local != "Comprobante" {
		return Invoice{}, NewError(NotCFDI, "document is not a CFDI 4.0 Comprobante")
	}

	var raw xmlComprobante
	if err := dec.DecodeElement(&raw, &root); err != nil {
		return Invoice{}, NewError(InvalidXML, "invalid XML: "+err.Error())
	}
	return raw.toInvoice()
}

func firstElement(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return xml.StartElement{}, NewError(InvalidXML, "invalid XML: no root element")
			}
			return xml.StartElement{}, NewError(InvalidXML, "invalid XML: "+err.Error())
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start, nil
		}
	}
}

type xmlComprobante struct {
	Version           string `xml:"Version,attr"`
	Series            string `xml:"Serie,attr"`
	Folio             string `xml:"Folio,attr"`
	Date              string `xml:"Fecha,attr"`
	Seal              string `xml:"Sello,attr"`
	PaymentForm       string `xml:"FormaPago,attr"`
	CertificateNumber string `xml:"NoCertificado,attr"`
	Certificate       string `xml:"Certificado,attr"`
	PaymentConditions string `xml:"CondicionesDePago,attr"`
	Subtotal          string `xml:"SubTotal,attr"`
	Discount          string `xml:"Descuento,attr"`
	Currency          string `xml:"Moneda,attr"`
	ExchangeRate      string `xml:"TipoCambio,attr"`
	Total             string `xml:"Total,attr"`
	VoucherType       string `xml:"TipoDeComprobante,attr"`
	Export            string `xml:"Exportacion,attr"`
	PaymentMethod     string `xml:"MetodoPago,attr"`
	IssuePlace        string `xml:"LugarExpedicion,attr"`
	Confirmation      string `xml:"Confirmacion,attr"`

	Related []struct {
		RelationType string `xml:"TipoRelacion,attr"`
		Items        []struct {
			UUID string `xml:"UUID,attr"`
		} `xml:"http://www.sat.gob.mx/cfd/4 CfdiRelacionado"`
	} `xml:"http://www.sat.gob.mx/cfd/4 CfdiRelacionados"`

	Issuer *struct {
		TaxID     string `xml:"Rfc,attr"`
		Name      string `xml:"Nombre,attr"`
		TaxRegime string `xml:"RegimenFiscal,attr"`
	} `xml:"http://www.sat.gob.mx/cfd/4 Emisor"`

	Receiver *struct {
		TaxID            string `xml:"Rfc,attr"`
		Name             string `xml:"Nombre,attr"`
		PostalCode       string `xml:"DomicilioFiscalReceptor,attr"`
		TaxRegime        string `xml:"RegimenFiscalReceptor,attr"`
		CFDIUse          string `xml:"UsoCFDI,attr"`
		ForeignResidence string `xml:"ResidenciaFiscal,attr"`
		ForeignTaxID     string `xml:"NumRegIdTrib,attr"`
	} `xml:"http://www.sat.gob.mx/cfd/4 Receptor"`

	Concepts []struct {
		ProductServiceCode string      `xml:"ClaveProdServ,attr"`
		ItemNumber         string      `xml:"NoIdentificacion,attr"`
		Quantity           string      `xml:"Cantidad,attr"`
		UnitCode           string      `xml:"ClaveUnidad,attr"`
		Unit               string      `xml:"Unidad,attr"`
		Description        string      `xml:"Descripcion,attr"`
		UnitPrice          string      `xml:"ValorUnitario,attr"`
		Amount             string      `xml:"Importe,attr"`
		Discount           string      `xml:"Descuento,attr"`
		TaxObject          string      `xml:"ObjetoImp,attr"`
		Taxes              xmlTaxGroup `xml:"http://www.sat.gob.mx/cfd/4 Impuestos"`
	} `xml:"http://www.sat.gob.mx/cfd/4 Conceptos>Concepto"`

	Taxes struct {
		TotalTransferred string `xml:"TotalImpuestosTrasladados,attr"`
		TotalWithheld    string `xml:"TotalImpuestosRetenidos,attr"`
		xmlTaxGroup
	} `xml:"http://www.sat.gob.mx/cfd/4 Impuestos"`

	Complement struct {
		Stamp *struct {
			Version              string `xml:"Version,attr"`
			UUID                 string `xml:"UUID,attr"`
			StampedAt            string `xml:"FechaTimbrado,attr"`
			ProviderTaxID        string `xml:"RfcProvCertif,attr"`
			Legend               string `xml:"Leyenda,attr"`
			CFDSeal              string `xml:"SelloCFD,attr"`
			SATCertificateNumber string `xml:"NoCertificadoSAT,attr"`
			SATSeal              string `xml:"SelloSAT,attr"`
		} `xml:"http://www.sat.gob.mx/TimbreFiscalDigital TimbreFiscalDigital"`
	} `xml:"http://www.sat.gob.mx/cfd/4 Complemento"`
}

type xmlTaxGroup struct {
	Transferred []xmlTax `xml:"http://www.sat.gob.mx/cfd/4 Traslados>Traslado"`
	Withheld    []xmlTax `xml:"http://www.sat.gob.mx/cfd/4 Retenciones>Retencion"`
}

type xmlTax struct {
	Base       string `xml:"Base,attr"`
	Tax        string `xml:"Impuesto,attr"`
	FactorType string `xml:"TipoFactor,attr"`
	Rate       string `xml:"TasaOCuota,attr"`
	Amount     string `xml:"Importe,attr"`
}

func (r xmlComprobante) toInvoice() (Invoice, error) {
	if v := clean(r.Version); v != supportedVersion {
		return Invoice{}, NewError(UnsupportedVersion, "unsupported CFDI version "+quote(v))
	}
	if r.Issuer == nil || clean(r.Issuer.TaxID) == "" {
		return Invoice{}, NewError(InvalidCFDI, "Emisor Rfc is missing")
	}
	issuedAt, err := parseDate(r.Date, "Fecha")
	if err != nil {
		return Invoice{}, err
	}

	inv := Invoice{
		Version:           supportedVersion,
		Series:            clean(r.Series),
		Folio:             clean(r.Folio),
		IssuedAt:          issuedAt,
		Seal:              clean(r.Seal),
		PaymentForm:       clean(r.PaymentForm),
		CertificateNumber: clean(r.CertificateNumber),
		Certificate:       clean(r.Certificate),
		PaymentConditions: clean(r.PaymentConditions),
		Subtotal:          clean(r.Subtotal),
		Discount:          clean(r.Discount),
		Currency:          clean(r.Currency),
		ExchangeRate:      clean(r.ExchangeRate),
		Total:             clean(r.Total),
		VoucherType:       clean(r.VoucherType),
		Export:            clean(r.Export),
		PaymentMethod:     clean(r.PaymentMethod),
		IssuePlace:        clean(r.IssuePlace),
		Confirmation:      clean(r.Confirmation),
		Issuer: Issuer{
			TaxID:     upper(r.Issuer.TaxID),
			Name:      clean(r.Issuer.Name),
			TaxRegime: clean(r.Issuer.TaxRegime),
		},
		Taxes: Taxes{
			TotalTransferred: clean(r.Taxes.TotalTransferred),
			TotalWithheld:    clean(r.Taxes.TotalWithheld),
			Transferred:      mapTaxes(r.Taxes.Transferred),
			Withheld:         mapTaxes(r.Taxes.Withheld),
		},
	}

	if rc := r.Receiver; rc != nil {
		inv.Receiver = Receiver{
			TaxID:            upper(rc.TaxID),
			Name:             clean(rc.Name),
			PostalCode:       clean(rc.PostalCode),
			TaxRegime:        clean(rc.TaxRegime),
			CFDIUse:          clean(rc.CFDIUse),
			ForeignResidence: clean(rc.ForeignResidence),
			ForeignTaxID:     clean(rc.ForeignTaxID),
		}
	}

	for _, rel := range r.Related {
		group := RelatedCFDIs{RelationType: clean(rel.RelationType)}
		for _, item := range rel.Items {
			group.UUIDs = append(group.UUIDs, upper(item.UUID))
		}
		inv.Related = append(inv.Related, group)
	}

	for _, c := range r.Concepts {
		inv.Concepts = append(inv.Concepts, Concept{
			ProductServiceCode: clean(c.ProductServiceCode),
			ItemNumber:         clean(c.ItemNumber),
			Quantity:           clean(c.Quantity),
			UnitCode:           clean(c.UnitCode),
			Unit:               clean(c.Unit),
			Description:        clean(c.Description),
			UnitPrice:          clean(c.UnitPrice),
			Amount:             clean(c.Amount),
			Discount:           clean(c.Discount),
			TaxObject:          clean(c.TaxObject),
			Taxes: ConceptTaxes{
				Transferred: mapTaxes(c.Taxes.Transferred),
				Withheld:    mapTaxes(c.Taxes.Withheld),
			},
		})
	}

	if s := r.Complement.Stamp; s != nil {
		stampedAt, err := parseDate(s.StampedAt, "FechaTimbrado")
		if err != nil {
			return Invoice{}, err
		}
		inv.Stamp = &Stamp{
			Version:              clean(s.Version),
			UUID:                 upper(s.UUID),
			StampedAt:            stampedAt,
			ProviderTaxID:        upper(s.ProviderTaxID),
			Legend:               clean(s.Legend),
			CFDSeal:              clean(s.CFDSeal),
			SATCertificateNumber: clean(s.SATCertificateNumber),
			SATSeal:              clean(s.SATSeal),
		}
	}
	return inv, nil
}

func mapTaxes(in []xmlTax) []Tax {
	if len(in) == 0 {
		return nil
	}
	out := make([]Tax, len(in))
	for i, t := range in {
		out[i] = Tax{
			Base:       clean(t.Base),
			Tax:        clean(t.Tax),
			FactorType: clean(t.FactorType),
			Rate:       clean(t.Rate),
			Amount:     clean(t.Amount),
		}
	}
	return out
}

func parseDate(value, attr string) (time.Time, error) {
	value = clean(value)
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, NewError(InvalidCFDI, attr+" is not a valid CFDI date")
	}
	return t, nil
}

func clean(s string) string { return strings.TrimSpace(s) }

func upper(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// quote keeps the offending value out of multi-line or unbounded messages.
func quote(v string) string {
	if v == "" {
		return "(missing)"
	}
	if len(v) > 16 {
		v = v[:16]
	}
	return `"` + strings.ReplaceAll(v, "\n", " ") + `"`
}

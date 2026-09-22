package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

const cfdiTemplate = `<?xml version="1.0" encoding="utf-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0" Serie="AB" Folio="1234"
    Fecha="2026-03-05T09:30:15" SubTotal="%[2]s" Moneda="MXN" Total="%[2]s">
  <cfdi:Emisor Rfc="ABC010101AA1" Nombre="PROVEEDOR DE PRUEBA SA DE CV" RegimenFiscal="601"/>
  <cfdi:Receptor Rfc="XAXX010101000" Nombre="CLIENTE" DomicilioFiscalReceptor="64010" RegimenFiscalReceptor="612" UsoCFDI="G03"/>
  <cfdi:Conceptos>
    <cfdi:Concepto ClaveProdServ="26121600" NoIdentificacion="SKU-1" Cantidad="10" ClaveUnidad="MTR" Unidad="mts"
        Descripcion="CABLE THHN 10 AWG" ValorUnitario="10.00" Importe="%[2]s" ObjetoImp="02"/>
  </cfdi:Conceptos>
  <cfdi:Complemento>
    <tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="1.1"
        UUID="%[1]s" FechaTimbrado="2026-03-05T09:30:20" RfcProvCertif="SPR190613I52"
        SelloCFD="SELLO" NoCertificadoSAT="00001" SelloSAT="SELLO-SAT"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`

func buildCFDI(uuid, total string) []byte {
	return []byte(strings.NewReplacer("%[1]s", uuid, "%[2]s", total).Replace(cfdiTemplate))
}
func validMappingDecision() domain.MappingDecisionMetadata {
	return domain.MappingDecisionMetadata{Actor: "tester", Origin: domain.MappingOriginManual, Reason: "test decision", At: time.Now()}
}
func newTestService() (*Service, *memoryRepository, *fakeSupplierDirectory) {
	repo := newMemoryRepository()
	suppliers := newFakeSupplierDirectory()
	return NewService(repo, suppliers), repo, suppliers
}

func TestImportCFDICreatesPurchaseAndAutoRegistersSupplier(t *testing.T) {
	service, _, suppliers := newTestService()
	result, err := service.ImportCFDI(t.Context(), buildCFDI("ABCDEF12-3456-7890-ABCD-EF1234567890", "100.00"), ImportOptions{Filename: "factura.xml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyExisted {
		t.Fatal("expected a fresh import")
	}
	if result.Purchase.CFDIUUID != "ABCDEF12-3456-7890-ABCD-EF1234567890" {
		t.Fatalf("cfdi uuid = %q", result.Purchase.CFDIUUID)
	}
	if len(result.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(result.Lines))
	}
	line := result.Lines[0]
	if line.SupplierSKU != "SKU-1" || line.SupplierProductID == nil {
		t.Fatalf("line = %+v, want resolved supplier product", line)
	}
	if line.DerivedStatus != domain.LinkPending {
		t.Fatalf("effective status = %q, want PENDIENTE for a brand new supplier product", line.DerivedStatus)
	}
	if suppliers.createSupplierCalls != 1 {
		t.Fatalf("createSupplierCalls = %d, want 1", suppliers.createSupplierCalls)
	}
}

func TestImportCFDIReusesKnownSupplier(t *testing.T) {
	service, _, suppliers := newTestService()
	if _, err := suppliers.CreateSupplier(t.Context(), supplierdomain.SupplierDetails{LegalName: "Existing", TaxIdentifier: "ABC010101AA1"}); err != nil {
		t.Fatalf("seed supplier: %v", err)
	}
	if _, err := service.ImportCFDI(t.Context(), buildCFDI("ABCDEF12-3456-7890-ABCD-EF1234567890", "100.00"), ImportOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suppliers.createSupplierCalls != 1 {
		t.Fatalf("createSupplierCalls = %d, want 1 (only the seed)", suppliers.createSupplierCalls)
	}
}
func TestImportCFDIIsIdempotentOnSameUUID(t *testing.T) {
	service, _, _ := newTestService()
	xml := buildCFDI("ABCDEF12-3456-7890-ABCD-EF1234567890", "100.00")
	first, err := service.ImportCFDI(t.Context(), xml, ImportOptions{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	second, err := service.ImportCFDI(t.Context(), xml, ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if !second.AlreadyExisted {
		t.Fatal("expected the second import to report AlreadyExisted")
	}
	if second.Purchase.ID != first.Purchase.ID {
		t.Fatalf("purchase id = %d, want %d (no duplicate)", second.Purchase.ID, first.Purchase.ID)
	}
}
func TestImportCFDIRejectsConflictingContentForSameUUID(t *testing.T) {
	service, _, _ := newTestService()
	uuid := "ABCDEF12-3456-7890-ABCD-EF1234567890"
	if _, err := service.ImportCFDI(t.Context(), buildCFDI(uuid, "100.00"), ImportOptions{}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	_, err := service.ImportCFDI(t.Context(), buildCFDI(uuid, "999.00"), ImportOptions{})
	if !errors.Is(err, domain.ErrPurchaseConflict) {
		t.Fatalf("error = %v, want ErrPurchaseConflict", err)
	}
}
func TestImportCFDIRejectsDocumentWithoutFiscalStamp(t *testing.T) {
	service, _, _ := newTestService()
	unstamped := []byte(`<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0" Fecha="2026-01-01T00:00:00" SubTotal="1" Moneda="MXN" Total="1"><cfdi:Emisor Rfc="ABC010101AA1"/></cfdi:Comprobante>`)
	_, err := service.ImportCFDI(t.Context(), unstamped, ImportOptions{})
	var verr domain.ValidationError
	if !errors.As(err, &verr) || verr.Field != "cfdi_uuid" {
		t.Fatalf("error = %v, want ValidationError on cfdi_uuid", err)
	}
}
func TestImportCFDIRejectsInvalidXML(t *testing.T) {
	service, _, _ := newTestService()
	_, err := service.ImportCFDI(t.Context(), []byte("not xml"), ImportOptions{})
	var verr domain.ValidationError
	if !errors.As(err, &verr) || verr.Field != "xml_content" {
		t.Fatalf("error = %v, want ValidationError on xml_content", err)
	}
}
func TestImportCFDIRejectsEmptyInput(t *testing.T) {
	service, _, _ := newTestService()
	_, err := service.ImportCFDI(t.Context(), nil, ImportOptions{})
	var verr domain.ValidationError
	if !errors.As(err, &verr) || verr.Field != "xml_content" {
		t.Fatalf("error = %v, want ValidationError on xml_content", err)
	}
}
func TestImportCFDIRejectsBranchNotOwnedBySupplier(t *testing.T) {
	service, _, suppliers := newTestService()
	other, err := suppliers.CreateSupplier(t.Context(), supplierdomain.SupplierDetails{LegalName: "Other", TaxIdentifier: "XYZ010101AA1"})
	if err != nil {
		t.Fatalf("seed other supplier: %v", err)
	}
	suppliers.branches[900] = supplierdomain.Branch{ID: 900, SupplierID: other.ID}
	_, err = service.ImportCFDI(t.Context(), buildCFDI("ABCDEF12-3456-7890-ABCD-EF1234567890", "100.00"), ImportOptions{BranchID: 900})
	if !errors.Is(err, supplierdomain.ErrBranchNotFound) {
		t.Fatalf("error = %v, want ErrBranchNotFound", err)
	}
}

func TestImportCFDIReusesExistingResourceLinkForKnownSKU(t *testing.T) {
	service, repo, _ := newTestService()
	first, err := service.ImportCFDI(t.Context(), buildCFDI("ABCDEF12-3456-7890-ABCD-EF1234567890", "100.00"), ImportOptions{})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	supplierProductID := *first.Lines[0].SupplierProductID
	if _, err := service.ConfirmMapping(t.Context(), domain.ConfirmMappingCommand{SupplierProductID: supplierProductID, ResourceID: 555, Decision: validMappingDecision()}); err != nil {
		t.Fatalf("confirm supplier product mapping: %v", err)
	}
	second, err := service.ImportCFDI(t.Context(), buildCFDI("11111111-2222-3333-4444-555555555555", "100.00"), ImportOptions{})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Lines[0].DerivedStatus != domain.LinkLinked {
		t.Fatalf("effective status = %q, want VINCULADO because the SKU is already known", second.Lines[0].DerivedStatus)
	}
	if *second.Lines[0].SupplierProductID != supplierProductID {
		t.Fatal("expected the same reused supplier product across purchases")
	}
	_ = repo
}

func TestSemanticMappingAndOverrideCommandsAreRetroactive(t *testing.T) {
	service, _, _ := newTestService()
	first, err := service.ImportCFDI(t.Context(), buildCFDI("ABCDEF12-3456-7890-ABCD-EF1234567890", "100.00"), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	id := *first.Lines[0].SupplierProductID
	decision := validMappingDecision()
	confirmed, err := service.ConfirmMapping(t.Context(), domain.ConfirmMappingCommand{SupplierProductID: id, ResourceID: 7, Decision: decision})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.MappingRevision != 1 {
		t.Fatalf("revision = %d, want 1", confirmed.MappingRevision)
	}
	lines, err := service.ListPurchaseLines(t.Context(), first.Purchase.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].DerivedStatus != domain.LinkLinked {
		t.Fatalf("effective status = %q, want linked", lines[0].DerivedStatus)
	}
	if _, err := service.SetResolutionOverride(t.Context(), domain.SetResolutionOverrideCommand{
		LineID: lines[0].ID, Override: domain.LinkNotApplicable, ExpectedRevision: lines[0].ResolutionRevision,
		Actor: "tester", Reason: "not applicable",
	}); err != nil {
		t.Fatal(err)
	}
	lines, _ = service.ListPurchaseLines(t.Context(), first.Purchase.ID)
	if lines[0].DerivedStatus != domain.LinkNotApplicable {
		t.Fatalf("override status = %q", lines[0].DerivedStatus)
	}
	if _, err := service.SetResolutionOverride(t.Context(), domain.SetResolutionOverrideCommand{
		LineID: lines[0].ID, Override: domain.LinkStatusNone, ExpectedRevision: 1,
		Actor: "tester", Reason: "clear after review",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CorrectMapping(t.Context(), domain.CorrectMappingCommand{SupplierProductID: id, ExpectedCurrentResource: 7, ResourceID: 8, ExpectedRevision: confirmed.MappingRevision, Decision: decision}); err != nil {
		t.Fatal(err)
	}
	entries, err := service.ListMappingAudit(t.Context(), id, domain.ListCriteria{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want confirm and correct", len(entries))
	}
	if _, err := service.ExceptionalUnlink(t.Context(), domain.ExceptionalUnlinkCommand{SupplierProductID: id, ExpectedCurrentResource: 8, ExpectedRevision: 2, Decision: decision}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmMapping(t.Context(), domain.ConfirmMappingCommand{SupplierProductID: id, ResourceID: 9, ExpectedRevision: 0, Decision: decision}); err == nil {
		t.Fatal("stale confirmation unexpectedly succeeded")
	}
}

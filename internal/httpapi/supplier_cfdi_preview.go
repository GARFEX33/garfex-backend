package httpapi

import (
	"net/http"

	"github.com/GARFEX33/garfex-costos-unitarios/cfdicore"
	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

type supplierCFDIPreviewResponse struct {
	Draft cfdiSupplierDraftResponse `json:"draft"`
	// Existing is the supplier already owning the Emisor RFC, active or not,
	// or null when the RFC is new.
	Existing *supplierResponse `json:"existing"`
}

// serveSupplierCFDIPreview reads a CFDI and reports what "load supplier from
// XML" would do: the supplier draft taken from the Emisor and the supplier
// that already owns that RFC, if any. It is read-only. Creating stays with
// POST /v1/suppliers, whose unique RFC index remains the authority if another
// request wins the race after this preview.
func serveSupplierCFDIPreview(w http.ResponseWriter, r *http.Request, reader SupplierReader) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
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

	draft := cfdiSupplierDraft(invoice)
	response := supplierCFDIPreviewResponse{Draft: draft}
	existing, err := reader.GetSupplierByTaxIdentifier(r.Context(), draft.TaxIdentifier)
	switch {
	case err == nil:
		mapped := mapSupplier(existing)
		response.Existing = &mapped
	case !suppliercore.IsCode(err, suppliercore.NotFound):
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

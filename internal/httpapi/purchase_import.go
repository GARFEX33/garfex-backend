package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

type purchaseImportResponse struct {
	Purchase       purchaseResponse       `json:"purchase"`
	Lines          []purchaseLineResponse `json:"lines"`
	AlreadyExisted bool                   `json:"alreadyExisted"`
}

// servePurchaseImport imports one CFDI 4.0 purchase document, sent as
// multipart/form-data: "file" (the XML), "actor" (required), and an
// optional "branchId".
func servePurchaseImport(w http.ResponseWriter, r *http.Request, writer PurchaseWriter) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}
	req, ok := readPurchaseImportUpload(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	result, err := writer.ImportPurchase(r.Context(), req)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	status := http.StatusCreated
	if result.AlreadyExisted {
		status = http.StatusOK
	}
	writeJSON(w, status, purchaseImportResponse{
		Purchase:       mapPurchase(result.Purchase),
		Lines:          mapPurchaseLines(result.Lines),
		AlreadyExisted: result.AlreadyExisted,
	})
}

// readPurchaseImportUpload parses the multipart form. The router already
// bounds the body to maxRequestBodyBytes, so every read below is bounded
// too.
func readPurchaseImportUpload(r *http.Request) (purchasecore.ImportRequest, bool) {
	if err := r.ParseMultipartForm(maxRequestBodyBytes); err != nil {
		return purchasecore.ImportRequest{}, false
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("file")
	if err != nil {
		return purchasecore.ImportRequest{}, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		return purchasecore.ImportRequest{}, false
	}

	req := purchasecore.ImportRequest{
		Actor:    r.FormValue("actor"),
		XML:      data,
		Filename: header.Filename,
	}
	if branchID := r.FormValue("branchId"); branchID != "" {
		id, err := strconv.ParseInt(branchID, 10, 64)
		if err != nil || id <= 0 {
			return purchasecore.ImportRequest{}, false
		}
		req.BranchID = id
	}
	return req, true
}

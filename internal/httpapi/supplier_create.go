package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

// SupplierWriter is the narrow Core capability required by the supplier
// create and update routes.
type SupplierWriter interface {
	CreateSupplier(context.Context, suppliercore.SupplierWriteRequest) (suppliercore.Supplier, error)
	UpdateSupplier(context.Context, suppliercore.SupplierUpdateRequest) (suppliercore.Supplier, error)
}

type supplierCreateRequest struct {
	Actor         string `json:"actor"`
	TradeName     string `json:"tradeName"`
	LegalName     string `json:"legalName"`
	TaxIdentifier string `json:"taxIdentifier"`
	Website       string `json:"website"`
	Notes         string `json:"notes"`
}

type supplierResponse struct {
	ID            string `json:"id"`
	TradeName     string `json:"tradeName"`
	LegalName     string `json:"legalName"`
	TaxIdentifier string `json:"taxIdentifier"`
	Website       string `json:"website"`
	Notes         string `json:"notes"`
	Active        bool   `json:"active"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func serveCreateSupplier(w http.ResponseWriter, r *http.Request, writer SupplierWriter) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if writer == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier writer unavailable"))
		return
	}
	var body supplierCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid request body"))
		return
	}
	supplier, err := writer.CreateSupplier(r.Context(), suppliercore.SupplierWriteRequest{
		Actor:         body.Actor,
		TradeName:     body.TradeName,
		LegalName:     body.LegalName,
		TaxIdentifier: body.TaxIdentifier,
		Website:       body.Website,
		Notes:         body.Notes,
	})
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mapSupplier(supplier))
}

func mapSupplier(s suppliercore.Supplier) supplierResponse {
	return supplierResponse{
		ID:            strconv.FormatInt(s.ID, 10),
		TradeName:     s.TradeName,
		LegalName:     s.LegalName,
		TaxIdentifier: s.TaxIdentifier,
		Website:       s.Website,
		Notes:         s.Notes,
		Active:        s.Active,
		CreatedAt:     s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:     s.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeSupplierError(w http.ResponseWriter, err error) {
	status, message := supplierError(err)
	writeJSON(w, status, errorResponse{Error: message})
}

func supplierError(err error) (int, string) {
	switch suppliercore.Code(err) {
	case suppliercore.InvalidArgument:
		return http.StatusBadRequest, "invalid request"
	case suppliercore.NotFound:
		return http.StatusNotFound, "not found"
	case suppliercore.Conflict:
		return http.StatusConflict, "conflict"
	case suppliercore.Validation:
		return http.StatusUnprocessableEntity, "validation failed"
	case suppliercore.Internal:
		return http.StatusInternalServerError, "internal server error"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

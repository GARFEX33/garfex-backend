package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/GARFEX33/garfex-backend/suppliercore"
)

const (
	defaultSupplierLimit = 50
	maximumSupplierLimit = 50
)

// SupplierReader is the narrow Core capability required by the supplier
// search and detail routes, including the nested branch and contact routes.
type SupplierReader interface {
	GetSupplier(context.Context, int64) (suppliercore.Supplier, error)
	GetSupplierByTaxIdentifier(context.Context, string) (suppliercore.Supplier, error)
	SearchSuppliers(context.Context, suppliercore.SupplierQuery) (suppliercore.SupplierPage, error)
	ListBranches(context.Context, suppliercore.BranchQuery) (suppliercore.BranchPage, error)
	GetBranch(context.Context, suppliercore.BranchKey) (suppliercore.Branch, error)
	ListContacts(context.Context, suppliercore.ContactQuery) (suppliercore.ContactPage, error)
	GetContact(context.Context, suppliercore.ContactKey) (suppliercore.Contact, error)
}

type supplierPageResponse struct {
	Suppliers   []supplierResponse `json:"suppliers"`
	HasPrevious bool               `json:"hasPrevious"`
	HasNext     bool               `json:"hasNext"`
}

func serveSuppliers(w http.ResponseWriter, r *http.Request, writer SupplierWriter, reader SupplierReader) {
	switch r.Method {
	case http.MethodPost:
		serveCreateSupplier(w, r, writer)
	case http.MethodGet:
		serveSupplierSearch(w, r, reader)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
	}
}

func serveSupplierSearch(w http.ResponseWriter, r *http.Request, reader SupplierReader) {
	query, ok := supplierSearchQuery(r)
	if !ok {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid supplier search query"))
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
		return
	}
	page, err := reader.SearchSuppliers(r.Context(), query)
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplierPage(page))
}

func supplierSearchQuery(r *http.Request) (suppliercore.SupplierQuery, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return suppliercore.SupplierQuery{}, false
	}
	query := suppliercore.SupplierQuery{
		Scope: suppliercore.ScopeActive,
		Text:  values.Get("text"),
		Limit: defaultSupplierLimit,
	}
	if _, present := values["scope"]; present {
		query.Scope = suppliercore.LifecycleScope(values.Get("scope"))
		switch query.Scope {
		case suppliercore.ScopeActive, suppliercore.ScopeInactive, suppliercore.ScopeAll:
		default:
			return suppliercore.SupplierQuery{}, false
		}
	}
	if _, present := values["limit"]; present {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit <= 0 || limit > maximumSupplierLimit {
			return suppliercore.SupplierQuery{}, false
		}
		query.Limit = limit
	}
	if _, present := values["offset"]; present {
		offset, err := strconv.Atoi(values.Get("offset"))
		if err != nil || offset < 0 {
			return suppliercore.SupplierQuery{}, false
		}
		query.Offset = offset
	}
	return query, true
}

func mapSupplierPage(page suppliercore.SupplierPage) supplierPageResponse {
	suppliers := make([]supplierResponse, len(page.Suppliers))
	for i, s := range page.Suppliers {
		suppliers[i] = mapSupplier(s)
	}
	return supplierPageResponse{Suppliers: suppliers, HasPrevious: page.HasPrevious, HasNext: page.HasNext}
}

func serveSupplierDetail(w http.ResponseWriter, r *http.Request, reader SupplierReader, writer SupplierWriter, id string) {
	switch r.Method {
	case http.MethodGet:
		serveSupplierGet(w, r, reader, id)
	case http.MethodPut:
		serveSupplierUpdate(w, r, writer, id)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
	}
}

func serveSupplierGet(w http.ResponseWriter, r *http.Request, reader SupplierReader, id string) {
	supplierID, ok := positiveInt64(id)
	if !ok {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid supplier id"))
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
		return
	}
	supplier, err := reader.GetSupplier(r.Context(), supplierID)
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplier(supplier))
}

func serveSupplierUpdate(w http.ResponseWriter, r *http.Request, writer SupplierWriter, id string) {
	supplierID, ok := positiveInt64(id)
	if !ok {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid supplier id"))
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
	supplier, err := writer.UpdateSupplier(r.Context(), suppliercore.SupplierUpdateRequest{
		Actor:         body.Actor,
		ID:            supplierID,
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
	writeJSON(w, http.StatusOK, mapSupplier(supplier))
}

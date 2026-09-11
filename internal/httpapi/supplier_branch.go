package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

type branchResponse struct {
	ID           string `json:"id"`
	SupplierID   string `json:"supplierId"`
	Name         string `json:"name"`
	Reference    string `json:"reference"`
	City         string `json:"city"`
	State        string `json:"state"`
	Country      string `json:"country"`
	Address      string `json:"address"`
	GeneralPhone string `json:"generalPhone"`
	GeneralEmail string `json:"generalEmail"`
	Notes        string `json:"notes"`
	Active       bool   `json:"active"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type branchPageResponse struct {
	Branches    []branchResponse `json:"branches"`
	HasPrevious bool             `json:"hasPrevious"`
	HasNext     bool             `json:"hasNext"`
}

func mapBranch(b suppliercore.Branch) branchResponse {
	return branchResponse{
		ID:           strconv.FormatInt(b.ID, 10),
		SupplierID:   strconv.FormatInt(b.SupplierID, 10),
		Name:         b.Name,
		Reference:    b.Reference,
		City:         b.City,
		State:        b.State,
		Country:      b.Country,
		Address:      b.Address,
		GeneralPhone: b.GeneralPhone,
		GeneralEmail: b.GeneralEmail,
		Notes:        b.Notes,
		Active:       b.Active,
		CreatedAt:    b.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:    b.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapBranchPage(page suppliercore.BranchPage) branchPageResponse {
	branches := make([]branchResponse, len(page.Branches))
	for i, b := range page.Branches {
		branches[i] = mapBranch(b)
	}
	return branchPageResponse{Branches: branches, HasPrevious: page.HasPrevious, HasNext: page.HasNext}
}

func serveBranchList(w http.ResponseWriter, r *http.Request, reader SupplierReader, supplierIDStr string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	supplierID, ok := positiveInt64(supplierIDStr)
	if !ok {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid supplier id"))
		return
	}
	query, ok := branchListQuery(r, supplierID)
	if !ok {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid branch list query"))
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
		return
	}
	page, err := reader.ListBranches(r.Context(), query)
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapBranchPage(page))
}

func branchListQuery(r *http.Request, supplierID int64) (suppliercore.BranchQuery, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return suppliercore.BranchQuery{}, false
	}
	query := suppliercore.BranchQuery{
		SupplierID: supplierID,
		Scope:      suppliercore.ScopeActive,
		Text:       values.Get("text"),
		Limit:      defaultSupplierLimit,
	}
	if _, present := values["scope"]; present {
		query.Scope = suppliercore.LifecycleScope(values.Get("scope"))
		switch query.Scope {
		case suppliercore.ScopeActive, suppliercore.ScopeInactive, suppliercore.ScopeAll:
		default:
			return suppliercore.BranchQuery{}, false
		}
	}
	if _, present := values["limit"]; present {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit <= 0 || limit > maximumSupplierLimit {
			return suppliercore.BranchQuery{}, false
		}
		query.Limit = limit
	}
	if _, present := values["offset"]; present {
		offset, err := strconv.Atoi(values.Get("offset"))
		if err != nil || offset < 0 {
			return suppliercore.BranchQuery{}, false
		}
		query.Offset = offset
	}
	return query, true
}

func serveBranchDetail(w http.ResponseWriter, r *http.Request, reader SupplierReader, supplierIDStr, branchIDStr string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	supplierID, ok := positiveInt64(supplierIDStr)
	branchID, ok2 := positiveInt64(branchIDStr)
	if !ok || !ok2 {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid branch id"))
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
		return
	}
	branch, err := reader.GetBranch(r.Context(), suppliercore.BranchKey{SupplierID: supplierID, BranchID: branchID})
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapBranch(branch))
}

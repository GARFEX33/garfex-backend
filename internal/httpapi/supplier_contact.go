package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/suppliercore"
)

type contactResponse struct {
	ID         string `json:"id"`
	SupplierID string `json:"supplierId"`
	BranchID   string `json:"branchId,omitempty"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Phone      string `json:"phone"`
	Mobile     string `json:"mobile"`
	Email      string `json:"email"`
	Notes      string `json:"notes"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type contactPageResponse struct {
	Contacts    []contactResponse `json:"contacts"`
	HasPrevious bool              `json:"hasPrevious"`
	HasNext     bool              `json:"hasNext"`
}

func mapContact(c suppliercore.Contact) contactResponse {
	response := contactResponse{
		ID:         strconv.FormatInt(c.ID, 10),
		SupplierID: strconv.FormatInt(c.SupplierID, 10),
		Name:       c.Name,
		Role:       c.Role,
		Phone:      c.Phone,
		Mobile:     c.Mobile,
		Email:      c.Email,
		Notes:      c.Notes,
		Active:     c.Active,
		CreatedAt:  c.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:  c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.BranchID != nil {
		response.BranchID = strconv.FormatInt(*c.BranchID, 10)
	}
	return response
}

func mapContactPage(page suppliercore.ContactPage) contactPageResponse {
	contacts := make([]contactResponse, len(page.Contacts))
	for i, c := range page.Contacts {
		contacts[i] = mapContact(c)
	}
	return contactPageResponse{Contacts: contacts, HasPrevious: page.HasPrevious, HasNext: page.HasNext}
}

func serveContactList(w http.ResponseWriter, r *http.Request, reader SupplierReader, supplierIDStr string) {
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
	query, ok := contactListQuery(r, supplierID)
	if !ok {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid contact list query"))
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
		return
	}
	page, err := reader.ListContacts(r.Context(), query)
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapContactPage(page))
}

func contactListQuery(r *http.Request, supplierID int64) (suppliercore.ContactQuery, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return suppliercore.ContactQuery{}, false
	}
	query := suppliercore.ContactQuery{
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
			return suppliercore.ContactQuery{}, false
		}
	}
	if _, present := values["branchId"]; present {
		branchID, ok := positiveInt64(values.Get("branchId"))
		if !ok {
			return suppliercore.ContactQuery{}, false
		}
		query.BranchID = &branchID
	}
	if _, present := values["limit"]; present {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit <= 0 || limit > maximumSupplierLimit {
			return suppliercore.ContactQuery{}, false
		}
		query.Limit = limit
	}
	if _, present := values["offset"]; present {
		offset, err := strconv.Atoi(values.Get("offset"))
		if err != nil || offset < 0 {
			return suppliercore.ContactQuery{}, false
		}
		query.Offset = offset
	}
	return query, true
}

func serveContactDetail(w http.ResponseWriter, r *http.Request, reader SupplierReader, supplierIDStr, contactIDStr string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	supplierID, ok := positiveInt64(supplierIDStr)
	contactID, ok2 := positiveInt64(contactIDStr)
	if !ok || !ok2 {
		writeSupplierError(w, suppliercore.NewError(suppliercore.InvalidArgument, "invalid contact id"))
		return
	}
	if reader == nil {
		writeSupplierError(w, suppliercore.NewError(suppliercore.Internal, "supplier reader unavailable"))
		return
	}
	contact, err := reader.GetContact(r.Context(), suppliercore.ContactKey{SupplierID: supplierID, ContactID: contactID})
	if err != nil {
		writeSupplierError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapContact(contact))
}

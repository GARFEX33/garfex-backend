package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GARFEX33/garfex-backend/purchasecore"
)

type purchasePageResponse struct {
	Purchases   []purchaseResponse `json:"purchases"`
	HasPrevious bool               `json:"hasPrevious"`
	HasNext     bool               `json:"hasNext"`
}

type supplierProductPageResponse struct {
	Products    []supplierProductResponse `json:"products"`
	HasPrevious bool                      `json:"hasPrevious"`
	HasNext     bool                      `json:"hasNext"`
}

type purchaseLineHistoryPageResponse struct {
	History     []purchaseLineHistoryResponse `json:"history"`
	HasPrevious bool                          `json:"hasPrevious"`
	HasNext     bool                          `json:"hasNext"`
}

type purchaseLineWorkbenchPageResponse struct {
	Lines       []purchaseLineWorkbenchRowResponse `json:"lines"`
	HasPrevious bool                               `json:"hasPrevious"`
	HasNext     bool                               `json:"hasNext"`
}

func servePurchaseDetail(w http.ResponseWriter, r *http.Request, reader PurchaseReader, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	purchaseID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid purchase id"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	purchase, err := reader.GetPurchase(r.Context(), purchaseID)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapPurchase(purchase))
}

func servePurchaseByUUID(w http.ResponseWriter, r *http.Request, reader PurchaseReader, uuid string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	purchase, err := reader.GetPurchaseByUUID(r.Context(), uuid)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapPurchase(purchase))
}

func servePurchaseLines(w http.ResponseWriter, r *http.Request, reader PurchaseReader, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	purchaseID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid purchase id"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	lines, err := reader.ListPurchaseLines(r.Context(), purchaseID)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapPurchaseLines(lines))
}

func servePurchaseLineWorkbench(w http.ResponseWriter, r *http.Request, reader PurchaseReader) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	query, ok := purchaseLineWorkbenchQuery(r)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid purchase line query"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	page, err := reader.ListPurchaseLinesWorkbench(r.Context(), query)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	lines := make([]purchaseLineWorkbenchRowResponse, len(page.Rows))
	for i, row := range page.Rows {
		lines[i] = mapPurchaseLineWorkbenchRow(row)
	}
	writeJSON(w, http.StatusOK, purchaseLineWorkbenchPageResponse{
		Lines:       lines,
		HasPrevious: page.HasPrevious,
		HasNext:     page.HasNext,
	})
}

func purchaseLineWorkbenchQuery(r *http.Request) (purchasecore.PurchaseLineQuery, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return purchasecore.PurchaseLineQuery{}, false
	}
	query := purchasecore.PurchaseLineQuery{Limit: defaultPurchaseLimit}
	if raw, present := values["supplierId"]; present {
		if len(raw) == 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		supplierID, ok := positiveInt64(raw[0])
		if !ok {
			return purchasecore.PurchaseLineQuery{}, false
		}
		query.SupplierID = &supplierID
	}
	if raw, present := values["status"]; present {
		if len(raw) == 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		query.EffectiveStatus = purchasecore.LinkStatus(raw[0])
		if !validPurchaseLineStatus(query.EffectiveStatus) {
			return purchasecore.PurchaseLineQuery{}, false
		}
	}
	if raw, present := values["dateFrom"]; present {
		if len(raw) == 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		dateFrom, ok := parsePurchaseLineDate(raw[0], false)
		if !ok {
			return purchasecore.PurchaseLineQuery{}, false
		}
		query.DateFrom = dateFrom
	}
	if raw, present := values["dateTo"]; present {
		if len(raw) == 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		dateTo, ok := parsePurchaseLineDate(raw[0], true)
		if !ok {
			return purchasecore.PurchaseLineQuery{}, false
		}
		query.DateTo = dateTo
	}
	if query.DateFrom != nil && query.DateTo != nil && query.DateFrom.After(*query.DateTo) {
		return purchasecore.PurchaseLineQuery{}, false
	}
	query.InvoiceText = values.Get("invoice")
	query.SupplierSKU = values.Get("supplierSku")
	query.Description = values.Get("description")
	if raw, present := values["limit"]; present {
		if len(raw) == 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		limit, err := strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > maximumPurchaseLimit {
			return purchasecore.PurchaseLineQuery{}, false
		}
		query.Limit = limit
	}
	if raw, present := values["offset"]; present {
		if len(raw) == 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		offset, err := strconv.Atoi(raw[0])
		if err != nil || offset < 0 {
			return purchasecore.PurchaseLineQuery{}, false
		}
		query.Offset = offset
	}
	return query, true
}

func parsePurchaseLineDate(value string, endOfDay bool) (*time.Time, bool) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, false
	}
	if endOfDay {
		date = time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
	}
	return &date, true
}

func validPurchaseLineStatus(status purchasecore.LinkStatus) bool {
	switch status {
	case purchasecore.LinkPending, purchasecore.LinkLinked, purchasecore.LinkSuspended, purchasecore.LinkNotApplicable, purchasecore.LinkConflict:
		return true
	default:
		return false
	}
}

func serveSupplierPurchases(w http.ResponseWriter, r *http.Request, reader PurchaseReader, supplierID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	id, ok := positiveInt64(supplierID)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier id"))
		return
	}
	criteria, ok := purchaseListCriteria(r)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid list query"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	page, err := reader.ListPurchasesBySupplier(r.Context(), id, criteria)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	purchases := make([]purchaseResponse, len(page.Purchases))
	for i, p := range page.Purchases {
		purchases[i] = mapPurchase(p)
	}
	writeJSON(w, http.StatusOK, purchasePageResponse{Purchases: purchases, HasPrevious: page.HasPrevious, HasNext: page.HasNext})
}

func serveSupplierProductDetail(w http.ResponseWriter, r *http.Request, reader PurchaseReader, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	productID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier product id"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	product, err := reader.GetSupplierProduct(r.Context(), productID)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplierProduct(product))
}

func serveSupplierProductFind(w http.ResponseWriter, r *http.Request, reader PurchaseReader, supplierID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	id, ok := positiveInt64(supplierID)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier id"))
		return
	}
	sku := r.URL.Query().Get("sku")
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	product, err := reader.FindSupplierProduct(r.Context(), id, sku)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplierProduct(product))
}

func serveSupplierProductList(w http.ResponseWriter, r *http.Request, reader PurchaseReader, supplierID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	id, ok := positiveInt64(supplierID)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier id"))
		return
	}
	criteria, ok := purchaseListCriteria(r)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid list query"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	page, err := reader.ListSupplierProducts(r.Context(), id, criteria)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	products := make([]supplierProductResponse, len(page.Products))
	for i, p := range page.Products {
		products[i] = mapSupplierProduct(p)
	}
	writeJSON(w, http.StatusOK, supplierProductPageResponse{Products: products, HasPrevious: page.HasPrevious, HasNext: page.HasNext})
}

func serveResourcePurchaseHistory(w http.ResponseWriter, r *http.Request, reader PurchaseReader, resourceID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	id, ok := positiveInt64(resourceID)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid resource id"))
		return
	}
	criteria, ok := purchaseListCriteria(r)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid list query"))
		return
	}
	if reader == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase reader unavailable"))
		return
	}
	page, err := reader.ListPurchaseLinesByResource(r.Context(), id, criteria)
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	history := make([]purchaseLineHistoryResponse, len(page.History))
	for i, h := range page.History {
		history[i] = mapPurchaseLineHistory(h)
	}
	writeJSON(w, http.StatusOK, purchaseLineHistoryPageResponse{History: history, HasPrevious: page.HasPrevious, HasNext: page.HasNext})
}

func purchaseListCriteria(r *http.Request) (purchasecore.ListCriteria, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return purchasecore.ListCriteria{}, false
	}
	criteria := purchasecore.ListCriteria{Limit: defaultPurchaseLimit}
	if _, present := values["limit"]; present {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit <= 0 || limit > maximumPurchaseLimit {
			return purchasecore.ListCriteria{}, false
		}
		criteria.Limit = limit
	}
	if _, present := values["offset"]; present {
		offset, err := strconv.Atoi(values.Get("offset"))
		if err != nil || offset < 0 {
			return purchasecore.ListCriteria{}, false
		}
		criteria.Offset = offset
	}
	return criteria, true
}

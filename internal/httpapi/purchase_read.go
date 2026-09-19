package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
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

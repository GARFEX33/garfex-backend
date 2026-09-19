package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

type linkSupplierProductRequest struct {
	Actor      string `json:"actor"`
	ResourceID string `json:"resourceId"`
}

type unlinkSupplierProductRequest struct {
	Actor string `json:"actor"`
}

type setLinkStatusRequest struct {
	Actor  string `json:"actor"`
	Status string `json:"status"`
}

func serveSupplierProductLink(w http.ResponseWriter, r *http.Request, writer PurchaseWriter, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	productID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier product id"))
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}
	var body linkSupplierProductRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid request body"))
		return
	}
	resourceID, ok := positiveInt64(body.ResourceID)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid resource id"))
		return
	}
	product, err := writer.LinkSupplierProductToResource(r.Context(), purchasecore.LinkSupplierProductRequest{
		Actor:             body.Actor,
		SupplierProductID: productID,
		ResourceID:        resourceID,
	})
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplierProduct(product))
}

func serveSupplierProductUnlink(w http.ResponseWriter, r *http.Request, writer PurchaseWriter, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	productID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier product id"))
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}
	var body unlinkSupplierProductRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid request body"))
		return
	}
	product, err := writer.UnlinkSupplierProduct(r.Context(), purchasecore.UnlinkSupplierProductRequest{
		Actor:             body.Actor,
		SupplierProductID: productID,
	})
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplierProduct(product))
}

func servePurchaseLineLinkStatus(w http.ResponseWriter, r *http.Request, writer PurchaseWriter, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	lineID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid purchase line id"))
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}
	var body setLinkStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid request body"))
		return
	}
	line, err := writer.SetPurchaseLineLinkStatus(r.Context(), purchasecore.SetPurchaseLineLinkStatusRequest{
		Actor:          body.Actor,
		PurchaseLineID: lineID,
		Status:         purchasecore.LinkStatus(body.Status),
	})
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapPurchaseLine(line))
}

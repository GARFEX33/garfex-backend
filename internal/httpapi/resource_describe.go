package httpapi

import (
	"net/http"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type resourceDescriptionResponse struct {
	Description string `json:"description"`
}

func serveResourceDescribe(w http.ResponseWriter, r *http.Request, reader ResourceReader, classCode, identityV1 string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource reader unavailable"))
		return
	}
	description, err := reader.DescribeResource(r.Context(), resourcecore.ResourceKey{ClassCode: classCode, IdentityV1: identityV1})
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resourceDescriptionResponse{Description: description})
}

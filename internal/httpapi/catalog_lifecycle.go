package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type catalogLifecycleRequest struct {
	Actor            string `json:"actor"`
	ExpectedRevision string `json:"expectedRevision"`
}

// serveCatalogLifecycle handles the deactivate/reactivate action paths.
// action is already validated by the router to be exactly "deactivate" or
// "reactivate".
func serveCatalogLifecycle(w http.ResponseWriter, r *http.Request, writer CatalogWriter, kind, id, action string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	req, ok := parseCatalogLifecycleRequest(w, r, kind, id)
	if !ok {
		return
	}
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog writer unavailable"))
		return
	}
	var record resourcecore.CatalogRecord
	var err error
	switch action {
	case "deactivate":
		record, err = writer.DeactivateCatalog(r.Context(), req)
	case "reactivate":
		record, err = writer.ReactivateCatalog(r.Context(), req)
	}
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapCatalogRecord(record)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// serveCatalogHardDelete permanently removes one catalog record. Unlike
// deactivate/reactivate, there is no record left to read back on success.
func serveCatalogHardDelete(w http.ResponseWriter, r *http.Request, writer CatalogWriter, kind, id string) {
	req, ok := parseCatalogLifecycleRequest(w, r, kind, id)
	if !ok {
		return
	}
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog writer unavailable"))
		return
	}
	if err := writer.HardDeleteCatalog(r.Context(), req); err != nil {
		writeCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseCatalogLifecycleRequest(w http.ResponseWriter, r *http.Request, kind, id string) (resourcecore.CatalogLifecycleRequest, bool) {
	key, ok := catalogKey(kind, id)
	if !ok {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid catalog id"))
		return resourcecore.CatalogLifecycleRequest{}, false
	}
	var body catalogLifecycleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return resourcecore.CatalogLifecycleRequest{}, false
	}
	expectedRevision, err := strconv.ParseUint(body.ExpectedRevision, 10, 64)
	if err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid expected revision"))
		return resourcecore.CatalogLifecycleRequest{}, false
	}
	return resourcecore.CatalogLifecycleRequest{Actor: body.Actor, Kind: key.Kind, ID: key.ID, ExpectedRevision: expectedRevision}, true
}

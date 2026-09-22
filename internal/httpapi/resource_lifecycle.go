package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

type resourceLifecycleRequest struct {
	Actor            string `json:"actor"`
	ExpectedRevision string `json:"expectedRevision"`
}

// serveResourceLifecycle handles the deactivate/reactivate action paths.
// action is already validated by the router to be exactly "deactivate" or
// "reactivate".
func serveResourceLifecycle(w http.ResponseWriter, r *http.Request, writer ResourceWriter, id, action string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	resourceID, ok := positiveInt64(id)
	if !ok {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid resource id"))
		return
	}
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource writer unavailable"))
		return
	}
	var body resourceLifecycleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	expectedRevision, err := strconv.ParseUint(body.ExpectedRevision, 10, 64)
	if err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid expected revision"))
		return
	}
	req := resourcecore.ResourceLifecycleRequest{Actor: body.Actor, ID: resourceID, ExpectedRevision: expectedRevision}

	var resource resourcecore.Resource
	switch action {
	case "deactivate":
		resource, err = writer.DeactivateResource(r.Context(), req)
	case "reactivate":
		resource, err = writer.ReactivateResource(r.Context(), req)
	}
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapResourceResponse(resource)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type resourceUpdateRequest struct {
	Actor            string                     `json:"actor"`
	ExpectedRevision string                     `json:"expectedRevision"`
	Scope            resourceScopeRequest       `json:"scope"`
	NaturalUnit      string                     `json:"naturalUnit"`
	Attributes       []resourceAttributeRequest `json:"attributes"`
}

// serveResourceUpdate handles the numeric-id write path. Unlike the
// natural-key read path, Core addresses resource writes by internal id and
// an expected revision for optimistic concurrency.
func serveResourceUpdate(w http.ResponseWriter, r *http.Request, writer ResourceWriter, id string) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
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
	var body resourceUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	req, err := mapResourceUpdateRequest(resourceID, body)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	resource, err := writer.UpdateResource(r.Context(), req)
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

func mapResourceUpdateRequest(id int64, body resourceUpdateRequest) (resourcecore.ResourceUpdateRequest, error) {
	expectedRevision, err := strconv.ParseUint(body.ExpectedRevision, 10, 64)
	if err != nil {
		return resourcecore.ResourceUpdateRequest{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid expected revision")
	}
	attributes := make([]resourcecore.AttributeValue, len(body.Attributes))
	for i, attr := range body.Attributes {
		value, err := parseCatalogValue(attr.Value)
		if err != nil {
			return resourcecore.ResourceUpdateRequest{}, err
		}
		attributes[i] = resourcecore.AttributeValue{Code: attr.Code, Value: value}
	}
	return resourcecore.ResourceUpdateRequest{
		Actor:            body.Actor,
		ID:               id,
		ExpectedRevision: expectedRevision,
		Scope: resourcecore.ResourceScope{
			ClassCode:  body.Scope.ClassCode,
			FamilyCode: body.Scope.FamilyCode,
			TypeCode:   body.Scope.TypeCode,
		},
		NaturalUnit: body.NaturalUnit,
		Attributes:  attributes,
	}, nil
}

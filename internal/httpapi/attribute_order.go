package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

// attributeOrderKeyResponse is the public representation of one Core
// AttributeOrderKey: the exact source level/code and characteristic code
// identifying a single ordered occurrence.
type attributeOrderKeyResponse struct {
	SourceLevel        string `json:"sourceLevel"`
	SourceCode         string `json:"sourceCode"`
	CharacteristicCode string `json:"characteristicCode"`
}

// attributeOrderResponse is the public representation of Core's
// ResourceAttributeOrder: the exact scope, the full ordered permutation,
// and an opaque OrderRevision used as the write's concurrency precondition.
// OrderRevision is never parsed or synthesized client-side; it is passed
// back verbatim on the next write's expectedOrderRevision.
type attributeOrderResponse struct {
	Scope             resourceScopeResponse       `json:"scope"`
	OrderedAttributes []attributeOrderKeyResponse `json:"orderedAttributes"`
	OrderRevision     string                      `json:"orderRevision"`
}

// attributeOrderKeyRequest is the public request shape mirroring
// attributeOrderKeyResponse for the PUT body's orderedAttributes.
type attributeOrderKeyRequest struct {
	SourceLevel        string `json:"sourceLevel"`
	SourceCode         string `json:"sourceCode"`
	CharacteristicCode string `json:"characteristicCode"`
}

// attributeOrderWriteRequest is the PUT body shape. Scope is not
// duplicated here: it comes from the route's path (typeCode) and query
// (classCode, familyCode), exactly like the GET side. Actor is audit
// metadata, not authentication, and ExpectedOrderRevision is an opaque
// pass-through string, never parsed.
type attributeOrderWriteRequest struct {
	Actor                 string                     `json:"actor"`
	ExpectedOrderRevision string                     `json:"expectedOrderRevision"`
	OrderedAttributes     []attributeOrderKeyRequest `json:"orderedAttributes"`
}

// serveTypeAttributeOrder handles both GET and PUT for
// /v1/types/{typeCode}/attributes/order, mirroring the multi-verb dispatch
// style used by serveCatalogDetail rather than a single-verb handler.
func serveTypeAttributeOrder(w http.ResponseWriter, r *http.Request, reader ResourceReader, writer ResourceWriter, typeCode string) {
	switch r.Method {
	case http.MethodGet:
		serveTypeAttributeOrderGet(w, r, reader, typeCode)
	case http.MethodPut:
		serveTypeAttributeOrderPut(w, r, writer, typeCode)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
	}
}

func serveTypeAttributeOrderGet(w http.ResponseWriter, r *http.Request, reader ResourceReader, typeCode string) {
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource reader unavailable"))
		return
	}
	scope := attributeOrderScope(r, typeCode)
	order, err := reader.AttributeOrderFor(r.Context(), scope)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapAttributeOrderResponse(order))
}

func serveTypeAttributeOrderPut(w http.ResponseWriter, r *http.Request, writer ResourceWriter, typeCode string) {
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource writer unavailable"))
		return
	}
	var body attributeOrderWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	scope := attributeOrderScope(r, typeCode)
	req, err := mapAttributeOrderWriteRequest(scope, body)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	order, err := writer.UpdateAttributeOrder(r.Context(), req)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapAttributeOrderResponse(order))
}

func attributeOrderScope(r *http.Request, typeCode string) resourcecore.ResourceScope {
	return resourcecore.ResourceScope{
		ClassCode: r.URL.Query().Get("classCode"), FamilyCode: r.URL.Query().Get("familyCode"), TypeCode: typeCode,
	}
}

// mapAttributeOrderWriteRequest validates the transport-level shape (actor,
// expectedOrderRevision, and orderedAttributes non-empty) before ever
// calling Core. This is distinct from and in addition to Core's own
// domain-level permutation validation, which is mapped to 422.
func mapAttributeOrderWriteRequest(scope resourcecore.ResourceScope, body attributeOrderWriteRequest) (resourcecore.AttributeOrderWriteRequest, error) {
	if strings.TrimSpace(body.Actor) == "" {
		return resourcecore.AttributeOrderWriteRequest{}, resourcecore.NewError(resourcecore.InvalidArgument, "actor is required")
	}
	if strings.TrimSpace(body.ExpectedOrderRevision) == "" {
		return resourcecore.AttributeOrderWriteRequest{}, resourcecore.NewError(resourcecore.InvalidArgument, "expected order revision is required")
	}
	if len(body.OrderedAttributes) == 0 {
		return resourcecore.AttributeOrderWriteRequest{}, resourcecore.NewError(resourcecore.InvalidArgument, "ordered attributes are required")
	}
	orderedAttributes := make([]resourcecore.AttributeOrderKey, len(body.OrderedAttributes))
	for i, key := range body.OrderedAttributes {
		orderedAttributes[i] = resourcecore.AttributeOrderKey{
			SourceLevel: key.SourceLevel, SourceCode: key.SourceCode, CharacteristicCode: key.CharacteristicCode,
		}
	}
	return resourcecore.AttributeOrderWriteRequest{
		Actor:                 body.Actor,
		Scope:                 scope,
		ExpectedOrderRevision: body.ExpectedOrderRevision,
		OrderedAttributes:     orderedAttributes,
	}, nil
}

func mapAttributeOrderResponse(order resourcecore.ResourceAttributeOrder) attributeOrderResponse {
	orderedAttributes := make([]attributeOrderKeyResponse, len(order.OrderedAttributes))
	for i, key := range order.OrderedAttributes {
		orderedAttributes[i] = attributeOrderKeyResponse{
			SourceLevel: key.SourceLevel, SourceCode: key.SourceCode, CharacteristicCode: key.CharacteristicCode,
		}
	}
	return attributeOrderResponse{
		Scope: resourceScopeResponse{
			ClassCode: order.Scope.ClassCode, FamilyCode: order.Scope.FamilyCode, TypeCode: order.Scope.TypeCode,
		},
		OrderedAttributes: orderedAttributes,
		OrderRevision:     order.OrderRevision,
	}
}

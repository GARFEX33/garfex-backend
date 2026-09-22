package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/GARFEX33/garfex-backend/resourcecore"
)

// ResourceWriter is the narrow Core capability required by the resource
// create, update, and lifecycle routes.
type ResourceWriter interface {
	CreateResource(context.Context, resourcecore.ResourceWriteRequest) (resourcecore.Resource, error)
	UpdateResource(context.Context, resourcecore.ResourceUpdateRequest) (resourcecore.Resource, error)
	DeactivateResource(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error)
	ReactivateResource(context.Context, resourcecore.ResourceLifecycleRequest) (resourcecore.Resource, error)
	UpdateAttributeOrder(context.Context, resourcecore.AttributeOrderWriteRequest) (resourcecore.ResourceAttributeOrder, error)
}

type resourceCreateRequest struct {
	Actor       string                     `json:"actor"`
	Scope       resourceScopeRequest       `json:"scope"`
	NaturalUnit string                     `json:"naturalUnit"`
	Attributes  []resourceAttributeRequest `json:"attributes"`
}

type resourceScopeRequest struct {
	ClassCode  string `json:"classCode"`
	FamilyCode string `json:"familyCode"`
	TypeCode   string `json:"typeCode"`
}

type resourceAttributeRequest struct {
	Code  string          `json:"code"`
	Value json.RawMessage `json:"value"`
}

type resourceResponse struct {
	ID          string                      `json:"id"`
	IdentityV1  string                      `json:"identityV1"`
	Scope       resourceScopeResponse       `json:"scope"`
	NaturalUnit string                      `json:"naturalUnit"`
	Active      bool                        `json:"active"`
	Revision    string                      `json:"revision"`
	Attributes  []resourceAttributeResponse `json:"attributes"`
}

type resourceScopeResponse struct {
	ClassCode  string `json:"classCode"`
	FamilyCode string `json:"familyCode"`
	TypeCode   string `json:"typeCode"`
}

type resourceAttributeResponse struct {
	Code  string               `json:"code"`
	Value catalogValueResponse `json:"value"`
}

func serveCreateResource(w http.ResponseWriter, r *http.Request, writer ResourceWriter) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource writer unavailable"))
		return
	}
	var body resourceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	req, err := mapResourceCreateRequest(body)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	resource, err := writer.CreateResource(r.Context(), req)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapResourceResponse(resource)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func mapResourceCreateRequest(body resourceCreateRequest) (resourcecore.ResourceWriteRequest, error) {
	attributes := make([]resourcecore.AttributeValue, len(body.Attributes))
	for i, attr := range body.Attributes {
		value, err := parseCatalogValue(attr.Value)
		if err != nil {
			return resourcecore.ResourceWriteRequest{}, err
		}
		attributes[i] = resourcecore.AttributeValue{Code: attr.Code, Value: value}
	}
	return resourcecore.ResourceWriteRequest{
		Actor: body.Actor,
		Scope: resourcecore.ResourceScope{
			ClassCode:  body.Scope.ClassCode,
			FamilyCode: body.Scope.FamilyCode,
			TypeCode:   body.Scope.TypeCode,
		},
		NaturalUnit: body.NaturalUnit,
		Attributes:  attributes,
	}, nil
}

func mapResourceResponse(res resourcecore.Resource) (resourceResponse, error) {
	attributes := make([]resourceAttributeResponse, len(res.Attributes))
	for i, attr := range res.Attributes {
		value, err := mapCatalogValue(attr.Value)
		if err != nil {
			return resourceResponse{}, fmt.Errorf("map attribute %q: %w", attr.Code, err)
		}
		attributes[i] = resourceAttributeResponse{Code: attr.Code, Value: value}
	}
	return resourceResponse{
		ID:         strconv.FormatInt(res.ID, 10),
		IdentityV1: res.IdentityV1,
		Scope: resourceScopeResponse{
			ClassCode:  res.Scope.ClassCode,
			FamilyCode: res.Scope.FamilyCode,
			TypeCode:   res.Scope.TypeCode,
		},
		NaturalUnit: res.NaturalUnit,
		Active:      res.Active,
		Revision:    strconv.FormatUint(res.Revision, 10),
		Attributes:  attributes,
	}, nil
}

// parseCatalogValue is the inverse of mapCatalogValue: it decodes one
// closed-union value from client JSON into a Core Value, rejecting unknown
// kinds and any shape carrying keys the matched kind does not expect.
func parseCatalogValue(raw json.RawMessage) (resourcecore.Value, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid value shape")
	}
	kind, err := popStringField(fields, "kind")
	if err != nil {
		return resourcecore.Value{}, err
	}

	switch resourcecore.ValueKind(kind) {
	case resourcecore.ValueText, resourcecore.ValueCode, resourcecore.ValueEnum, resourcecore.ValueControlledOption,
		resourcecore.ValueInteger, resourcecore.ValueDecimal:
		text, err := popStringField(fields, "value")
		if err != nil {
			return resourcecore.Value{}, err
		}
		if len(fields) != 0 {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "unexpected value fields")
		}
		return resourcecore.Value{Kind: resourcecore.ValueKind(kind), Text: text}, nil
	case resourcecore.ValueBool:
		raw, ok := fields["value"]
		delete(fields, "value")
		if !ok || len(fields) != 0 {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid BOOLEAN value shape")
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid boolean value")
		}
		return resourcecore.Value{Kind: resourcecore.ValueBool, Bool: b}, nil
	case resourcecore.ValueQuantity:
		text, err := popStringField(fields, "value")
		if err != nil {
			return resourcecore.Value{}, err
		}
		unit, err := popStringField(fields, "unitCode")
		if err != nil {
			return resourcecore.Value{}, err
		}
		if len(fields) != 0 {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "unexpected value fields")
		}
		return resourcecore.Value{Kind: resourcecore.ValueQuantity, Text: text, UnitCode: unit}, nil
	case resourcecore.ValueReference:
		raw, ok := fields["reference"]
		delete(fields, "reference")
		if !ok || len(fields) != 0 {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid REFERENCE value shape")
		}
		reference, err := parseReference(raw)
		if err != nil {
			return resourcecore.Value{}, err
		}
		return resourcecore.Value{Kind: resourcecore.ValueReference, Reference: &reference}, nil
	case resourcecore.ValueStringList:
		raw, ok := fields["values"]
		delete(fields, "values")
		if !ok || len(fields) != 0 {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid STRING_LIST value shape")
		}
		var values []string
		if err := json.Unmarshal(raw, &values); err != nil {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid string list value")
		}
		return resourcecore.Value{Kind: resourcecore.ValueStringList, Strings: values}, nil
	case resourcecore.ValueNotApplicable:
		if len(fields) != 0 {
			return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "unexpected value fields")
		}
		return resourcecore.Value{Kind: resourcecore.ValueNotApplicable}, nil
	default:
		return resourcecore.Value{}, resourcecore.NewError(resourcecore.InvalidArgument, "unknown value kind")
	}
}

func parseReference(raw json.RawMessage) (resourcecore.Reference, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return resourcecore.Reference{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid reference shape")
	}
	kind, err := popStringField(fields, "kind")
	if err != nil {
		return resourcecore.Reference{}, err
	}
	idText, err := popStringField(fields, "id")
	if err != nil {
		return resourcecore.Reference{}, err
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return resourcecore.Reference{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid id")
	}
	code, err := popStringField(fields, "code")
	if err != nil {
		return resourcecore.Reference{}, err
	}
	if len(fields) != 0 {
		return resourcecore.Reference{}, resourcecore.NewError(resourcecore.InvalidArgument, "unexpected reference fields")
	}
	return resourcecore.Reference{Kind: resourcecore.KindCode(kind), ID: id, Code: code}, nil
}

func popStringField(fields map[string]json.RawMessage, key string) (string, error) {
	raw, ok := fields[key]
	if !ok {
		return "", resourcecore.NewError(resourcecore.InvalidArgument, fmt.Sprintf("%s is required", key))
	}
	delete(fields, key)
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", resourcecore.NewError(resourcecore.InvalidArgument, fmt.Sprintf("invalid %s", key))
	}
	return value, nil
}

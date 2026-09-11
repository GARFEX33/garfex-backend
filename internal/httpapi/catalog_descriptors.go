package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

// CatalogDescriptorReader is the narrow Core capability required by the descriptors route.
type CatalogDescriptorReader interface {
	CatalogDescriptors(context.Context) ([]resourcecore.CatalogDescriptor, error)
}

// CatalogReader composes the read capabilities required by the catalog HTTP surface.
type CatalogReader interface {
	CatalogDescriptorReader
	ListCatalog(context.Context, resourcecore.CatalogQuery) (resourcecore.CatalogPage, error)
	GetCatalog(context.Context, resourcecore.CatalogKey) (resourcecore.CatalogRecord, error)
	ActiveClasses(context.Context) ([]resourcecore.CatalogRecord, error)
}

type catalogDescriptorsResponse struct {
	Descriptors []catalogDescriptorResponse `json:"descriptors"`
}

type activeClassesResponse struct {
	Records []catalogRecordResponse `json:"records"`
}

type catalogDescriptorResponse struct {
	Kind           string                    `json:"kind"`
	Singular       string                    `json:"singular"`
	Plural         string                    `json:"plural"`
	Fields         []fieldDescriptorResponse `json:"fields"`
	IdentityFields []string                  `json:"identityFields"`
	ParentKind     string                    `json:"parentKind"`
	ParentField    string                    `json:"parentField"`
}

type fieldDescriptorResponse struct {
	Name        string              `json:"name"`
	Label       string              `json:"label"`
	Kind        string              `json:"kind"`
	Required    bool                `json:"required"`
	RefKind     string              `json:"refKind"`
	RefScopedBy []string            `json:"refScopedBy"`
	EnumValues  []enumValueResponse `json:"enumValues"`
}

type enumValueResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func serveCatalogDescriptors(w http.ResponseWriter, r *http.Request, reader CatalogDescriptorReader) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog descriptor reader unavailable"))
		return
	}
	descriptors, err := reader.CatalogDescriptors(r.Context())
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response := catalogDescriptorsResponse{Descriptors: make([]catalogDescriptorResponse, len(descriptors))}
	for i, descriptor := range descriptors {
		response.Descriptors[i] = mapCatalogDescriptor(descriptor)
	}
	writeJSON(w, http.StatusOK, response)
}

func serveActiveClasses(w http.ResponseWriter, r *http.Request, reader CatalogReader) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog reader unavailable"))
		return
	}
	records, err := reader.ActiveClasses(r.Context())
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response := activeClassesResponse{Records: make([]catalogRecordResponse, len(records))}
	for i, record := range records {
		mapped, err := mapCatalogRecord(record)
		if err != nil {
			writeCatalogError(w, err)
			return
		}
		response.Records[i] = mapped
	}
	writeJSON(w, http.StatusOK, response)
}

func mapCatalogDescriptor(descriptor resourcecore.CatalogDescriptor) catalogDescriptorResponse {
	response := catalogDescriptorResponse{
		Kind:           string(descriptor.Kind),
		Singular:       descriptor.Singular,
		Plural:         descriptor.Plural,
		Fields:         make([]fieldDescriptorResponse, len(descriptor.Fields)),
		IdentityFields: stringsOrEmpty(descriptor.IdentityFields),
		ParentKind:     string(descriptor.ParentKind),
		ParentField:    descriptor.ParentField,
	}
	for i, field := range descriptor.Fields {
		response.Fields[i] = fieldDescriptorResponse{
			Name: field.Name, Label: field.Label, Kind: string(field.Kind), Required: field.Required,
			RefKind: string(field.RefKind), RefScopedBy: stringsOrEmpty(field.RefScopedBy),
			EnumValues: enumValuesOrEmpty(field.EnumValues),
		}
	}
	return response
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func enumValuesOrEmpty(values []resourcecore.EnumValue) []enumValueResponse {
	response := make([]enumValueResponse, len(values))
	for i, value := range values {
		response[i] = enumValueResponse{Value: value.Value, Label: value.Label}
	}
	return response
}

func writeCatalogError(w http.ResponseWriter, err error) {
	status, message := catalogError(err)
	log.Printf("catalog error: code=%s status=%d message=%q", resourcecore.Code(err), status, err.Error())
	writeJSON(w, status, errorResponse{Error: message})
}

func catalogError(err error) (int, string) {
	switch resourcecore.Code(err) {
	case resourcecore.InvalidArgument:
		return http.StatusBadRequest, "invalid request"
	case resourcecore.NotFound:
		return http.StatusNotFound, "not found"
	case resourcecore.Duplicate, resourcecore.Integrity, resourcecore.IdentityConflict,
		resourcecore.InvalidLifecycle, resourcecore.ReactivationImpossible, resourcecore.InUse,
		resourcecore.ImmutableCode, resourcecore.Conflict:
		return http.StatusConflict, "conflict"
	case resourcecore.InvalidReference, resourcecore.Validation, resourcecore.InvalidCatalog:
		return http.StatusUnprocessableEntity, "validation failed"
	case resourcecore.Unavailable:
		return http.StatusServiceUnavailable, "unavailable"
	case resourcecore.Internal:
		return http.StatusInternalServerError, "internal server error"
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

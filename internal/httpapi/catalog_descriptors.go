package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/GARFEX33/garfex-backend/resourcecore"
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
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Required    bool     `json:"required"`
	RefKind     string   `json:"refKind"`
	RefScopedBy []string `json:"refScopedBy"`
	// AllowCreate marks a reference field as reuse-before-create: a client
	// should search existing records of refKind first and offer creating a
	// new one only when nothing matches, instead of defaulting straight to a
	// create form. False (and meaningless) for non-reference fields.
	AllowCreate bool                `json:"allowCreate"`
	EnumValues  []enumValueResponse `json:"enumValues"`
}

type enumValueResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type errorResponse struct {
	Error string `json:"error"`
	// Code is the stable, machine-readable resourcecore.ErrorCode backing
	// Error (e.g. "IN_USE", "INVALID_LIFECYCLE", "CONFLICT", "NOT_FOUND").
	// Always safe to expose and to switch on: it is one of a small fixed set
	// of category names, never derived from request or infrastructure
	// content — unlike Error/Detail, which are human-readable and may collapse
	// several distinct Code values into the same text (e.g. every 409 reads
	// "conflict" regardless of Code). Empty when the failure never reached
	// Core (e.g. an HTTP method mismatch).
	Code string `json:"code,omitempty"`
	// Detail carries the Core's own validation message. It is populated only
	// for the closed set of validation-class codes (see isValidationCode),
	// whose messages are always developer-authored static strings that never
	// embed request data or infrastructure details — every other code keeps
	// returning a fully sanitized, generic message.
	Detail string `json:"detail,omitempty"`
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
			AllowCreate: field.AllowCreate,
			EnumValues:  enumValuesOrEmpty(field.EnumValues),
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
	code := resourcecore.Code(err)
	log.Printf("catalog error: code=%s status=%d message=%q", code, status, err.Error())
	resp := errorResponse{Error: message, Code: string(code)}
	if isValidationCode(code) {
		resp.Detail = err.Error()
	}
	writeJSON(w, status, resp)
}

// isValidationCode reports whether code's message is always a static,
// developer-authored string safe to return verbatim to the client — never a
// wrapped infrastructure/repository error that could carry secrets or paths.
func isValidationCode(code resourcecore.ErrorCode) bool {
	switch code {
	case resourcecore.InvalidArgument, resourcecore.Validation, resourcecore.InvalidReference, resourcecore.InvalidCatalog:
		return true
	default:
		return false
	}
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

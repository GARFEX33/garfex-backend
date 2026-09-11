package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

// CatalogWriter is the narrow Core capability required by the catalog
// create, update, and lifecycle routes.
type CatalogWriter interface {
	CreateCatalog(context.Context, resourcecore.CatalogWriteRequest) (resourcecore.CatalogRecord, error)
	UpdateCatalog(context.Context, resourcecore.CatalogUpdateRequest) (resourcecore.CatalogRecord, error)
	DeactivateCatalog(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error)
	ReactivateCatalog(context.Context, resourcecore.CatalogLifecycleRequest) (resourcecore.CatalogRecord, error)
}

type catalogCreateRequest struct {
	Actor  string                           `json:"actor"`
	Active bool                             `json:"active"`
	Values map[string]json.RawMessage       `json:"values"`
	Rules  []applicabilityRuleCreateRequest `json:"rules"`
}

type applicabilityRuleCreateRequest struct {
	AttributeCode        string          `json:"attributeCode"`
	Equals               json.RawMessage `json:"equals"`
	Mode                 string          `json:"mode"`
	IdentityParticipates bool            `json:"identityParticipates"`
	NotApplicable        bool            `json:"notApplicable"`
	Active               bool            `json:"active"`
}

func serveCreateCatalog(w http.ResponseWriter, r *http.Request, writer CatalogWriter, kind string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog writer unavailable"))
		return
	}
	var body catalogCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	req, err := mapCatalogCreateRequest(kind, body)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	record, err := writer.CreateCatalog(r.Context(), req)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapCatalogRecord(record)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

// mapCatalogCreateRequest preserves the nil/empty distinction on Rules:
// an omitted "rules" key stays nil, an explicit empty array stays a
// non-nil empty slice, matching CatalogWriteRequest's documented contract.
func mapCatalogCreateRequest(kind string, body catalogCreateRequest) (resourcecore.CatalogWriteRequest, error) {
	values := make(map[string]resourcecore.Value, len(body.Values))
	for name, raw := range body.Values {
		value, err := parseCatalogValue(raw)
		if err != nil {
			return resourcecore.CatalogWriteRequest{}, err
		}
		values[name] = value
	}
	var rules []resourcecore.ApplicabilityRule
	if body.Rules != nil {
		rules = make([]resourcecore.ApplicabilityRule, len(body.Rules))
		for i, rule := range body.Rules {
			equals, err := parseCatalogValue(rule.Equals)
			if err != nil {
				return resourcecore.CatalogWriteRequest{}, err
			}
			rules[i] = resourcecore.ApplicabilityRule{
				AttributeCode:        rule.AttributeCode,
				Equals:               equals,
				Mode:                 rule.Mode,
				IdentityParticipates: rule.IdentityParticipates,
				NotApplicable:        rule.NotApplicable,
				Active:               rule.Active,
			}
		}
	}
	return resourcecore.CatalogWriteRequest{
		Actor:  body.Actor,
		Kind:   resourcecore.KindCode(kind),
		Active: body.Active,
		Values: values,
		Rules:  rules,
	}, nil
}

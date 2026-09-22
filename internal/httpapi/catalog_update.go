package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

type catalogUpdateRequest struct {
	Actor            string                           `json:"actor"`
	ExpectedRevision string                           `json:"expectedRevision"`
	Active           bool                             `json:"active"`
	Values           map[string]json.RawMessage       `json:"values"`
	Rules            []applicabilityRuleCreateRequest `json:"rules"`
}

func serveCatalogUpdate(w http.ResponseWriter, r *http.Request, writer CatalogWriter, kind, id string) {
	key, ok := catalogKey(kind, id)
	if !ok {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid catalog id"))
		return
	}
	if writer == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog writer unavailable"))
		return
	}
	var body catalogUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid request body"))
		return
	}
	req, err := mapCatalogUpdateRequest(key, body)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	record, err := writer.UpdateCatalog(r.Context(), req)
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

// mapCatalogUpdateRequest preserves the nil/empty distinction on Rules, same
// as mapCatalogCreateRequest.
func mapCatalogUpdateRequest(key resourcecore.CatalogKey, body catalogUpdateRequest) (resourcecore.CatalogUpdateRequest, error) {
	expectedRevision, err := strconv.ParseUint(body.ExpectedRevision, 10, 64)
	if err != nil {
		return resourcecore.CatalogUpdateRequest{}, resourcecore.NewError(resourcecore.InvalidArgument, "invalid expected revision")
	}
	values := make(map[string]resourcecore.Value, len(body.Values))
	for name, raw := range body.Values {
		value, err := parseCatalogValue(raw)
		if err != nil {
			return resourcecore.CatalogUpdateRequest{}, err
		}
		values[name] = value
	}
	var rules []resourcecore.ApplicabilityRule
	if body.Rules != nil {
		rules = make([]resourcecore.ApplicabilityRule, len(body.Rules))
		for i, rule := range body.Rules {
			equals, err := parseCatalogValue(rule.Equals)
			if err != nil {
				return resourcecore.CatalogUpdateRequest{}, err
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
	return resourcecore.CatalogUpdateRequest{
		Actor:            body.Actor,
		Kind:             key.Kind,
		ID:               key.ID,
		ExpectedRevision: expectedRevision,
		Active:           body.Active,
		Values:           values,
		Rules:            rules,
	}, nil
}

package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

const (
	defaultResourceLimit = 50
	maximumResourceLimit = 50
)

// ResourceReader is the narrow Core capability required by the resource
// search and detail routes.
type ResourceReader interface {
	GetResource(context.Context, resourcecore.ResourceKey) (resourcecore.Resource, error)
	SearchResources(context.Context, resourcecore.ResourceQuery) (resourcecore.ResourcePage, error)
}

type resourcePageResponse struct {
	Resources   []resourceResponse `json:"resources"`
	HasPrevious bool               `json:"hasPrevious"`
	HasNext     bool               `json:"hasNext"`
}

func serveResources(w http.ResponseWriter, r *http.Request, writer ResourceWriter, reader ResourceReader) {
	switch r.Method {
	case http.MethodPost:
		serveCreateResource(w, r, writer)
	case http.MethodGet:
		serveResourceSearch(w, r, reader)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
	}
}

func serveResourceSearch(w http.ResponseWriter, r *http.Request, reader ResourceReader) {
	query, ok := resourceSearchQuery(r)
	if !ok {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid resource search query"))
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource reader unavailable"))
		return
	}
	page, err := reader.SearchResources(r.Context(), query)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapResourcePage(page)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func resourceSearchQuery(r *http.Request) (resourcecore.ResourceQuery, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return resourcecore.ResourceQuery{}, false
	}
	query := resourcecore.ResourceQuery{
		Scope:      resourcecore.ScopeActive,
		Text:       values.Get("text"),
		ClassCode:  values.Get("classCode"),
		FamilyCode: values.Get("familyCode"),
		TypeCode:   values.Get("typeCode"),
		Limit:      defaultResourceLimit,
	}
	if _, present := values["scope"]; present {
		query.Scope = resourcecore.LifecycleScope(values.Get("scope"))
		switch query.Scope {
		case resourcecore.ScopeActive, resourcecore.ScopeInactive, resourcecore.ScopeAll:
		default:
			return resourcecore.ResourceQuery{}, false
		}
	}
	if _, present := values["limit"]; present {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit <= 0 || limit > maximumResourceLimit {
			return resourcecore.ResourceQuery{}, false
		}
		query.Limit = limit
	}
	if _, present := values["offset"]; present {
		offset, err := strconv.Atoi(values.Get("offset"))
		if err != nil || offset < 0 {
			return resourcecore.ResourceQuery{}, false
		}
		query.Offset = offset
	}
	return query, true
}

func mapResourcePage(page resourcecore.ResourcePage) (resourcePageResponse, error) {
	resources := make([]resourceResponse, len(page.Resources))
	for i, res := range page.Resources {
		mapped, err := mapResourceResponse(res)
		if err != nil {
			return resourcePageResponse{}, err
		}
		resources[i] = mapped
	}
	return resourcePageResponse{Resources: resources, HasPrevious: page.HasPrevious, HasNext: page.HasNext}, nil
}

func serveResourceDetail(w http.ResponseWriter, r *http.Request, reader ResourceReader, classCode, identityV1 string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "resource reader unavailable"))
		return
	}
	resource, err := reader.GetResource(r.Context(), resourcecore.ResourceKey{ClassCode: classCode, IdentityV1: identityV1})
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

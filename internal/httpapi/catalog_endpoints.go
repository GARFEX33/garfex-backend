package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/GARFEX33/garfex-costos-unitarios/resourcecore"
)

const (
	defaultCatalogLimit = 50
	maximumCatalogLimit = 50
)

func serveCatalogDetail(w http.ResponseWriter, r *http.Request, reader CatalogReader, kind, id string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	key, ok := catalogKey(kind, id)
	if !ok {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid catalog id"))
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog reader unavailable"))
		return
	}
	record, err := reader.GetCatalog(r.Context(), key)
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

func catalogKey(kind, id string) (resourcecore.CatalogKey, bool) {
	if len(id) == 0 || id[0] < '1' || id[0] > '9' {
		return resourcecore.CatalogKey{}, false
	}
	for _, char := range id[1:] {
		if char < '0' || char > '9' {
			return resourcecore.CatalogKey{}, false
		}
	}
	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return resourcecore.CatalogKey{}, false
	}
	return resourcecore.CatalogKey{Kind: resourcecore.KindCode(kind), ID: parsed}, true
}

func serveCatalogList(w http.ResponseWriter, r *http.Request, reader CatalogReader, kind string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	query, ok := catalogListQuery(r, kind)
	if !ok {
		writeCatalogError(w, resourcecore.NewError(resourcecore.InvalidArgument, "invalid catalog list query"))
		return
	}
	if reader == nil {
		writeCatalogError(w, resourcecore.NewError(resourcecore.Internal, "catalog reader unavailable"))
		return
	}
	page, err := reader.ListCatalog(r.Context(), query)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	response, err := mapCatalogPage(page)
	if err != nil {
		writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func catalogListQuery(r *http.Request, kind string) (resourcecore.CatalogQuery, bool) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return resourcecore.CatalogQuery{}, false
	}
	query := resourcecore.CatalogQuery{
		Kind:   resourcecore.KindCode(kind),
		Scope:  resourcecore.ScopeActive,
		Text:   values.Get("text"),
		Limit:  defaultCatalogLimit,
		Offset: 0,
	}
	if _, present := values["scope"]; present {
		query.Scope = resourcecore.LifecycleScope(values.Get("scope"))
		switch query.Scope {
		case resourcecore.ScopeActive, resourcecore.ScopeInactive, resourcecore.ScopeAll:
		default:
			return resourcecore.CatalogQuery{}, false
		}
	}
	if _, present := values["limit"]; present {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit <= 0 || limit > maximumCatalogLimit {
			return resourcecore.CatalogQuery{}, false
		}
		query.Limit = limit
	}
	if _, present := values["offset"]; present {
		offset, err := strconv.Atoi(values.Get("offset"))
		if err != nil || offset < 0 {
			return resourcecore.CatalogQuery{}, false
		}
		query.Offset = offset
	}
	return query, true
}

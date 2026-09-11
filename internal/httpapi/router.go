// Package httpapi exposes Garfex's public HTTP operational endpoints.
package httpapi

import (
	"net/http"
	"strings"
)

// NewRouter returns the complete public HTTP surface for this unit.
func NewRouter(reader CatalogReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { route(w, r, reader) })
}

func route(w http.ResponseWriter, r *http.Request, reader CatalogReader) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	switch r.URL.Path {
	case "/healthz":
		serveGet(w, r, serveHealth)
	case "/v1/catalog/descriptors":
		serveCatalogDescriptors(w, r, reader)
	case "/openapi.yaml":
		serveGet(w, r, serveOpenAPI)
	case "/docs":
		serveGet(w, r, serveDocs)
	default:
		if kind, id, ok := catalogDetailPath(r.URL.Path); ok {
			serveCatalogDetail(w, r, reader, kind, id)
			return
		}
		if kind, ok := catalogListKind(r.URL.Path); ok {
			serveCatalogList(w, r, reader, kind)
			return
		}
		writeText(w, http.StatusNotFound, "404 not found\n")
	}
}

func catalogDetailPath(path string) (kind, id string, ok bool) {
	const prefix = "/v1/catalog/"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", "", false
	}
	kind, id, ok = strings.Cut(remainder, "/")
	return kind, id, ok && kind != "" && id != "" && !strings.Contains(id, "/")
}

func catalogListKind(path string) (string, bool) {
	const prefix = "/v1/catalog/"
	kind, ok := strings.CutPrefix(path, prefix)
	return kind, ok && kind != "" && !strings.Contains(kind, "/")
}

func serveGet(w http.ResponseWriter, r *http.Request, handler func(http.ResponseWriter)) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeText(w, http.StatusMethodNotAllowed, "405 method not allowed\n")
		return
	}
	handler(w)
}

func serveHealth(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
}

func serveOpenAPI(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write([]byte(openAPIDocument))
}

func serveDocs(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsPage))
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

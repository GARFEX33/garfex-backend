// Package httpapi exposes Garfex's public HTTP operational endpoints.
package httpapi

import (
	"net/http"
	"strings"
)

// NewRouter returns the complete public HTTP surface for this unit.
func NewRouter(reader CatalogReader, supplierWriter SupplierWriter, resourceWriter ResourceWriter, supplierReader SupplierReader, resourceReader ResourceReader, catalogWriter CatalogWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route(w, r, reader, supplierWriter, resourceWriter, supplierReader, resourceReader, catalogWriter)
	})
}

// maxRequestBodyBytes bounds every request body read through this router.
// 1 MiB is generous for the largest documented JSON payload (a catalog
// record with its values and rules) while still rejecting pathological
// inputs before they reach a decoder.
const maxRequestBodyBytes = 1 << 20

func route(w http.ResponseWriter, r *http.Request, reader CatalogReader, supplierWriter SupplierWriter, resourceWriter ResourceWriter, supplierReader SupplierReader, resourceReader ResourceReader, catalogWriter CatalogWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	switch r.URL.Path {
	case "/healthz":
		serveGet(w, r, serveHealth)
	case "/v1/catalog/descriptors":
		serveCatalogDescriptors(w, r, reader)
	case "/v1/catalog/classes":
		serveActiveClasses(w, r, reader)
	case "/v1/suppliers":
		serveSuppliers(w, r, supplierWriter, supplierReader)
	case "/v1/resources":
		serveResources(w, r, resourceWriter, resourceReader)
	case "/v1/cfdi/parse":
		serveCFDIParse(w, r)
	case "/v1/suppliers/from-cfdi/preview":
		serveSupplierCFDIPreview(w, r, supplierReader)
	case "/openapi.yaml":
		serveGet(w, r, serveOpenAPI)
	case "/docs":
		serveGet(w, r, serveDocs)
	default:
		if kind, id, action, ok := catalogLifecyclePath(r.URL.Path); ok {
			serveCatalogLifecycle(w, r, catalogWriter, kind, id, action)
			return
		}
		if kind, id, ok := catalogDetailPath(r.URL.Path); ok {
			serveCatalogDetail(w, r, reader, catalogWriter, kind, id)
			return
		}
		if kind, ok := catalogListKind(r.URL.Path); ok {
			serveCatalogByKind(w, r, reader, catalogWriter, kind)
			return
		}
		if supplierID, branchID, ok := supplierBranchDetailPath(r.URL.Path); ok {
			serveBranchDetail(w, r, supplierReader, supplierID, branchID)
			return
		}
		if supplierID, ok := supplierBranchListPath(r.URL.Path); ok {
			serveBranchList(w, r, supplierReader, supplierID)
			return
		}
		if supplierID, contactID, ok := supplierContactDetailPath(r.URL.Path); ok {
			serveContactDetail(w, r, supplierReader, supplierID, contactID)
			return
		}
		if supplierID, ok := supplierContactListPath(r.URL.Path); ok {
			serveContactList(w, r, supplierReader, supplierID)
			return
		}
		if id, ok := supplierDetailPath(r.URL.Path); ok {
			serveSupplierDetail(w, r, supplierReader, supplierWriter, id)
			return
		}
		if id, action, ok := resourceLifecyclePath(r.URL.Path); ok {
			serveResourceLifecycle(w, r, resourceWriter, id, action)
			return
		}
		if classCode, identityV1, ok := resourceDescribePath(r.URL.Path); ok {
			serveResourceDescribe(w, r, resourceReader, classCode, identityV1)
			return
		}
		if classCode, identityV1, ok := resourceDetailPath(r.URL.Path); ok {
			serveResourceDetail(w, r, resourceReader, classCode, identityV1)
			return
		}
		if id, ok := resourceIDPath(r.URL.Path); ok {
			serveResourceUpdate(w, r, resourceWriter, id)
			return
		}
		if typeCode, ok := typeAttributesEffectivePath(r.URL.Path); ok {
			serveEffectiveAttributes(w, r, resourceReader, typeCode)
			return
		}
		if typeCode, ok := typeAttributesEvaluatePath(r.URL.Path); ok {
			serveEvaluateAttributes(w, r, resourceReader, typeCode)
			return
		}
		writeText(w, http.StatusNotFound, "404 not found\n")
	}
}

// resourceIDPath matches the numeric-id write path, distinct from
// resourceDetailPath's natural-key read path: Core addresses resource
// writes by internal id, obtained as a byproduct of a prior read.
func resourceIDPath(path string) (id string, ok bool) {
	const prefix = "/v1/resources/"
	id, ok = strings.CutPrefix(path, prefix)
	return id, ok && id != "" && !strings.Contains(id, "/")
}

// resourceLifecyclePath matches the reserved deactivate/reactivate action
// paths. It is checked before resourceDetailPath, since both share the
// same two-segment shape; only these two literal action names are reserved.
func resourceLifecyclePath(path string) (id, action string, ok bool) {
	const prefix = "/v1/resources/"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", "", false
	}
	id, action, ok = strings.Cut(remainder, "/")
	if !ok || id == "" {
		return "", "", false
	}
	switch action {
	case "deactivate", "reactivate":
		return id, action, true
	default:
		return "", "", false
	}
}

func supplierDetailPath(path string) (id string, ok bool) {
	const prefix = "/v1/suppliers/"
	id, ok = strings.CutPrefix(path, prefix)
	return id, ok && id != "" && !strings.Contains(id, "/")
}

func supplierBranchListPath(path string) (supplierID string, ok bool) {
	return supplierNestedListPath(path, "/branches")
}

func supplierBranchDetailPath(path string) (supplierID, branchID string, ok bool) {
	return supplierNestedDetailPath(path, "/branches/")
}

func supplierContactListPath(path string) (supplierID string, ok bool) {
	return supplierNestedListPath(path, "/contacts")
}

func supplierContactDetailPath(path string) (supplierID, contactID string, ok bool) {
	return supplierNestedDetailPath(path, "/contacts/")
}

func supplierNestedListPath(path, suffix string) (supplierID string, ok bool) {
	const prefix = "/v1/suppliers/"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", false
	}
	supplierID, ok = strings.CutSuffix(remainder, suffix)
	return supplierID, ok && supplierID != "" && !strings.Contains(supplierID, "/")
}

func supplierNestedDetailPath(path, separator string) (supplierID, childID string, ok bool) {
	const prefix = "/v1/suppliers/"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", "", false
	}
	supplierID, childID, ok = strings.Cut(remainder, separator)
	return supplierID, childID, ok && supplierID != "" && childID != "" && !strings.Contains(childID, "/")
}

// resourceDescribePath matches the reserved /describe action path, a
// three-segment shape that resourceDetailPath's two-segment Cut already
// rejects, so no ordering is required between them.
func resourceDescribePath(path string) (classCode, identityV1 string, ok bool) {
	const prefix = "/v1/resources/"
	const suffix = "/describe"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", "", false
	}
	trimmed, ok := strings.CutSuffix(remainder, suffix)
	if !ok {
		return "", "", false
	}
	classCode, identityV1, ok = strings.Cut(trimmed, "/")
	return classCode, identityV1, ok && classCode != "" && identityV1 != "" && !strings.Contains(identityV1, "/")
}

func resourceDetailPath(path string) (classCode, identityV1 string, ok bool) {
	const prefix = "/v1/resources/"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", "", false
	}
	classCode, identityV1, ok = strings.Cut(remainder, "/")
	return classCode, identityV1, ok && classCode != "" && identityV1 != "" && !strings.Contains(identityV1, "/")
}

// catalogLifecyclePath matches the reserved deactivate/reactivate action
// paths, a three-segment shape (kind/id/action) that catalogDetailPath's
// two-segment Cut already rejects, so no ordering is required between them.
func catalogLifecyclePath(path string) (kind, id, action string, ok bool) {
	const prefix = "/v1/catalog/"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", "", "", false
	}
	kind, rest, ok := strings.Cut(remainder, "/")
	if !ok || kind == "" {
		return "", "", "", false
	}
	id, action, ok = strings.Cut(rest, "/")
	if !ok || id == "" {
		return "", "", "", false
	}
	switch action {
	case "deactivate", "reactivate":
		return kind, id, action, true
	default:
		return "", "", "", false
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

// typeAttributesEffectivePath matches GET
// /v1/types/{typeCode}/attributes/effective.
func typeAttributesEffectivePath(path string) (typeCode string, ok bool) {
	const prefix = "/v1/types/"
	const suffix = "/attributes/effective"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", false
	}
	typeCode, ok = strings.CutSuffix(remainder, suffix)
	return typeCode, ok && typeCode != "" && !strings.Contains(typeCode, "/")
}

// typeAttributesEvaluatePath matches POST
// /v1/types/{typeCode}/attributes/evaluate.
func typeAttributesEvaluatePath(path string) (typeCode string, ok bool) {
	const prefix = "/v1/types/"
	const suffix = "/attributes/evaluate"
	remainder, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", false
	}
	typeCode, ok = strings.CutSuffix(remainder, suffix)
	return typeCode, ok && typeCode != "" && !strings.Contains(typeCode, "/")
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

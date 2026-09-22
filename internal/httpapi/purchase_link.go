package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/purchasecore"
)

type confirmMappingRequest struct {
	Actor            *string `json:"actor"`
	Reason           *string `json:"reason"`
	ResourceID       *string `json:"resourceId"`
	ExpectedRevision *string `json:"expectedRevision"`
}

type correctMappingRequest struct {
	Actor                     *string `json:"actor"`
	Reason                    *string `json:"reason"`
	ExpectedCurrentResourceID *string `json:"expectedCurrentResourceId"`
	ResourceID                *string `json:"resourceId"`
	ExpectedRevision          *string `json:"expectedRevision"`
}

type retireMappingRequest struct {
	Actor                     *string `json:"actor"`
	Reason                    *string `json:"reason"`
	ExpectedCurrentResourceID *string `json:"expectedCurrentResourceId"`
	ExpectedRevision          *string `json:"expectedRevision"`
}

type reportMappingConflictRequest struct {
	Actor                     *string `json:"actor"`
	Reason                    *string `json:"reason"`
	ExpectedCurrentResourceID *string `json:"expectedCurrentResourceId"`
	ExpectedRevision          *string `json:"expectedRevision"`
}

type resolveMappingConflictRequest struct {
	Actor                     *string `json:"actor"`
	Reason                    *string `json:"reason"`
	ExpectedCurrentResourceID *string `json:"expectedCurrentResourceId"`
	ResourceID                *string `json:"resourceId"`
	ExpectedRevision          *string `json:"expectedRevision"`
}

type nullableString struct {
	Value   string
	Present bool
	Null    bool
}

func (n *nullableString) UnmarshalJSON(data []byte) error {
	n.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Value = ""
		n.Null = true
		return nil
	}
	n.Null = false
	return json.Unmarshal(data, &n.Value)
}

type resolvePurchaseLineRequest struct {
	Actor                      *string        `json:"actor"`
	Reason                     *string        `json:"reason"`
	ResourceID                 *string        `json:"resourceId"`
	ExpectedSupplierProductID  nullableString `json:"expectedSupplierProductId"`
	ExpectedMappingRevision    nullableString `json:"expectedMappingRevision"`
	ExpectedResolutionRevision *string        `json:"expectedResolutionRevision"`
	CommercialSupplierSKU      *string        `json:"commercialSupplierSku"`
}

type resolutionOverrideRequest struct {
	Actor            *string `json:"actor"`
	Reason           *string `json:"reason"`
	Override         *string `json:"override"`
	ExpectedRevision *string `json:"expectedRevision"`
}

type commercialIdentityResponse struct {
	SupplierProductID     string  `json:"supplierProductId"`
	SupplierID            string  `json:"supplierId"`
	CommercialSupplierSKU string  `json:"commercialSupplierSku"`
	Disposition           string  `json:"disposition"`
	MappingRevision       string  `json:"mappingRevision"`
	ResourceID            *string `json:"resourceId"`
}

type resolvePurchaseLineResponse struct {
	Line               purchaseLineResponse       `json:"line"`
	SupplierProduct    supplierProductResponse    `json:"supplierProduct"`
	CommercialIdentity commercialIdentityResponse `json:"commercialIdentity"`
}

func decodePurchaseRequest(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("request contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeInvalidPurchaseRequest(w http.ResponseWriter, detail string) {
	writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, detail))
}

func mappingDecision(actor, reason *string) (purchasecore.MappingDecisionMetadata, bool) {
	if actor == nil || reason == nil {
		return purchasecore.MappingDecisionMetadata{}, false
	}
	return purchasecore.MappingDecisionMetadata{
		Actor:  *actor,
		Origin: purchasecore.MappingOriginManual,
		Reason: *reason,
		At:     time.Now().UTC(),
	}, true
}

func serveSupplierProductMapping(w http.ResponseWriter, r *http.Request, writer PurchaseWriter, id, action string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	productID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid supplier product id"))
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}

	var product purchasecore.SupplierProduct
	var err error
	switch action {
	case "confirm":
		var body confirmMappingRequest
		if err := decodePurchaseRequest(r, &body); err != nil {
			writeInvalidPurchaseRequest(w, "invalid request body")
			return
		}
		decision, valid := mappingDecision(body.Actor, body.Reason)
		if !valid || body.ResourceID == nil || body.ExpectedRevision == nil {
			writeInvalidPurchaseRequest(w, "missing required request field")
			return
		}
		resourceID, valid := positiveInt64(*body.ResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid resource id")
			return
		}
		expectedRevision, valid := parseMappingRevision(*body.ExpectedRevision)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected revision")
			return
		}
		product, err = writer.ConfirmMapping(r.Context(), purchasecore.ConfirmMappingRequest{
			SupplierProductID: productID,
			ResourceID:        resourceID,
			ExpectedRevision:  expectedRevision,
			Decision:          decision,
		})
	case "correct":
		var body correctMappingRequest
		if err := decodePurchaseRequest(r, &body); err != nil {
			writeInvalidPurchaseRequest(w, "invalid request body")
			return
		}
		decision, valid := mappingDecision(body.Actor, body.Reason)
		if !valid || body.ExpectedCurrentResourceID == nil || body.ResourceID == nil || body.ExpectedRevision == nil {
			writeInvalidPurchaseRequest(w, "missing required request field")
			return
		}
		expectedResourceID, valid := positiveInt64(*body.ExpectedCurrentResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected current resource id")
			return
		}
		resourceID, valid := positiveInt64(*body.ResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid resource id")
			return
		}
		expectedRevision, valid := parseMappingRevision(*body.ExpectedRevision)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected revision")
			return
		}
		product, err = writer.CorrectMapping(r.Context(), purchasecore.CorrectMappingRequest{
			SupplierProductID:         productID,
			ExpectedCurrentResourceID: expectedResourceID,
			ResourceID:                resourceID,
			ExpectedRevision:          expectedRevision,
			Decision:                  decision,
		})
	case "retire":
		var body retireMappingRequest
		if err := decodePurchaseRequest(r, &body); err != nil {
			writeInvalidPurchaseRequest(w, "invalid request body")
			return
		}
		decision, valid := mappingDecision(body.Actor, body.Reason)
		if !valid || body.ExpectedCurrentResourceID == nil || body.ExpectedRevision == nil {
			writeInvalidPurchaseRequest(w, "missing required request field")
			return
		}
		expectedResourceID, valid := positiveInt64(*body.ExpectedCurrentResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected current resource id")
			return
		}
		expectedRevision, valid := parseMappingRevision(*body.ExpectedRevision)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected revision")
			return
		}
		product, err = writer.ExceptionalUnlink(r.Context(), purchasecore.ExceptionalUnlinkRequest{
			SupplierProductID:         productID,
			ExpectedCurrentResourceID: expectedResourceID,
			ExpectedRevision:          expectedRevision,
			Decision:                  decision,
		})
	case "report-conflict":
		var body reportMappingConflictRequest
		if err := decodePurchaseRequest(r, &body); err != nil {
			writeInvalidPurchaseRequest(w, "invalid request body")
			return
		}
		decision, valid := mappingDecision(body.Actor, body.Reason)
		if !valid || body.ExpectedCurrentResourceID == nil || body.ExpectedRevision == nil {
			writeInvalidPurchaseRequest(w, "missing required request field")
			return
		}
		expectedResourceID, valid := positiveInt64(*body.ExpectedCurrentResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected current resource id")
			return
		}
		expectedRevision, valid := parseMappingRevision(*body.ExpectedRevision)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected revision")
			return
		}
		product, err = writer.ReportIdentityConflict(r.Context(), purchasecore.ReportIdentityConflictRequest{
			SupplierProductID:         productID,
			ExpectedCurrentResourceID: expectedResourceID,
			ExpectedRevision:          expectedRevision,
			Decision:                  decision,
		})
	case "resolve-conflict":
		var body resolveMappingConflictRequest
		if err := decodePurchaseRequest(r, &body); err != nil {
			writeInvalidPurchaseRequest(w, "invalid request body")
			return
		}
		decision, valid := mappingDecision(body.Actor, body.Reason)
		if !valid || body.ExpectedCurrentResourceID == nil || body.ResourceID == nil || body.ExpectedRevision == nil {
			writeInvalidPurchaseRequest(w, "missing required request field")
			return
		}
		expectedResourceID, valid := positiveInt64(*body.ExpectedCurrentResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected current resource id")
			return
		}
		resourceID, valid := positiveInt64(*body.ResourceID)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid resource id")
			return
		}
		expectedRevision, valid := parseMappingRevision(*body.ExpectedRevision)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected revision")
			return
		}
		product, err = writer.ResolveIdentityConflict(r.Context(), purchasecore.ResolveIdentityConflictRequest{
			SupplierProductID:         productID,
			ExpectedCurrentResourceID: expectedResourceID,
			ResourceID:                resourceID,
			ExpectedRevision:          expectedRevision,
			Decision:                  decision,
		})
	default:
		writeText(w, http.StatusNotFound, "404 not found\n")
		return
	}
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSupplierProduct(product))
}

func servePurchaseLineResolve(w http.ResponseWriter, r *http.Request, writer PurchaseWriter, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	lineID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid purchase line id"))
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}
	var body resolvePurchaseLineRequest
	if err := decodePurchaseRequest(r, &body); err != nil {
		writeInvalidPurchaseRequest(w, "invalid request body")
		return
	}
	if body.Actor == nil || body.Reason == nil || body.ResourceID == nil ||
		body.ExpectedResolutionRevision == nil || body.CommercialSupplierSKU == nil ||
		!body.ExpectedSupplierProductID.Present || !body.ExpectedMappingRevision.Present {
		writeInvalidPurchaseRequest(w, "missing required request field")
		return
	}
	resourceID, ok := positiveInt64(*body.ResourceID)
	if !ok {
		writeInvalidPurchaseRequest(w, "invalid resource id")
		return
	}
	expectedResolutionRevision, ok := parseResolutionRevision(*body.ExpectedResolutionRevision)
	if !ok {
		writeInvalidPurchaseRequest(w, "invalid expected resolution revision")
		return
	}
	var expectedSupplierProductID *int64
	var expectedMappingRevision *purchasecore.MappingRevision
	if body.ExpectedSupplierProductID.Null != body.ExpectedMappingRevision.Null {
		writeInvalidPurchaseRequest(w, "expected snapshot fields must both be null or non-null")
		return
	}
	if !body.ExpectedSupplierProductID.Null {
		value, valid := positiveInt64(body.ExpectedSupplierProductID.Value)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected supplier product id")
			return
		}
		expectedSupplierProductID = &value
		valueRevision, valid := parseMappingRevision(body.ExpectedMappingRevision.Value)
		if !valid {
			writeInvalidPurchaseRequest(w, "invalid expected mapping revision")
			return
		}
		expectedMappingRevision = &valueRevision
	}
	result, err := writer.ResolvePurchaseLine(r.Context(), purchasecore.ResolvePurchaseLineRequest{
		LineID:                     lineID,
		ResourceID:                 resourceID,
		CommercialSupplierSKU:      *body.CommercialSupplierSKU,
		ExpectedSupplierProductID:  expectedSupplierProductID,
		ExpectedMappingRevision:    expectedMappingRevision,
		ExpectedResolutionRevision: expectedResolutionRevision,
		Actor:                      *body.Actor,
		Reason:                     *body.Reason,
	})
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resolvePurchaseLineResponse{
		Line:            mapPurchaseLine(result.Line),
		SupplierProduct: mapSupplierProduct(result.SupplierProduct),
		CommercialIdentity: commercialIdentityResponse{
			SupplierProductID:     strconv.FormatInt(result.SupplierProduct.ID, 10),
			SupplierID:            strconv.FormatInt(result.SupplierProduct.SupplierID, 10),
			CommercialSupplierSKU: result.SupplierProduct.SupplierSKU,
			Disposition:           string(result.CommercialIdentityDisposition),
			MappingRevision:       strconv.FormatUint(uint64(result.SupplierProduct.MappingRevision), 10),
			ResourceID:            formatOptionalID(result.SupplierProduct.CurrentMapping.ResourceID),
		},
	})
}

func servePurchaseLineResolutionOverride(w http.ResponseWriter, r *http.Request, writer PurchaseWriter, id string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	lineID, ok := positiveInt64(id)
	if !ok {
		writePurchaseError(w, purchasecore.NewError(purchasecore.InvalidArgument, "invalid purchase line id"))
		return
	}
	if writer == nil {
		writePurchaseError(w, purchasecore.NewError(purchasecore.Internal, "purchase writer unavailable"))
		return
	}
	var body resolutionOverrideRequest
	if err := decodePurchaseRequest(r, &body); err != nil {
		writeInvalidPurchaseRequest(w, "invalid request body")
		return
	}
	if body.Actor == nil || body.Reason == nil || body.Override == nil || body.ExpectedRevision == nil {
		writeInvalidPurchaseRequest(w, "missing required request field")
		return
	}
	expectedRevision, ok := parseResolutionRevision(*body.ExpectedRevision)
	if !ok {
		writeInvalidPurchaseRequest(w, "invalid expected revision")
		return
	}
	line, err := writer.SetResolutionOverride(r.Context(), purchasecore.SetResolutionOverrideRequest{
		LineID:           lineID,
		Override:         purchasecore.LinkStatus(*body.Override),
		ExpectedRevision: expectedRevision,
		Actor:            *body.Actor,
		Reason:           *body.Reason,
	})
	if err != nil {
		writePurchaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapPurchaseLine(line))
}

func parseMappingRevision(value string) (purchasecore.MappingRevision, bool) {
	parsed, ok := parseUnsignedDecimal(value)
	if !ok {
		return 0, false
	}
	return purchasecore.MappingRevision(parsed), true
}

func parseResolutionRevision(value string) (purchasecore.ResolutionRevision, bool) {
	parsed, ok := parseUnsignedDecimal(value)
	if !ok {
		return 0, false
	}
	return purchasecore.ResolutionRevision(parsed), true
}

func parseUnsignedDecimal(value string) (uint64, bool) {
	if len(value) == 0 || (len(value) > 1 && value[0] == '0') {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

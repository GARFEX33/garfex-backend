package purchasecore

import "time"

func copyInt64(id *int64) *int64 {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}
func copyString(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}
func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}
func copyBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func CloneXMLDocument(doc XMLDocument) XMLDocument { doc.Content = copyBytes(doc.Content); return doc }
func ClonePurchase(p Purchase) Purchase {
	p.BranchID = copyInt64(p.BranchID)
	p.ExchangeRate = copyString(p.ExchangeRate)
	p.XML = CloneXMLDocument(p.XML)
	return p
}
func ClonePurchaseLine(l PurchaseLine) PurchaseLine {
	l.SupplierProductID = copyInt64(l.SupplierProductID)
	return l
}

func ClonePurchaseLineQuery(q PurchaseLineQuery) PurchaseLineQuery {
	q.SupplierID = copyInt64(q.SupplierID)
	q.DateFrom = copyTime(q.DateFrom)
	q.DateTo = copyTime(q.DateTo)
	return q
}

func ClonePurchaseLineRow(row PurchaseLineRow) PurchaseLineRow {
	row.CommercialSupplierSKU = copyString(row.CommercialSupplierSKU)
	row.SupplierProductID = copyInt64(row.SupplierProductID)
	row.ResourceID = copyInt64(row.ResourceID)
	row.ResourceIdentity = copyString(row.ResourceIdentity)
	row.ResourceDisplayName = copyString(row.ResourceDisplayName)
	if row.MappingRevision != nil {
		revision := *row.MappingRevision
		row.MappingRevision = &revision
	}
	return row
}
func CloneSupplierProduct(sp SupplierProduct) SupplierProduct {
	sp.CurrentMapping.ResourceID = copyInt64(sp.CurrentMapping.ResourceID)
	sp.ResourceActive = copyBool(sp.ResourceActive)
	return sp
}
func CloneMappingAuditEntry(entry MappingAuditEntry) MappingAuditEntry {
	entry.PreviousMapping.ResourceID = copyInt64(entry.PreviousMapping.ResourceID)
	entry.NewMapping.ResourceID = copyInt64(entry.NewMapping.ResourceID)
	return entry
}
func ClonePurchaseLineHistory(h PurchaseLineHistory) PurchaseLineHistory {
	h.Line = ClonePurchaseLine(h.Line)
	h.SupplierProduct = CloneSupplierProduct(h.SupplierProduct)
	h.BranchID = copyInt64(h.BranchID)
	return h
}

func clonePurchaseSlice(values []Purchase) []Purchase {
	if values == nil {
		return nil
	}
	out := make([]Purchase, len(values))
	for i := range values {
		out[i] = ClonePurchase(values[i])
	}
	return out
}
func cloneSupplierProductSlice(values []SupplierProduct) []SupplierProduct {
	if values == nil {
		return nil
	}
	out := make([]SupplierProduct, len(values))
	for i := range values {
		out[i] = CloneSupplierProduct(values[i])
	}
	return out
}
func clonePurchaseLineSlice(values []PurchaseLine) []PurchaseLine {
	if values == nil {
		return nil
	}
	out := make([]PurchaseLine, len(values))
	for i := range values {
		out[i] = ClonePurchaseLine(values[i])
	}
	return out
}
func clonePurchaseLineRowSlice(values []PurchaseLineRow) []PurchaseLineRow {
	if values == nil {
		return nil
	}
	out := make([]PurchaseLineRow, len(values))
	for i := range values {
		out[i] = ClonePurchaseLineRow(values[i])
	}
	return out
}
func clonePurchaseLineHistorySlice(values []PurchaseLineHistory) []PurchaseLineHistory {
	if values == nil {
		return nil
	}
	out := make([]PurchaseLineHistory, len(values))
	for i := range values {
		out[i] = ClonePurchaseLineHistory(values[i])
	}
	return out
}
func cloneMappingAuditSlice(values []MappingAuditEntry) []MappingAuditEntry {
	if values == nil {
		return nil
	}
	out := make([]MappingAuditEntry, len(values))
	for i := range values {
		out[i] = CloneMappingAuditEntry(values[i])
	}
	return out
}

package purchasecore

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

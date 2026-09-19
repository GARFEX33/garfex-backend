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

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// CloneXMLDocument returns a copy of doc with an independent Content slice.
func CloneXMLDocument(doc XMLDocument) XMLDocument {
	doc.Content = copyBytes(doc.Content)
	return doc
}

// ClonePurchase returns a copy of p with independent ExchangeRate, BranchID,
// and XML content.
func ClonePurchase(p Purchase) Purchase {
	p.BranchID = copyInt64(p.BranchID)
	p.ExchangeRate = copyString(p.ExchangeRate)
	p.XML = CloneXMLDocument(p.XML)
	return p
}

// ClonePurchaseLine returns a copy of l with an independent
// SupplierProductID pointer.
func ClonePurchaseLine(l PurchaseLine) PurchaseLine {
	l.SupplierProductID = copyInt64(l.SupplierProductID)
	return l
}

// CloneSupplierProduct returns a copy of sp with an independent ResourceID
// pointer.
func CloneSupplierProduct(sp SupplierProduct) SupplierProduct {
	sp.ResourceID = copyInt64(sp.ResourceID)
	return sp
}

// ClonePurchaseLineHistory returns a copy of h with every nested value
// independently copied.
func ClonePurchaseLineHistory(h PurchaseLineHistory) PurchaseLineHistory {
	h.Line = ClonePurchaseLine(h.Line)
	h.SupplierProduct = CloneSupplierProduct(h.SupplierProduct)
	h.BranchID = copyInt64(h.BranchID)
	return h
}

func clonePurchaseSlice(purchases []Purchase) []Purchase {
	if purchases == nil {
		return nil
	}
	out := make([]Purchase, len(purchases))
	for i := range purchases {
		out[i] = ClonePurchase(purchases[i])
	}
	return out
}

func cloneSupplierProductSlice(products []SupplierProduct) []SupplierProduct {
	if products == nil {
		return nil
	}
	out := make([]SupplierProduct, len(products))
	for i := range products {
		out[i] = CloneSupplierProduct(products[i])
	}
	return out
}

func clonePurchaseLineSlice(lines []PurchaseLine) []PurchaseLine {
	if lines == nil {
		return nil
	}
	out := make([]PurchaseLine, len(lines))
	for i := range lines {
		out[i] = ClonePurchaseLine(lines[i])
	}
	return out
}

func clonePurchaseLineHistorySlice(history []PurchaseLineHistory) []PurchaseLineHistory {
	if history == nil {
		return nil
	}
	out := make([]PurchaseLineHistory, len(history))
	for i := range history {
		out[i] = ClonePurchaseLineHistory(history[i])
	}
	return out
}

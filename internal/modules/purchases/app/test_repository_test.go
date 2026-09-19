package app

import (
	"context"
	"sort"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
)

// memoryRepository is a minimal in-process stand-in for domain.Repository,
// reproducing only the invariants app package tests exercise: fiscal UUID
// uniqueness and get-or-create supplier product identity. It is not a
// substitute for the PostgreSQL repository's concurrency and atomicity
// guarantees, which are covered by the postgres package's own tests.
type memoryRepository struct {
	nextID           int64
	purchasesByID    map[int64]domain.Purchase
	purchasesByUUID  map[string]int64
	lines            map[int64][]domain.PurchaseLine
	supplierProducts map[int64]domain.SupplierProduct
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		nextID:           1,
		purchasesByID:    map[int64]domain.Purchase{},
		purchasesByUUID:  map[string]int64{},
		lines:            map[int64][]domain.PurchaseLine{},
		supplierProducts: map[int64]domain.SupplierProduct{},
	}
}

func (r *memoryRepository) id() int64 { id := r.nextID; r.nextID++; return id }

func (r *memoryRepository) Import(_ context.Context, draft domain.PurchaseDraft) (domain.ImportResult, error) {
	if existingID, ok := r.purchasesByUUID[draft.CFDIUUID]; ok {
		existing := r.purchasesByID[existingID]
		existingLines := r.lines[existingID]
		if existing.SameRelevantContent(draft) && domain.SameLines(existingLines, draft.Lines) {
			return domain.ImportResult{Purchase: existing, Lines: existingLines, AlreadyExisted: true}, nil
		}
		return domain.ImportResult{}, domain.ErrPurchaseConflict
	}

	purchase := domain.Purchase{
		ID: r.id(), Supplier: draft.Supplier, CFDIUUID: draft.CFDIUUID, Series: draft.Series, Folio: draft.Folio,
		IssuedAt: draft.IssuedAt, Currency: draft.Currency, ExchangeRate: draft.ExchangeRate,
		Subtotal: draft.Subtotal, Discount: draft.Discount, TaxTransferred: draft.TaxTransferred, TaxWithheld: draft.TaxWithheld,
		Total: draft.Total, IssuerTaxID: draft.IssuerTaxID, IssuerName: draft.IssuerName, XML: draft.XML,
	}
	r.purchasesByID[purchase.ID] = purchase
	r.purchasesByUUID[purchase.CFDIUUID] = purchase.ID

	lines := make([]domain.PurchaseLine, 0, len(draft.Lines))
	for _, lineDraft := range draft.Lines {
		var supplierProductID *int64
		status := domain.LinkPending
		if lineDraft.HasSupplierIdentity() {
			id := r.getOrCreateSupplierProduct(draft.Supplier.SupplierID, lineDraft.SupplierSKU, lineDraft.Description)
			supplierProductID = &id
			if r.supplierProducts[id].ResourceID != nil {
				status = domain.LinkLinked
			}
		}
		lines = append(lines, domain.PurchaseLine{
			ID: r.id(), PurchaseID: purchase.ID, LineNumber: lineDraft.LineNumber, Description: lineDraft.Description,
			SupplierSKU: lineDraft.SupplierSKU, SATProductCode: lineDraft.SATProductCode, Quantity: lineDraft.Quantity,
			UnitCode: lineDraft.UnitCode, Unit: lineDraft.Unit, UnitPrice: lineDraft.UnitPrice, Amount: lineDraft.Amount,
			Discount: lineDraft.Discount, TaxTransferred: lineDraft.TaxTransferred, TaxWithheld: lineDraft.TaxWithheld,
			TaxObject: lineDraft.TaxObject, SupplierProductID: supplierProductID, LinkStatus: status,
		})
	}
	r.lines[purchase.ID] = lines
	return domain.ImportResult{Purchase: purchase, Lines: lines, AlreadyExisted: false}, nil
}

func (r *memoryRepository) getOrCreateSupplierProduct(supplierID int64, sku, description string) int64 {
	for _, product := range r.supplierProducts {
		if product.SupplierID == supplierID && product.SupplierSKU == sku {
			return product.ID
		}
	}
	id := r.id()
	r.supplierProducts[id] = domain.SupplierProduct{ID: id, SupplierID: supplierID, SupplierSKU: sku, Description: description}
	return id
}

func (r *memoryRepository) GetPurchase(_ context.Context, id int64) (domain.Purchase, error) {
	value, ok := r.purchasesByID[id]
	if !ok {
		return domain.Purchase{}, domain.ErrPurchaseNotFound
	}
	return value, nil
}

func (r *memoryRepository) GetPurchaseByUUID(_ context.Context, uuid string) (domain.Purchase, error) {
	id, ok := r.purchasesByUUID[strings.ToUpper(strings.TrimSpace(uuid))]
	if !ok {
		return domain.Purchase{}, domain.ErrPurchaseNotFound
	}
	return r.purchasesByID[id], nil
}

func (r *memoryRepository) ListPurchaseLines(_ context.Context, purchaseID int64) ([]domain.PurchaseLine, error) {
	return r.lines[purchaseID], nil
}

func (r *memoryRepository) ListPurchasesBySupplier(_ context.Context, supplierID int64, _ domain.ListCriteria) ([]domain.Purchase, error) {
	values := make([]domain.Purchase, 0)
	for _, purchase := range r.purchasesByID {
		if purchase.Supplier.SupplierID == supplierID {
			values = append(values, purchase)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

func (r *memoryRepository) GetSupplierProduct(_ context.Context, id int64) (domain.SupplierProduct, error) {
	value, ok := r.supplierProducts[id]
	if !ok {
		return domain.SupplierProduct{}, domain.ErrSupplierProductNotFound
	}
	return value, nil
}

func (r *memoryRepository) FindSupplierProduct(_ context.Context, supplierID int64, sku string) (domain.SupplierProduct, error) {
	for _, product := range r.supplierProducts {
		if product.SupplierID == supplierID && product.SupplierSKU == sku {
			return product, nil
		}
	}
	return domain.SupplierProduct{}, domain.ErrSupplierProductNotFound
}

func (r *memoryRepository) ListSupplierProducts(_ context.Context, supplierID int64, _ domain.ListCriteria) ([]domain.SupplierProduct, error) {
	values := make([]domain.SupplierProduct, 0)
	for _, product := range r.supplierProducts {
		if product.SupplierID == supplierID {
			values = append(values, product)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

func (r *memoryRepository) LinkSupplierProductToResource(_ context.Context, supplierProductID, resourceID int64) (domain.SupplierProduct, error) {
	product, ok := r.supplierProducts[supplierProductID]
	if !ok {
		return domain.SupplierProduct{}, domain.ErrSupplierProductNotFound
	}
	product, err := product.WithResource(resourceID)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.supplierProducts[supplierProductID] = product
	for purchaseID, lines := range r.lines {
		for i, line := range lines {
			if line.SupplierProductID != nil && *line.SupplierProductID == supplierProductID && line.LinkStatus == domain.LinkPending {
				lines[i].LinkStatus = domain.LinkLinked
			}
		}
		r.lines[purchaseID] = lines
	}
	return product, nil
}

func (r *memoryRepository) UnlinkSupplierProduct(_ context.Context, supplierProductID int64) (domain.SupplierProduct, error) {
	product, ok := r.supplierProducts[supplierProductID]
	if !ok {
		return domain.SupplierProduct{}, domain.ErrSupplierProductNotFound
	}
	product = product.WithoutResource()
	r.supplierProducts[supplierProductID] = product
	for purchaseID, lines := range r.lines {
		for i, line := range lines {
			if line.SupplierProductID != nil && *line.SupplierProductID == supplierProductID && line.LinkStatus == domain.LinkLinked {
				lines[i].LinkStatus = domain.LinkPending
			}
		}
		r.lines[purchaseID] = lines
	}
	return product, nil
}

func (r *memoryRepository) SetPurchaseLineLinkStatus(_ context.Context, lineID int64, status domain.LinkStatus) (domain.PurchaseLine, error) {
	for purchaseID, lines := range r.lines {
		for i, line := range lines {
			if line.ID == lineID {
				lines[i].LinkStatus = status
				r.lines[purchaseID] = lines
				return lines[i], nil
			}
		}
	}
	return domain.PurchaseLine{}, domain.ErrPurchaseLineNotFound
}

func (r *memoryRepository) ListPurchaseLinesByResource(_ context.Context, resourceID int64, _ domain.ListCriteria) ([]domain.PurchaseLineHistory, error) {
	history := make([]domain.PurchaseLineHistory, 0)
	for _, purchase := range r.purchasesByID {
		for _, line := range r.lines[purchase.ID] {
			if line.SupplierProductID == nil {
				continue
			}
			product := r.supplierProducts[*line.SupplierProductID]
			if product.ResourceID == nil || *product.ResourceID != resourceID {
				continue
			}
			history = append(history, domain.PurchaseLineHistory{
				Line: line, SupplierProduct: product, PurchaseID: purchase.ID, PurchaseUUID: purchase.CFDIUUID,
				SupplierID: purchase.Supplier.SupplierID, BranchID: purchase.Supplier.BranchID,
				IssuedAt: purchase.IssuedAt, Currency: purchase.Currency,
			})
		}
	}
	return history, nil
}

// fakeSupplierDirectory is a minimal in-process stand-in for
// SupplierDirectory.
type fakeSupplierDirectory struct {
	nextID              int64
	suppliers           map[int64]supplierdomain.Supplier
	branches            map[int64]supplierdomain.Branch
	createSupplierCalls int
	createErr           error
}

func newFakeSupplierDirectory() *fakeSupplierDirectory {
	return &fakeSupplierDirectory{nextID: 1, suppliers: map[int64]supplierdomain.Supplier{}, branches: map[int64]supplierdomain.Branch{}}
}

func (d *fakeSupplierDirectory) id() int64 { id := d.nextID; d.nextID++; return id }

func (d *fakeSupplierDirectory) GetSupplierByTaxIdentifier(_ context.Context, taxID string) (supplierdomain.Supplier, error) {
	for _, supplier := range d.suppliers {
		if strings.EqualFold(supplier.TaxIdentifier, taxID) {
			return supplier, nil
		}
	}
	return supplierdomain.Supplier{}, supplierdomain.ErrSupplierNotFound
}

func (d *fakeSupplierDirectory) CreateSupplier(_ context.Context, details supplierdomain.SupplierDetails) (supplierdomain.Supplier, error) {
	d.createSupplierCalls++
	if d.createErr != nil {
		return supplierdomain.Supplier{}, d.createErr
	}
	supplier := supplierdomain.Supplier{ID: d.id(), LegalName: details.LegalName, TaxIdentifier: details.TaxIdentifier, Active: true}
	d.suppliers[supplier.ID] = supplier
	return supplier, nil
}

func (d *fakeSupplierDirectory) GetBranch(_ context.Context, supplierID, branchID int64) (supplierdomain.Branch, error) {
	branch, ok := d.branches[branchID]
	if !ok || branch.SupplierID != supplierID {
		return supplierdomain.Branch{}, supplierdomain.ErrBranchNotFound
	}
	return branch, nil
}

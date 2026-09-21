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
	resources        map[int64]bool
	audit            map[int64][]domain.MappingAuditEntry
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		nextID:           1,
		purchasesByID:    map[int64]domain.Purchase{},
		purchasesByUUID:  map[string]int64{},
		lines:            map[int64][]domain.PurchaseLine{},
		supplierProducts: map[int64]domain.SupplierProduct{},
		resources:        map[int64]bool{},
		audit:            map[int64][]domain.MappingAuditEntry{},
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
		if lineDraft.HasSupplierIdentity() {
			id := r.getOrCreateSupplierProduct(draft.Supplier.SupplierID, lineDraft.SupplierSKU, lineDraft.Description)
			supplierProductID = &id
		}
		line := domain.PurchaseLine{
			ID: r.id(), PurchaseID: purchase.ID, LineNumber: lineDraft.LineNumber, Description: lineDraft.Description,
			SupplierSKU: lineDraft.SupplierSKU, SATProductCode: lineDraft.SATProductCode, Quantity: lineDraft.Quantity,
			UnitCode: lineDraft.UnitCode, Unit: lineDraft.Unit, UnitPrice: lineDraft.UnitPrice, Amount: lineDraft.Amount,
			Discount: lineDraft.Discount, TaxTransferred: lineDraft.TaxTransferred, TaxWithheld: lineDraft.TaxWithheld,
			TaxObject: lineDraft.TaxObject, SupplierProductID: supplierProductID, ResolutionOverride: domain.LinkStatusNone,
		}
		if supplierProductID != nil {
			product := r.supplierProducts[*supplierProductID]
			active := true
			if product.CurrentMapping.ResourceID != nil {
				if value, ok := r.resources[*product.CurrentMapping.ResourceID]; ok {
					active = value
				}
			}
			line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(product.CurrentMapping, active)
		} else {
			line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(domain.NewUnresolvedSupplierProductMapping(), true)
		}
		lines = append(lines, line)
	}
	r.lines[purchase.ID] = lines
	return domain.ImportResult{Purchase: purchase, Lines: lines, AlreadyExisted: false}, nil
}

func (r *memoryRepository) getOrCreateSupplierProduct(supplierID int64, sku, description string) int64 {
	for _, product := range r.supplierProducts {
		if product.SupplierID == supplierID && product.SupplierSKU == sku {
			product.Description = description
			r.supplierProducts[product.ID] = product
			return product.ID
		}
	}
	id := r.id()
	r.supplierProducts[id] = domain.SupplierProduct{ID: id, SupplierID: supplierID, SupplierSKU: sku, Description: description, CurrentMapping: domain.NewUnresolvedSupplierProductMapping()}
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

func (r *memoryRepository) refreshLine(line domain.PurchaseLine) domain.PurchaseLine {
	if line.SupplierProductID == nil {
		line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(domain.NewUnresolvedSupplierProductMapping(), true)
		return line
	}
	product := r.supplierProducts[*line.SupplierProductID]
	active := true
	if product.CurrentMapping.ResourceID != nil {
		if value, ok := r.resources[*product.CurrentMapping.ResourceID]; ok {
			active = value
		}
	}
	line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(product.CurrentMapping, active)
	return line
}

func (r *memoryRepository) ListPurchaseLines(_ context.Context, purchaseID int64) ([]domain.PurchaseLine, error) {
	lines := r.lines[purchaseID]
	for i := range lines {
		lines[i] = r.refreshLine(lines[i])
	}
	r.lines[purchaseID] = lines
	return lines, nil
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

func (r *memoryRepository) mappingProduct(id int64) (domain.SupplierProduct, error) {
	product, ok := r.supplierProducts[id]
	if !ok {
		return domain.SupplierProduct{}, domain.ErrSupplierProductNotFound
	}
	return product, nil
}

func (r *memoryRepository) applyMapping(id int64, transition domain.MappingTransition) (domain.SupplierProduct, error) {
	product, err := r.mappingProduct(id)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	if transition.Changed {
		r.supplierProducts[id] = product
		r.audit[id] = append(r.audit[id], *transition.Audit)
	}
	return product, nil
}

func (r *memoryRepository) ConfirmMapping(_ context.Context, command domain.ConfirmMappingCommand) (domain.SupplierProduct, error) {
	product, err := r.mappingProduct(command.SupplierProductID)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	if command.ResourceID <= 0 {
		return domain.SupplierProduct{}, domain.ErrResourceNotFound
	}
	r.resources[command.ResourceID] = true
	transition, err := product.ConfirmMapping(command.ResourceID, command.ExpectedRevision, command.Decision)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.supplierProducts[command.SupplierProductID] = product
	return r.applyMapping(command.SupplierProductID, transition)
}

func (r *memoryRepository) CorrectMapping(_ context.Context, command domain.CorrectMappingCommand) (domain.SupplierProduct, error) {
	product, err := r.mappingProduct(command.SupplierProductID)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.resources[command.ResourceID] = true
	transition, err := product.CorrectMapping(command.ExpectedCurrentResource, command.ResourceID, command.ExpectedRevision, command.Decision)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.supplierProducts[command.SupplierProductID] = product
	return r.applyMapping(command.SupplierProductID, transition)
}

func (r *memoryRepository) ExceptionalUnlink(_ context.Context, command domain.ExceptionalUnlinkCommand) (domain.SupplierProduct, error) {
	product, err := r.mappingProduct(command.SupplierProductID)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	transition, err := product.ExceptionalUnlink(command.ExpectedCurrentResource, command.ExpectedRevision, command.Decision)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.supplierProducts[command.SupplierProductID] = product
	return r.applyMapping(command.SupplierProductID, transition)
}

func (r *memoryRepository) ReportIdentityConflict(_ context.Context, command domain.ReportIdentityConflictCommand) (domain.SupplierProduct, error) {
	product, err := r.mappingProduct(command.SupplierProductID)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	transition, err := product.ReportIdentityConflict(command.ExpectedCurrentResource, command.ExpectedRevision, command.Decision)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.supplierProducts[command.SupplierProductID] = product
	return r.applyMapping(command.SupplierProductID, transition)
}

func (r *memoryRepository) ResolveIdentityConflict(_ context.Context, command domain.ResolveIdentityConflictCommand) (domain.SupplierProduct, error) {
	product, err := r.mappingProduct(command.SupplierProductID)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.resources[command.ResourceID] = true
	transition, err := product.ResolveIdentityConflict(command.ExpectedCurrentResource, command.ResourceID, command.ExpectedRevision, command.Decision)
	if err != nil {
		return domain.SupplierProduct{}, err
	}
	r.supplierProducts[command.SupplierProductID] = product
	return r.applyMapping(command.SupplierProductID, transition)
}

func (r *memoryRepository) updateLineOverride(lineID int64, override domain.LinkStatus) (domain.PurchaseLine, error) {
	for purchaseID, lines := range r.lines {
		for i, line := range lines {
			if line.ID != lineID {
				continue
			}
			var err error
			if override == domain.LinkStatusNone {
				line = line.ClearResolutionOverride()
			} else {
				line, err = line.SetResolutionOverride(override)
				if err != nil {
					return domain.PurchaseLine{}, err
				}
			}
			if line.SupplierProductID != nil {
				product := r.supplierProducts[*line.SupplierProductID]
				active := true
				if product.CurrentMapping.ResourceID != nil {
					if value, ok := r.resources[*product.CurrentMapping.ResourceID]; ok {
						active = value
					}
				}
				line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(product.CurrentMapping, active)
			} else {
				line.DerivedStatus, line.DerivedCause = line.EffectiveStatus(domain.NewUnresolvedSupplierProductMapping(), true)
			}
			lines[i] = line
			r.lines[purchaseID] = lines
			return line, nil
		}
	}
	return domain.PurchaseLine{}, domain.ErrPurchaseLineNotFound
}
func (r *memoryRepository) MarkNotApplicable(_ context.Context, lineID int64) (domain.PurchaseLine, error) {
	return r.updateLineOverride(lineID, domain.LinkNotApplicable)
}
func (r *memoryRepository) MarkConflict(_ context.Context, lineID int64) (domain.PurchaseLine, error) {
	return r.updateLineOverride(lineID, domain.LinkConflict)
}
func (r *memoryRepository) ClearOverride(_ context.Context, lineID int64) (domain.PurchaseLine, error) {
	return r.updateLineOverride(lineID, domain.LinkStatusNone)
}

func (r *memoryRepository) ListMappingAudit(_ context.Context, id int64, criteria domain.ListCriteria) ([]domain.MappingAuditEntry, error) {
	entries := r.audit[id]
	start := criteria.Offset
	if start < 0 {
		start = 0
	}
	if start > len(entries) {
		start = len(entries)
	}
	pageLimit := criteria.Limit
	if pageLimit <= 0 {
		pageLimit = 100
	}
	end := start + pageLimit
	if end > len(entries) {
		end = len(entries)
	}
	return entries[start:end], nil
}

func (r *memoryRepository) ListPurchaseLinesByResource(_ context.Context, resourceID int64, _ domain.ListCriteria) ([]domain.PurchaseLineHistory, error) {
	history := make([]domain.PurchaseLineHistory, 0)
	for _, purchase := range r.purchasesByID {
		for _, rawLine := range r.lines[purchase.ID] {
			line := r.refreshLine(rawLine)
			if line.SupplierProductID == nil {
				continue
			}
			product := r.supplierProducts[*line.SupplierProductID]
			if product.CurrentMapping.ResourceID == nil || *product.CurrentMapping.ResourceID != resourceID {
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

package app

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	supplierdomain "github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
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

func (r *memoryRepository) ListPurchaseLinesWorkbench(_ context.Context, criteria domain.PurchaseLineWorkbenchCriteria) ([]domain.PurchaseLineWorkbenchRow, error) {
	rows := make([]domain.PurchaseLineWorkbenchRow, 0)
	for _, purchase := range r.purchasesByID {
		if criteria.SupplierID != nil && purchase.Supplier.SupplierID != *criteria.SupplierID {
			continue
		}
		if criteria.DateFrom != nil && purchase.IssuedAt.Before(*criteria.DateFrom) {
			continue
		}
		if criteria.DateTo != nil && purchase.IssuedAt.After(*criteria.DateTo) {
			continue
		}
		for _, rawLine := range r.lines[purchase.ID] {
			line := r.refreshLine(rawLine)
			if criteria.EffectiveStatus != "" && line.DerivedStatus != criteria.EffectiveStatus {
				continue
			}
			if !containsFold(line.SupplierSKU, criteria.SupplierSKU) || !containsFold(line.Description, criteria.Description) {
				continue
			}
			if criteria.InvoiceText != "" && !containsFold(purchase.Series, criteria.InvoiceText) && !containsFold(purchase.Folio, criteria.InvoiceText) && !containsFold(purchase.CFDIUUID, criteria.InvoiceText) {
				continue
			}
			row := domain.PurchaseLineWorkbenchRow{
				LineID: line.ID, LineNumber: line.LineNumber, PurchaseID: purchase.ID, IssuedAt: purchase.IssuedAt, Series: purchase.Series, Folio: purchase.Folio,
				CFDIUUID: purchase.CFDIUUID, SupplierID: purchase.Supplier.SupplierID, SupplierDisplayName: strconv.FormatInt(purchase.Supplier.SupplierID, 10),
				Description: line.Description, SupplierSKU: line.SupplierSKU, SATProductCode: line.SATProductCode, Quantity: line.Quantity,
				UnitCode: line.UnitCode, Unit: line.Unit, UnitPrice: line.UnitPrice, Amount: line.Amount, Currency: purchase.Currency,
				SupplierProductID: line.SupplierProductID, ResolutionRevision: line.ResolutionRevision, ResolutionOverride: line.ResolutionOverride, EffectiveStatus: line.DerivedStatus, EffectiveCause: line.DerivedCause,
			}
			if line.SupplierProductID != nil {
				product := r.supplierProducts[*line.SupplierProductID]
				row.CommercialSupplierSKU = stringPointer(product.SupplierSKU)
				row.ResourceID = copyInt64ForTest(product.CurrentMapping.ResourceID)
				row.MappingRevision = mappingRevisionPointerForTest(product.MappingRevision)
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].IssuedAt.Equal(rows[j].IssuedAt) {
			return rows[i].LineID > rows[j].LineID
		}
		return rows[i].IssuedAt.After(rows[j].IssuedAt)
	})
	start := criteria.Offset
	if start < 0 {
		start = 0
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := start + criteria.Limit
	if criteria.Limit <= 0 || end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], nil
}

func containsFold(value, query string) bool {
	return query == "" || strings.Contains(strings.ToLower(value), strings.ToLower(query))
}

func stringPointer(value string) *string { return &value }

func copyInt64ForTest(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func mappingRevisionPointerForTest(value domain.MappingRevision) *domain.MappingRevision {
	copy := value
	return &copy
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

func (r *memoryRepository) SetResolutionOverride(_ context.Context, command domain.SetResolutionOverrideCommand) (domain.PurchaseLine, error) {
	for purchaseID, lines := range r.lines {
		for i, line := range lines {
			if line.ID != command.LineID {
				continue
			}
			if _, err := line.ChangeResolutionOverride(command.Override, command.ExpectedRevision, command.Decision); err != nil {
				return domain.PurchaseLine{}, err
			}
			line = r.refreshLine(line)
			lines[i] = line
			r.lines[purchaseID] = lines
			return line, nil
		}
	}
	return domain.PurchaseLine{}, domain.ErrPurchaseLineNotFound
}

func (r *memoryRepository) ResolvePurchaseLine(_ context.Context, command domain.ResolvePurchaseLineCommand) (domain.ResolvePurchaseLineResult, error) {
	for purchaseID, lines := range r.lines {
		for i, line := range lines {
			if line.ID != command.LineID {
				continue
			}
			if line.ResolutionRevision != command.ExpectedResolutionRevision || line.ResolutionOverride != domain.LinkStatusNone {
				return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
			}
			if !sameTestID(line.SupplierProductID, command.ExpectedSupplierProductID) {
				return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineStateConflict
			}
			var product domain.SupplierProduct
			identityCreated := false
			if line.SupplierProductID == nil {
				supplierID := r.purchasesByID[purchaseID].Supplier.SupplierID
				identityCreated = !r.hasSupplierProduct(supplierID, command.CommercialSupplierSKU)
				id := r.getOrCreateSupplierProduct(supplierID, command.CommercialSupplierSKU, line.Description)
				line.SupplierProductID = &id
				product = r.supplierProducts[id]
			} else {
				product = r.supplierProducts[*line.SupplierProductID]
			}
			expected := product.MappingRevision
			if command.ExpectedMappingRevision != nil {
				expected = *command.ExpectedMappingRevision
			}
			transition, err := product.ConfirmMapping(command.ResourceID, expected, command.Decision)
			if err != nil {
				return domain.ResolvePurchaseLineResult{}, err
			}
			r.resources[command.ResourceID] = true
			r.supplierProducts[product.ID] = product
			if transition.Changed {
				r.audit[product.ID] = append(r.audit[product.ID], *transition.Audit)
			}
			line = r.refreshLine(line)
			lines[i] = line
			r.lines[purchaseID] = lines
			disposition := domain.CommercialIdentityAlreadyMapped
			if identityCreated {
				disposition = domain.CommercialIdentityCreated
			} else if transition.Changed {
				disposition = domain.CommercialIdentityReused
			}
			return domain.ResolvePurchaseLineResult{
				Line: line, SupplierProduct: product, CommercialIdentityDisposition: disposition,
			}, nil
		}
	}
	return domain.ResolvePurchaseLineResult{}, domain.ErrPurchaseLineNotFound
}

func (r *memoryRepository) hasSupplierProduct(supplierID int64, sku string) bool {
	for _, product := range r.supplierProducts {
		if product.SupplierID == supplierID && product.SupplierSKU == sku {
			return true
		}
	}
	return false
}

func sameTestID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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

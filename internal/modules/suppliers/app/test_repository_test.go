package app

import (
	"context"
	"sort"
	"strings"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
)

type memoryRepository struct {
	nextID                 int64
	suppliers              map[int64]domain.Supplier
	branches               map[int64]domain.Branch
	contacts               map[int64]domain.Contact
	createContactCalls     int
	setSupplierActiveCalls int
	getByTaxIDCalls        []string
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{nextID: 1, suppliers: map[int64]domain.Supplier{}, branches: map[int64]domain.Branch{}, contacts: map[int64]domain.Contact{}}
}

func (r *memoryRepository) id() int64 { id := r.nextID; r.nextID++; return id }
func (r *memoryRepository) CreateSupplier(_ context.Context, value domain.Supplier) (domain.Supplier, error) {
	value.ID = r.id()
	r.suppliers[value.ID] = value
	return value, nil
}
func (r *memoryRepository) GetSupplier(_ context.Context, id int64) (domain.Supplier, error) {
	value, ok := r.suppliers[id]
	if !ok {
		return domain.Supplier{}, domain.ErrSupplierNotFound
	}
	return value, nil
}
func (r *memoryRepository) GetSupplierByTaxIdentifier(_ context.Context, taxID string) (domain.Supplier, error) {
	r.getByTaxIDCalls = append(r.getByTaxIDCalls, taxID)
	for _, value := range r.suppliers {
		if strings.EqualFold(strings.TrimSpace(value.TaxIdentifier), strings.TrimSpace(taxID)) && value.TaxIdentifier != "" {
			return value, nil
		}
	}
	return domain.Supplier{}, domain.ErrSupplierNotFound
}
func (r *memoryRepository) SearchSuppliers(_ context.Context, criteria domain.SupplierSearch) ([]domain.Supplier, error) {
	values := make([]domain.Supplier, 0)
	for _, value := range r.suppliers {
		if criteria.Active != nil && value.Active != *criteria.Active {
			continue
		}
		if criteria.Text != "" && !strings.Contains(strings.ToLower(value.TradeName+" "+value.LegalName+" "+value.TaxIdentifier), strings.ToLower(criteria.Text)) {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}
func (r *memoryRepository) UpdateSupplier(_ context.Context, value domain.Supplier) (domain.Supplier, error) {
	r.suppliers[value.ID] = value
	return value, nil
}
func (r *memoryRepository) SetSupplierActive(_ context.Context, id int64, active bool) (domain.Supplier, error) {
	r.setSupplierActiveCalls++
	value, ok := r.suppliers[id]
	if !ok {
		return domain.Supplier{}, domain.ErrSupplierNotFound
	}
	value.Active = active
	r.suppliers[id] = value
	return value, nil
}
func (r *memoryRepository) CreateBranch(_ context.Context, value domain.Branch) (domain.Branch, error) {
	value.ID = r.id()
	r.branches[value.ID] = value
	return value, nil
}
func (r *memoryRepository) GetBranch(_ context.Context, supplierID, id int64) (domain.Branch, error) {
	value, ok := r.branches[id]
	if !ok || value.SupplierID != supplierID {
		return domain.Branch{}, domain.ErrBranchNotFound
	}
	return value, nil
}
func (r *memoryRepository) ListBranches(_ context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error) {
	values := []domain.Branch{}
	for _, value := range r.branches {
		if value.SupplierID == supplierID && (criteria.Active == nil || value.Active == *criteria.Active) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}
func (r *memoryRepository) UpdateBranch(_ context.Context, value domain.Branch) (domain.Branch, error) {
	r.branches[value.ID] = value
	return value, nil
}
func (r *memoryRepository) SetBranchActive(_ context.Context, supplierID, id int64, active bool) (domain.Branch, error) {
	value, err := r.GetBranch(context.Background(), supplierID, id)
	if err != nil {
		return domain.Branch{}, err
	}
	value.Active = active
	r.branches[id] = value
	return value, nil
}
func (r *memoryRepository) CreateContact(_ context.Context, value domain.Contact) (domain.Contact, error) {
	r.createContactCalls++
	value.ID = r.id()
	r.contacts[value.ID] = value
	return value, nil
}
func (r *memoryRepository) GetContact(_ context.Context, supplierID, id int64) (domain.Contact, error) {
	value, ok := r.contacts[id]
	if !ok || value.SupplierID != supplierID {
		return domain.Contact{}, domain.ErrContactNotFound
	}
	return value, nil
}
func (r *memoryRepository) ListContacts(_ context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error) {
	values := []domain.Contact{}
	for _, value := range r.contacts {
		if value.SupplierID != supplierID || criteria.Active != nil && value.Active != *criteria.Active {
			continue
		}
		if criteria.BranchID != nil && (value.BranchID == nil || *value.BranchID != *criteria.BranchID) {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}
func (r *memoryRepository) UpdateContact(_ context.Context, value domain.Contact) (domain.Contact, error) {
	r.contacts[value.ID] = value
	return value, nil
}
func (r *memoryRepository) SetContactActive(_ context.Context, supplierID, id int64, active bool) (domain.Contact, error) {
	value, err := r.GetContact(context.Background(), supplierID, id)
	if err != nil {
		return domain.Contact{}, err
	}
	value.Active = active
	r.contacts[id] = value
	return value, nil
}

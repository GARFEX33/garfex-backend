package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

func (s *Service) AddContact(ctx context.Context, supplierID int64, details domain.ContactDetails) (domain.Contact, error) {
	if _, err := s.GetSupplier(ctx, supplierID); err != nil {
		return domain.Contact{}, err
	}
	contact, err := domain.NewContact(supplierID, details)
	if err != nil {
		return domain.Contact{}, err
	}
	if err := s.ensureBranchOwnership(ctx, supplierID, contact.BranchID); err != nil {
		return domain.Contact{}, err
	}
	created, err := s.repo.CreateContact(ctx, contact)
	return created, wrap("add contact", err)
}

func (s *Service) GetContact(ctx context.Context, supplierID, contactID int64) (domain.Contact, error) {
	if err := validID("supplier_id", supplierID); err != nil {
		return domain.Contact{}, err
	}
	if err := validID("contact_id", contactID); err != nil {
		return domain.Contact{}, err
	}
	contact, err := s.repo.GetContact(ctx, supplierID, contactID)
	return contact, wrap("get contact", err)
}

func (s *Service) ListContacts(ctx context.Context, supplierID int64, criteria domain.ContactListCriteria) ([]domain.Contact, error) {
	if _, err := s.GetSupplier(ctx, supplierID); err != nil {
		return nil, err
	}
	if err := s.ensureBranchOwnership(ctx, supplierID, criteria.BranchID); err != nil {
		return nil, err
	}
	contacts, err := s.repo.ListContacts(ctx, supplierID, criteria)
	return contacts, wrap("list contacts", err)
}

func (s *Service) UpdateContact(ctx context.Context, supplierID, contactID int64, details domain.ContactDetails) (domain.Contact, error) {
	current, err := s.GetContact(ctx, supplierID, contactID)
	if err != nil {
		return domain.Contact{}, err
	}
	next, err := current.WithDetails(details)
	if err != nil {
		return domain.Contact{}, err
	}
	if err := s.ensureBranchOwnership(ctx, supplierID, next.BranchID); err != nil {
		return domain.Contact{}, err
	}
	updated, err := s.repo.UpdateContact(ctx, next)
	return updated, wrap("update contact", err)
}

func (s *Service) DeactivateContact(ctx context.Context, supplierID, contactID int64) (domain.Contact, error) {
	return s.setContactActive(ctx, supplierID, contactID, false)
}

func (s *Service) ReactivateContact(ctx context.Context, supplierID, contactID int64) (domain.Contact, error) {
	return s.setContactActive(ctx, supplierID, contactID, true)
}

func (s *Service) setContactActive(ctx context.Context, supplierID, contactID int64, active bool) (domain.Contact, error) {
	current, err := s.GetContact(ctx, supplierID, contactID)
	if err != nil {
		return domain.Contact{}, err
	}
	var changed bool
	if active {
		changed = current.Activate()
	} else {
		changed = current.Deactivate()
	}
	if !changed {
		return current, nil
	}
	updated, err := s.repo.SetContactActive(ctx, supplierID, contactID, active)
	return updated, wrap("set contact active", err)
}

func (s *Service) ensureBranchOwnership(ctx context.Context, supplierID int64, branchID *int64) error {
	if branchID == nil {
		return nil
	}
	if _, err := s.GetBranch(ctx, supplierID, *branchID); err != nil {
		if errors.Is(err, domain.ErrBranchNotFound) {
			return fmt.Errorf("%w: branch %d is outside supplier %d", domain.ErrBranchOwnership, *branchID, supplierID)
		}
		return err
	}
	return nil
}

package app

import (
	"context"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

func (s *Service) AddBranch(ctx context.Context, supplierID int64, details domain.BranchDetails) (domain.Branch, error) {
	if _, err := s.GetSupplier(ctx, supplierID); err != nil {
		return domain.Branch{}, err
	}
	branch, err := domain.NewBranch(supplierID, details)
	if err != nil {
		return domain.Branch{}, err
	}
	created, err := s.repo.CreateBranch(ctx, branch)
	return created, wrap("add branch", err)
}

func (s *Service) GetBranch(ctx context.Context, supplierID, branchID int64) (domain.Branch, error) {
	if err := validID("supplier_id", supplierID); err != nil {
		return domain.Branch{}, err
	}
	if err := validID("branch_id", branchID); err != nil {
		return domain.Branch{}, err
	}
	branch, err := s.repo.GetBranch(ctx, supplierID, branchID)
	return branch, wrap("get branch", err)
}

func (s *Service) ListBranches(ctx context.Context, supplierID int64, criteria domain.ListCriteria) ([]domain.Branch, error) {
	if _, err := s.GetSupplier(ctx, supplierID); err != nil {
		return nil, err
	}
	branches, err := s.repo.ListBranches(ctx, supplierID, criteria)
	return branches, wrap("list branches", err)
}

func (s *Service) UpdateBranch(ctx context.Context, supplierID, branchID int64, details domain.BranchDetails) (domain.Branch, error) {
	current, err := s.GetBranch(ctx, supplierID, branchID)
	if err != nil {
		return domain.Branch{}, err
	}
	next, err := current.WithDetails(details)
	if err != nil {
		return domain.Branch{}, err
	}
	updated, err := s.repo.UpdateBranch(ctx, next)
	return updated, wrap("update branch", err)
}

func (s *Service) DeactivateBranch(ctx context.Context, supplierID, branchID int64) (domain.Branch, error) {
	return s.setBranchActive(ctx, supplierID, branchID, false)
}

func (s *Service) ReactivateBranch(ctx context.Context, supplierID, branchID int64) (domain.Branch, error) {
	return s.setBranchActive(ctx, supplierID, branchID, true)
}

func (s *Service) setBranchActive(ctx context.Context, supplierID, branchID int64, active bool) (domain.Branch, error) {
	current, err := s.GetBranch(ctx, supplierID, branchID)
	if err != nil {
		return domain.Branch{}, err
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
	updated, err := s.repo.SetBranchActive(ctx, supplierID, branchID, active)
	return updated, wrap("set branch active", err)
}

// Package app implements UI-independent Supplier Master use cases.
package app

import (
	"fmt"

	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
)

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

func validID(field string, id int64) error {
	if id <= 0 {
		return domain.NewValidationError(field, "must be positive")
	}
	return nil
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

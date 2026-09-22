package postgres

import (
	"reflect"
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/domain"
)

func TestAppendActiveFilterUsesExactStatusPredicate(t *testing.T) {
	tests := []struct {
		name   string
		status domain.CatalogStatus
		want   []string
	}{
		{"active zero value", domain.CatalogStatusActive, []string{"tenant = $1", "item.active"}},
		{"inactive", domain.CatalogStatusInactive, []string{"tenant = $1", "NOT item.active"}},
		{"all", domain.CatalogStatusAll, []string{"tenant = $1"}},
		{"unknown fails closed", domain.CatalogStatus(99), []string{"tenant = $1", "item.active"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appendActiveFilter([]string{"tenant = $1"}, tt.status, "item.active"); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("appendActiveFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

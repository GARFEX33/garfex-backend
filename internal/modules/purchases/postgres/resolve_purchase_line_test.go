package postgres

import (
	"testing"

	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
)

func TestCommercialIdentityDisposition(t *testing.T) {
	tests := []struct {
		name       string
		created    bool
		transition domain.MappingTransition
		want       domain.CommercialIdentityDisposition
	}{
		{name: "created identity", created: true, transition: domain.MappingTransition{Changed: true}, want: domain.CommercialIdentityCreated},
		{name: "reused identity with mapping transition", transition: domain.MappingTransition{Changed: true}, want: domain.CommercialIdentityReused},
		{name: "already mapped identity", want: domain.CommercialIdentityAlreadyMapped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commercialIdentityDisposition(tt.created, tt.transition); got != tt.want {
				t.Fatalf("commercialIdentityDisposition() = %q, want %q", got, tt.want)
			}
		})
	}
}

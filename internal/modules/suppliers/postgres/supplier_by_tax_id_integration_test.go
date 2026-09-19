package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GARFEX33/garfex-costos-unitarios/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestGetSupplierByTaxIdentifierIntegration proves against real PostgreSQL that
// the lookup matches the unique index semantics: exact, case and
// whitespace-insensitive, active or not, and never matching a supplier that has
// no tax identifier. The runtime role creates the fixtures, as in production;
// it cannot delete, so the admin role removes them by id.
func TestGetSupplierByTaxIdentifierIntegration(t *testing.T) {
	runtimeDSN := os.Getenv("GARFEX_TEST_DSN")
	adminDSN := os.Getenv("GARFEX_ADMIN_TEST_DSN")
	if runtimeDSN == "" || adminDSN == "" {
		t.Skip("GARFEX_TEST_DSN and GARFEX_ADMIN_TEST_DSN must target an isolated test database")
	}
	ctx := t.Context()

	runtimePool := openSupplierTestPool(t, runtimeDSN)
	adminPool := openSupplierTestPool(t, adminDSN)
	repo := NewRepository(runtimePool)

	var created []int64
	t.Cleanup(func() {
		if len(created) == 0 {
			return
		}
		if _, err := adminPool.Exec(context.Background(), `DELETE FROM public.suppliers WHERE id = ANY($1)`, created); err != nil {
			t.Errorf("cleanup fixtures: %v", err)
		}
	})
	create := func(details domain.SupplierDetails) domain.Supplier {
		t.Helper()
		supplier, err := domain.NewSupplier(details)
		if err != nil {
			t.Fatalf("build supplier: %v", err)
		}
		got, err := repo.CreateSupplier(ctx, supplier)
		if err != nil {
			t.Fatalf("create supplier: %v", err)
		}
		created = append(created, got.ID)
		return got
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	activeRFC := "TSTA" + unique
	inactiveRFC := "TSTI" + unique
	active := create(domain.SupplierDetails{LegalName: "TEST ACTIVE " + unique, TaxIdentifier: activeRFC})
	inactive := create(domain.SupplierDetails{LegalName: "TEST INACTIVE " + unique, TaxIdentifier: inactiveRFC})
	create(domain.SupplierDetails{LegalName: "TEST NO TAX ID " + unique})
	if _, err := repo.SetSupplierActive(ctx, inactive.ID, false); err != nil {
		t.Fatalf("deactivate fixture: %v", err)
	}

	tests := []struct {
		name       string
		taxID      string
		wantID     int64
		wantActive bool
		wantErr    error
	}{
		{"exact match", activeRFC, active.ID, true, nil},
		{"lowercase input", "tsta" + unique, active.ID, true, nil},
		// btrim trims spaces only, exactly like the unique index. Tabs and
		// newlines are trimmed by the service (strings.TrimSpace) before the
		// repository is called; that path is covered in the app package.
		{"space padded input", "  " + activeRFC + "  ", active.ID, true, nil},
		{"inactive supplier is still found", inactiveRFC, inactive.ID, false, nil},
		{"unknown identifier", "TSTX" + unique, 0, false, domain.ErrSupplierNotFound},
		{"partial identifier is not a match", activeRFC[:len(activeRFC)-3], 0, false, domain.ErrSupplierNotFound},
		{"blank never matches a supplier without tax id", "", 0, false, domain.ErrSupplierNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetSupplierByTaxIdentifier(ctx, tt.taxID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got.ID != tt.wantID {
				t.Fatalf("id = %d, want %d", got.ID, tt.wantID)
			}
			if tt.wantErr == nil && got.Active != tt.wantActive {
				t.Fatalf("active = %t, want %t", got.Active, tt.wantActive)
			}
		})
	}
}

func openSupplierTestPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect test PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

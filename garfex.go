// Package garfex exposes the composed GARFEX Core application.
package garfex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/GARFEX33/garfex-backend/internal/app/catalogo"
	"github.com/GARFEX33/garfex-backend/internal/app/recursos"
	purchasebridge "github.com/GARFEX33/garfex-backend/internal/bridge/purchasecore"
	resourcebridge "github.com/GARFEX33/garfex-backend/internal/bridge/resourcecore"
	supplierbridge "github.com/GARFEX33/garfex-backend/internal/bridge/suppliercore"
	"github.com/GARFEX33/garfex-backend/internal/domain"
	"github.com/GARFEX33/garfex-backend/internal/modules/purchases"
	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers"
	"github.com/GARFEX33/garfex-backend/internal/postgres"
	"github.com/GARFEX33/garfex-backend/purchasecore"
	"github.com/GARFEX33/garfex-backend/resourcecore"
	"github.com/GARFEX33/garfex-backend/suppliercore"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config configures one Core application instance.
type Config struct {
	// DSN is required. pgx parses the supplied connection string and may apply
	// its documented PG* defaults to connection parameters omitted from it.
	DSN string
}

// Application owns the Core services and their PostgreSQL connection pool.
//
// Application must not be copied after first use. Close drains and closes the
// pool; pgxpool.Close waits for acquired connections to be returned.
type Application struct {
	ResourceReader *resourcecore.Reader
	ResourceWriter *resourcecore.Writer
	SupplierReader *suppliercore.Reader
	SupplierWriter *suppliercore.Writer
	PurchaseReader *purchasecore.Reader
	PurchaseWriter *purchasecore.Writer

	pool      *pgxpool.Pool
	closeOnce sync.Once
}

// Open creates a fully initialized Core application. It requires an explicit
// non-blank DSN, loads the catalog eagerly with ctx, and returns no partial
// application on failure.
func Open(ctx context.Context, config Config) (*Application, error) {
	if strings.TrimSpace(config.DSN) == "" {
		return nil, errors.New("garfex: invalid database configuration")
	}
	if err := ctx.Err(); err != nil {
		return nil, safeOpenError(err)
	}

	poolConfig, err := pgxpool.ParseConfig(config.DSN)
	if err != nil {
		return nil, errors.New("garfex: invalid database configuration")
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, safeOpenError(err)
	}

	catalog, err := postgres.LoadResourceCatalog(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}

	authority := domain.NewCatalogAuthority(catalog)
	catalogService := catalogo.NewServiceWithCatalogAuthority(
		postgres.NewCatalogAdminRepository(pool),
		domain.NewCatalogRegistry(),
		authority,
	).WithCatalogAdminRepositoryV2(postgres.NewCatalogAdminRepositoryV2(pool))
	resourceService := recursos.NewServiceWithCatalogAuthority(
		postgres.NewResourceRepository(pool),
		authority,
	).WithAttributeOrderStore(postgres.NewAttributeOrderRepositoryFull(pool))
	resourceAdapter := resourcebridge.NewAdapter(catalogService, resourceService)
	resourceReader, err := resourcecore.NewReadOnly(resourceAdapter)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}
	resourceWriter, err := resourcecore.NewWriter(resourceAdapter)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}

	suppliersModule := suppliers.New(pool)
	supplierAdapter := supplierbridge.NewAdapter(suppliersModule.Service)
	supplierReader, err := suppliercore.NewReadOnly(supplierAdapter)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}
	supplierWriter, err := suppliercore.NewWriter(supplierAdapter)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}

	purchaseAdapter := purchasebridge.NewAdapter(purchases.New(pool, suppliersModule.Service).Service)
	purchaseReader, err := purchasecore.NewReadOnly(purchaseAdapter)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}
	purchaseWriter, err := purchasecore.NewWriter(purchaseAdapter)
	if err != nil {
		pool.Close()
		return nil, safeOpenError(err)
	}

	return &Application{
		ResourceReader: resourceReader,
		ResourceWriter: resourceWriter,
		SupplierReader: supplierReader,
		SupplierWriter: supplierWriter,
		PurchaseReader: purchaseReader,
		PurchaseWriter: purchaseWriter,
		pool:           pool,
	}, nil
}

// Close releases Core-owned PostgreSQL resources. It is safe to call multiple
// times and must be called when the application is no longer in use.
func (a *Application) Close() {
	if a == nil {
		return
	}
	a.closeOnce.Do(func() {
		if a.pool != nil {
			a.pool.Close()
		}
	})
}

func safeOpenError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("garfex: initialization canceled: %w", context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("garfex: initialization timed out: %w", context.DeadlineExceeded)
	default:
		return errors.New("garfex: application initialization failed")
	}
}

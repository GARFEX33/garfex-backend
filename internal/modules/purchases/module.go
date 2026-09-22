// Package purchases composes the interface-independent Purchase and Price
// History backend, reusing the Supplier Master and Resource Master cores.
package purchases

import (
	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/app"
	"github.com/GARFEX33/garfex-backend/internal/modules/purchases/domain"
	purchasepostgres "github.com/GARFEX33/garfex-backend/internal/modules/purchases/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	Service    *app.Service
	Repository domain.Repository
}

// New builds the backend module without attaching it to any delivery
// interface. suppliers resolves and minimally registers the supplier that
// issued each imported purchase document.
func New(pool *pgxpool.Pool, suppliers app.SupplierDirectory) Module {
	repository := purchasepostgres.NewRepository(pool)
	return Module{Service: app.NewService(repository, suppliers), Repository: repository}
}

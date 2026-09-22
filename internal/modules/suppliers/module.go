// Package suppliers composes the interface-independent Supplier Master backend.
package suppliers

import (
	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/app"
	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	supplierpostgres "github.com/GARFEX33/garfex-backend/internal/modules/suppliers/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	Service    *app.Service
	Repository domain.Repository
}

// New builds the backend module without attaching it to any delivery interface.
func New(pool *pgxpool.Pool) Module {
	repository := supplierpostgres.NewRepository(pool)
	return Module{Service: app.NewService(repository), Repository: repository}
}

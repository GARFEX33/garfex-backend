package postgres

import (
	"github.com/GARFEX33/garfex-backend/internal/modules/suppliers/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ domain.Repository = (*repository)(nil)

func NewRepository(pool *pgxpool.Pool) domain.Repository { return &repository{pool: pool} }

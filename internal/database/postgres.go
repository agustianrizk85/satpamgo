package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/config"
)

func NewPostgresPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 1
	poolConfig.MaxConnIdleTime = 30 * time.Second
	poolConfig.MaxConnLifetime = 30 * time.Minute

	return pgxpool.NewWithConfig(ctx, poolConfig)
}

package tokenconfig

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("token config not found")

type Repository struct{ db *pgxpool.Pool }

type TokenConfig struct {
	AccessTTLSeconds  int       `json:"access_ttl_seconds"`
	RefreshTTLSeconds int       `json:"refresh_ttl_seconds"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context) (*TokenConfig, error) {
	const sql = `
		select access_ttl_seconds, refresh_ttl_seconds, created_at, updated_at
		from token_configs
		where id = true
		limit 1
	`
	var item TokenConfig
	err := r.db.QueryRow(ctx, sql).Scan(&item.AccessTTLSeconds, &item.RefreshTTLSeconds, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Upsert(ctx context.Context, accessTTLSeconds, refreshTTLSeconds int) (*TokenConfig, error) {
	const sql = `
		insert into token_configs (id, access_ttl_seconds, refresh_ttl_seconds)
		values (true, $1, $2)
		on conflict (id)
		do update set
			access_ttl_seconds = excluded.access_ttl_seconds,
			refresh_ttl_seconds = excluded.refresh_ttl_seconds,
			updated_at = now()
		returning access_ttl_seconds, refresh_ttl_seconds, created_at, updated_at
	`
	var item TokenConfig
	if err := r.db.QueryRow(ctx, sql, accessTTLSeconds, refreshTTLSeconds).Scan(&item.AccessTTLSeconds, &item.RefreshTTLSeconds, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

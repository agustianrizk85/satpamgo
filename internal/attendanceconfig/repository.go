package attendanceconfig

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound   = errors.New("attendance config not found")
	ErrForeignKey = errors.New("place not found")
)

type Repository struct{ db *pgxpool.Pool }

type AttendanceConfig struct {
	ID              string    `json:"id"`
	PlaceID         string    `json:"place_id"`
	AllowedRadiusM  int       `json:"allowed_radius_m"`
	CenterLatitude  *float64  `json:"center_latitude"`
	CenterLongitude *float64  `json:"center_longitude"`
	RequirePhoto    bool      `json:"require_photo"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type UpsertInput struct {
	PlaceID         string
	AllowedRadiusM  int
	CenterLatitude  *float64
	CenterLongitude *float64
	RequirePhoto    bool
	IsActive        bool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) GetByPlaceID(ctx context.Context, placeID string) (*AttendanceConfig, error) {
	const sql = `select id, place_id, allowed_radius_m, center_latitude::float8, center_longitude::float8, require_photo, is_active, created_at, updated_at from attendance_config where place_id = $1 limit 1`
	var item AttendanceConfig
	err := r.db.QueryRow(ctx, sql, placeID).Scan(&item.ID, &item.PlaceID, &item.AllowedRadiusM, &item.CenterLatitude, &item.CenterLongitude, &item.RequirePhoto, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Upsert(ctx context.Context, input UpsertInput) (*AttendanceConfig, error) {
	const sql = `
		insert into attendance_config (place_id, allowed_radius_m, center_latitude, center_longitude, require_photo, is_active)
		values ($1,$2,$3,$4,$5,$6)
		on conflict (place_id)
		do update set
			allowed_radius_m = excluded.allowed_radius_m,
			center_latitude = excluded.center_latitude,
			center_longitude = excluded.center_longitude,
			require_photo = excluded.require_photo,
			is_active = excluded.is_active,
			updated_at = now()
		returning id, place_id, allowed_radius_m, center_latitude::float8, center_longitude::float8, require_photo, is_active, created_at, updated_at
	`
	var item AttendanceConfig
	err := r.db.QueryRow(ctx, sql, input.PlaceID, input.AllowedRadiusM, input.CenterLatitude, input.CenterLongitude, input.RequirePhoto, input.IsActive).Scan(
		&item.ID, &item.PlaceID, &item.AllowedRadiusM, &item.CenterLatitude, &item.CenterLongitude, &item.RequirePhoto, &item.IsActive, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if isPgCode(err, "23503") {
			return nil, ErrForeignKey
		}
		return nil, err
	}
	return &item, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

package places

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
)

var (
	ErrPlaceNotFound         = errors.New("place not found")
	ErrPlaceCodeExists       = errors.New("place code already exists")
	ErrLatLngOutOfRange      = errors.New("latitude/longitude out of range")
	ErrPlaceNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct {
	db *pgxpool.Pool
}

type Place struct {
	ID        string     `json:"id"`
	PlaceCode string     `json:"place_code"`
	PlaceName string     `json:"place_name"`
	Address   *string    `json:"address"`
	Latitude  *float64   `json:"latitude"`
	Longitude *float64   `json:"longitude"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	Query       listquery.Query
}

type CreateInput struct {
	PlaceCode string
	PlaceName string
	Address   *string
	Latitude  *float64
	Longitude *float64
	Status    string
}

type UpdateInput struct {
	PlaceCode *string
	PlaceName *string
	Address   **string
	Latitude  **float64
	Longitude **float64
	Status    *string
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[Place], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"placeCode": "place_code",
		"placeName": "place_name",
		"status":    "status",
	}[params.Query.SortBy]
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select
			id, place_code, place_name, address, latitude::float8, longitude::float8, status, created_at, updated_at, deleted_at,
			count(*) over()::int as total_count
		from places
		where deleted_at is null
	`
	args := []any{params.Query.PageSize, params.Query.Offset}

	if !auth.IsGlobalAdminRole(params.ActorRole) {
		sql = `
			select
				p.id, p.place_code, p.place_name, p.address, p.latitude::float8, p.longitude::float8, p.status, p.created_at, p.updated_at, p.deleted_at,
				count(*) over()::int as total_count
			from places p
			where p.deleted_at is null
			  and exists (
				select 1
				from user_place_roles upr
				where upr.place_id = p.id
				  and upr.user_id = $1
				  and upr.is_active = true
			  )
		`
		args = []any{params.ActorUserID, params.Query.PageSize, params.Query.Offset}
	}

	sql += fmt.Sprintf(`
		order by %s %s, id asc
		limit $%d offset $%d
	`, sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Place]{}, err
	}
	defer rows.Close()

	data := make([]Place, 0)
	total := 0
	for rows.Next() {
		var item Place
		if err := rows.Scan(&item.ID, &item.PlaceCode, &item.PlaceName, &item.Address, &item.Latitude, &item.Longitude, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &total); err != nil {
			return listquery.Response[Place]{}, err
		}
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		return listquery.Response[Place]{}, err
	}

	return listquery.BuildResponse(data, params.Query, total), nil
}

func (r *Repository) FindByID(ctx context.Context, placeID string) (*Place, error) {
	const sql = `
		select
			id, place_code, place_name, address, latitude::float8, longitude::float8, status, created_at, updated_at, deleted_at
		from places
		where id = $1
		  and deleted_at is null
		limit 1
	`

	var item Place
	err := r.db.QueryRow(ctx, sql, placeID).Scan(&item.ID, &item.PlaceCode, &item.PlaceName, &item.Address, &item.Latitude, &item.Longitude, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlaceNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	const sql = `
		insert into places (place_code, place_name, address, latitude, longitude, status)
		values ($1, $2, $3, $4, $5, $6)
		returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, strings.TrimSpace(input.PlaceCode), strings.TrimSpace(input.PlaceName), input.Address, input.Latitude, input.Longitude, input.Status).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return "", ErrPlaceCodeExists
		case isPgCode(err, "22003"):
			return "", ErrLatLngOutOfRange
		default:
			return "", err
		}
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, placeID string, input UpdateInput) (*Place, error) {
	setParts := make([]string, 0, 6)
	args := make([]any, 0, 7)

	if input.PlaceCode != nil {
		args = append(args, strings.TrimSpace(*input.PlaceCode))
		setParts = append(setParts, fmt.Sprintf("place_code = $%d", len(args)))
	}
	if input.PlaceName != nil {
		args = append(args, strings.TrimSpace(*input.PlaceName))
		setParts = append(setParts, fmt.Sprintf("place_name = $%d", len(args)))
	}
	if input.Address != nil {
		args = append(args, *input.Address)
		setParts = append(setParts, fmt.Sprintf("address = $%d", len(args)))
	}
	if input.Latitude != nil {
		args = append(args, *input.Latitude)
		setParts = append(setParts, fmt.Sprintf("latitude = $%d", len(args)))
	}
	if input.Longitude != nil {
		args = append(args, *input.Longitude)
		setParts = append(setParts, fmt.Sprintf("longitude = $%d", len(args)))
	}
	if input.Status != nil {
		args = append(args, *input.Status)
		setParts = append(setParts, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrPlaceNoFieldsToUpdate
	}

	setParts = append(setParts, "updated_at = now()")
	args = append(args, placeID)

	sql := fmt.Sprintf(`
		update places
		set %s
		where id = $%d
		  and deleted_at is null
		returning id, place_code, place_name, address, latitude::float8, longitude::float8, status, created_at, updated_at, deleted_at
	`, strings.Join(setParts, ", "), len(args))

	var item Place
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceCode, &item.PlaceName, &item.Address, &item.Latitude, &item.Longitude, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrPlaceNotFound
		case isPgCode(err, "23505"):
			return nil, ErrPlaceCodeExists
		case isPgCode(err, "22003"):
			return nil, ErrLatLngOutOfRange
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) SoftDelete(ctx context.Context, placeID string) (string, error) {
	const sql = `
		update places
		set deleted_at = now(),
			updated_at = now()
		where id = $1
		  and deleted_at is null
		returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, placeID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPlaceNotFound
		}
		return "", err
	}
	return id, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

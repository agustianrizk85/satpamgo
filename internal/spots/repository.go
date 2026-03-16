package spots

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
	ErrNotFound         = errors.New("spot not found")
	ErrAlreadyExists    = errors.New("spot already exists")
	ErrPlaceNotFound    = errors.New("place not found")
	ErrNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct{ db *pgxpool.Pool }

type Spot struct {
	ID        string     `json:"id"`
	PlaceID   string     `json:"place_id"`
	SpotCode  string     `json:"spot_code"`
	SpotName  string     `json:"spot_name"`
	Latitude  *float64   `json:"latitude"`
	Longitude *float64   `json:"longitude"`
	QRToken   *string    `json:"qr_token"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	Query       listquery.Query
}

type CreateInput struct {
	PlaceID   string
	SpotCode  string
	SpotName  string
	Latitude  *float64
	Longitude *float64
	QRToken   *string
	Status    string
}

type UpdateInput struct {
	PlaceID   *string
	SpotCode  *string
	SpotName  *string
	Latitude  **float64
	Longitude **float64
	QRToken   **string
	Status    *string
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[Spot], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"spotCode":  "spot_code",
		"spotName":  "spot_name",
		"status":    "status",
		"placeId":   "place_id",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select id, place_id, spot_code, spot_name, latitude::float8, longitude::float8, qr_token, status, created_at, updated_at, deleted_at, count(*) over()::int as total_count
		from spots
		where deleted_at is null
	`
	args := []any{}
	if params.PlaceID != "" {
		args = append(args, params.PlaceID)
		sql += fmt.Sprintf(" and place_id = $%d", len(args))
	} else if !auth.IsGlobalAdminRole(params.ActorRole) {
		args = append(args, params.ActorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Spot]{}, err
	}
	defer rows.Close()
	data := make([]Spot, 0)
	total := 0
	for rows.Next() {
		var item Spot
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.SpotCode, &item.SpotName, &item.Latitude, &item.Longitude, &item.QRToken, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &total); err != nil {
			return listquery.Response[Spot]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) FindByID(ctx context.Context, id string) (*Spot, error) {
	const sql = `
		select id, place_id, spot_code, spot_name, latitude::float8, longitude::float8, qr_token, status, created_at, updated_at, deleted_at
		from spots where id = $1 and deleted_at is null limit 1
	`
	var item Spot
	err := r.db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.PlaceID, &item.SpotCode, &item.SpotName, &item.Latitude, &item.Longitude, &item.QRToken, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	const sql = `
		insert into spots (place_id, spot_code, spot_name, latitude, longitude, qr_token, status)
		values ($1,$2,$3,$4,$5,$6,$7) returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, input.PlaceID, strings.TrimSpace(input.SpotCode), strings.TrimSpace(input.SpotName), input.Latitude, input.Longitude, input.QRToken, input.Status).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return "", ErrAlreadyExists
		case isPgCode(err, "23503"):
			return "", ErrPlaceNotFound
		default:
			return "", err
		}
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput) (*Spot, error) {
	setParts := make([]string, 0, 7)
	args := make([]any, 0, 8)
	if input.PlaceID != nil {
		args = append(args, *input.PlaceID)
		setParts = append(setParts, fmt.Sprintf("place_id = $%d", len(args)))
	}
	if input.SpotCode != nil {
		args = append(args, strings.TrimSpace(*input.SpotCode))
		setParts = append(setParts, fmt.Sprintf("spot_code = $%d", len(args)))
	}
	if input.SpotName != nil {
		args = append(args, strings.TrimSpace(*input.SpotName))
		setParts = append(setParts, fmt.Sprintf("spot_name = $%d", len(args)))
	}
	if input.Latitude != nil {
		args = append(args, *input.Latitude)
		setParts = append(setParts, fmt.Sprintf("latitude = $%d", len(args)))
	}
	if input.Longitude != nil {
		args = append(args, *input.Longitude)
		setParts = append(setParts, fmt.Sprintf("longitude = $%d", len(args)))
	}
	if input.QRToken != nil {
		args = append(args, *input.QRToken)
		setParts = append(setParts, fmt.Sprintf("qr_token = $%d", len(args)))
	}
	if input.Status != nil {
		args = append(args, *input.Status)
		setParts = append(setParts, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrNoFieldsToUpdate
	}
	setParts = append(setParts, "updated_at = now()")
	args = append(args, id)
	sql := fmt.Sprintf(`
		update spots set %s
		where id = $%d and deleted_at is null
		returning id, place_id, spot_code, spot_name, latitude::float8, longitude::float8, qr_token, status, created_at, updated_at, deleted_at
	`, strings.Join(setParts, ", "), len(args))
	var item Spot
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.SpotCode, &item.SpotName, &item.Latitude, &item.Longitude, &item.QRToken, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.DeletedAt)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrNotFound
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrPlaceNotFound
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) SoftDelete(ctx context.Context, id string) (string, error) {
	const sql = `update spots set deleted_at = now(), updated_at = now() where id = $1 and deleted_at is null returning id`
	var out string
	err := r.db.QueryRow(ctx, sql, id).Scan(&out)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return out, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

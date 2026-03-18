package visitors

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
	ErrNotFound         = errors.New("visitor not found")
	ErrAlreadyExists    = errors.New("visitor already exists")
	ErrPlaceNotFound    = errors.New("place not found")
	ErrNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct{ db *pgxpool.Pool }

type Visitor struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	UserID    string    `json:"user_id"`
	NIK       string    `json:"nik"`
	Nama      string    `json:"nama"`
	Tujuan    *string   `json:"tujuan"`
	Catatan   *string   `json:"catatan"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	UserID      string
	Query       listquery.Query
}

type CreateInput struct {
	PlaceID string
	UserID  string
	NIK     string
	Nama    string
	Tujuan  *string
	Catatan *string
}

type UpdateInput struct {
	PlaceID *string
	UserID  *string
	NIK     *string
	Nama    *string
	Tujuan  **string
	Catatan **string
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[Visitor], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"placeId":   "place_id",
		"userId":    "user_id",
		"nik":       "nik",
		"nama":      "nama",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select id, place_id, user_id, nik, nama, tujuan, catatan, created_at, updated_at, count(*) over()::int as total_count
		from visitors
		where true
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
	if params.UserID != "" {
		args = append(args, params.UserID)
		sql += fmt.Sprintf(" and user_id = $%d", len(args))
	}
	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(" order by %s %s nulls last, created_at desc, id desc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Visitor]{}, err
	}
	defer rows.Close()

	data := make([]Visitor, 0)
	total := 0
	for rows.Next() {
		var item Visitor
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.NIK, &item.Nama, &item.Tujuan, &item.Catatan, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Visitor]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) FindByID(ctx context.Context, id string) (*Visitor, error) {
	const sql = `
		select id, place_id, user_id, nik, nama, tujuan, catatan, created_at, updated_at
		from visitors
		where id = $1
		limit 1
	`

	var item Visitor
	err := r.db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.NIK, &item.Nama, &item.Tujuan, &item.Catatan, &item.CreatedAt, &item.UpdatedAt)
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
		insert into visitors (place_id, user_id, nik, nama, tujuan, catatan)
		values ($1,$2,$3,$4,$5,$6)
		returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, input.PlaceID, input.UserID, strings.TrimSpace(input.NIK), strings.TrimSpace(input.Nama), input.Tujuan, input.Catatan).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23503"):
			return "", ErrPlaceNotFound
		default:
			return "", err
		}
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput) (*Visitor, error) {
	setParts := make([]string, 0, 6)
	args := make([]any, 0, 7)
	addNullableText := func(column string, value **string) {
		args = append(args, *value)
		setParts = append(setParts, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if input.PlaceID != nil {
		args = append(args, *input.PlaceID)
		setParts = append(setParts, fmt.Sprintf("place_id = $%d", len(args)))
	}
	if input.UserID != nil {
		args = append(args, *input.UserID)
		setParts = append(setParts, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if input.NIK != nil {
		args = append(args, strings.TrimSpace(*input.NIK))
		setParts = append(setParts, fmt.Sprintf("nik = $%d", len(args)))
	}
	if input.Nama != nil {
		args = append(args, strings.TrimSpace(*input.Nama))
		setParts = append(setParts, fmt.Sprintf("nama = $%d", len(args)))
	}
	if input.Tujuan != nil {
		addNullableText("tujuan", input.Tujuan)
	}
	if input.Catatan != nil {
		addNullableText("catatan", input.Catatan)
	}
	if len(setParts) == 0 {
		return nil, ErrNoFieldsToUpdate
	}

	setParts = append(setParts, "updated_at = now()")
	args = append(args, id)
	sql := fmt.Sprintf(`
		update visitors set %s
		where id = $%d
		returning id, place_id, user_id, nik, nama, tujuan, catatan, created_at, updated_at
	`, strings.Join(setParts, ", "), len(args))

	var item Visitor
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.NIK, &item.Nama, &item.Tujuan, &item.Catatan, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrNotFound
		case isPgCode(err, "23503"):
			return nil, ErrPlaceNotFound
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) Delete(ctx context.Context, id string) (string, error) {
	const sql = `delete from visitors where id = $1 returning id`
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

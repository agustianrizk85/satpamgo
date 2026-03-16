package spotassignments

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
	ErrNotFound         = errors.New("spot assignment not found")
	ErrAlreadyExists    = errors.New("spot assignment already exists")
	ErrForeignKey       = errors.New("related row not found")
	ErrNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct{ db *pgxpool.Pool }

type SpotAssignment struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	UserID    string    `json:"user_id"`
	ShiftID   string    `json:"shift_id"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	UserID      string
	IsActive    *bool
	Query       listquery.Query
}

type CreateInput struct {
	PlaceID  string
	UserID   string
	ShiftID  string
	IsActive bool
}

type UpdateInput struct {
	PlaceID  *string
	UserID   *string
	ShiftID  *string
	IsActive *bool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[SpotAssignment], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"placeId":   "place_id",
		"userId":    "user_id",
		"shiftId":   "shift_id",
		"isActive":  "is_active",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select id, place_id, user_id, shift_id, is_active, created_at, updated_at, count(*) over()::int as total_count
		from spot_assignments
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
	if params.IsActive != nil {
		args = append(args, *params.IsActive)
		sql += fmt.Sprintf(" and is_active = $%d", len(args))
	}
	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[SpotAssignment]{}, err
	}
	defer rows.Close()
	data := make([]SpotAssignment, 0)
	total := 0
	for rows.Next() {
		var item SpotAssignment
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.ShiftID, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[SpotAssignment]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) FindByID(ctx context.Context, id string) (*SpotAssignment, error) {
	const sql = `select id, place_id, user_id, shift_id, is_active, created_at, updated_at from spot_assignments where id = $1 limit 1`
	var item SpotAssignment
	err := r.db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.ShiftID, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if input.IsActive {
		if _, err := tx.Exec(ctx, `
			update spot_assignments
			set is_active = false,
			    updated_at = now()
			where place_id = $1
			  and user_id = $2
			  and is_active = true
		`, input.PlaceID, input.UserID); err != nil {
			return "", err
		}
	}

	const sql = `insert into spot_assignments (place_id, user_id, shift_id, is_active) values ($1,$2,$3,$4) returning id`
	var id string
	err = tx.QueryRow(ctx, sql, input.PlaceID, input.UserID, input.ShiftID, input.IsActive).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23503"):
			return "", ErrForeignKey
		default:
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput) (*SpotAssignment, error) {
	setParts := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if input.PlaceID != nil {
		args = append(args, *input.PlaceID)
		setParts = append(setParts, fmt.Sprintf("place_id = $%d", len(args)))
	}
	if input.UserID != nil {
		args = append(args, *input.UserID)
		setParts = append(setParts, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if input.ShiftID != nil {
		args = append(args, *input.ShiftID)
		setParts = append(setParts, fmt.Sprintf("shift_id = $%d", len(args)))
	}
	if input.IsActive != nil {
		args = append(args, *input.IsActive)
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrNoFieldsToUpdate
	}
	setParts = append(setParts, "updated_at = now()")
	args = append(args, id)
	sql := fmt.Sprintf(`update spot_assignments set %s where id = $%d returning id, place_id, user_id, shift_id, is_active, created_at, updated_at`, strings.Join(setParts, ", "), len(args))
	var item SpotAssignment
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.ShiftID, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrNotFound
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) Delete(ctx context.Context, id string) (string, error) {
	const sql = `delete from spot_assignments where id = $1 returning id`
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

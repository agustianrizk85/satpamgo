package shifts

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
	ErrShiftNotFound         = errors.New("shift not found")
	ErrShiftAlreadyExists    = errors.New("shift already exists")
	ErrShiftPlaceNotFound    = errors.New("place not found")
	ErrShiftStillInUse       = errors.New("shift is still in use")
	ErrShiftNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct {
	db *pgxpool.Pool
}

type Shift struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	Name      string    `json:"name"`
	StartTime string    `json:"start_time"`
	EndTime   string    `json:"end_time"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	Query       listquery.Query
}

type CreateInput struct {
	PlaceID   string
	Name      string
	StartTime string
	EndTime   string
	IsActive  bool
}

type UpdateInput struct {
	PlaceID   *string
	Name      *string
	StartTime *string
	EndTime   *string
	IsActive  *bool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[Shift], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"name":      "name",
		"startTime": "start_time",
		"endTime":   "end_time",
		"isActive":  "is_active",
	}[params.Query.SortBy]
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select
			id, place_id, name, start_time::text, end_time::text, is_active, created_at, updated_at,
			count(*) over()::int as total_count
		from shifts
	`
	args := make([]any, 0, 4)

	switch {
	case params.PlaceID != "":
		args = append(args, params.PlaceID)
		sql += " where place_id = $1"
	case !auth.IsGlobalAdminRole(params.ActorRole):
		args = append(args, params.ActorUserID)
		sql += `
			where place_id in (
				select distinct upr.place_id
				from user_place_roles upr
				join places p on p.id = upr.place_id
				where upr.user_id = $1
				  and upr.is_active = true
				  and p.deleted_at is null
			)
		`
	}

	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(`
		order by %s %s, id asc
		limit $%d offset $%d
	`, sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Shift]{}, err
	}
	defer rows.Close()

	data := make([]Shift, 0)
	total := 0
	for rows.Next() {
		var item Shift
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.Name, &item.StartTime, &item.EndTime, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Shift]{}, err
		}
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		return listquery.Response[Shift]{}, err
	}

	return listquery.BuildResponse(data, params.Query, total), nil
}

func (r *Repository) FindByID(ctx context.Context, shiftID string) (*Shift, error) {
	const sql = `
		select id, place_id, name, start_time::text, end_time::text, is_active, created_at, updated_at
		from shifts
		where id = $1
		limit 1
	`
	var item Shift
	err := r.db.QueryRow(ctx, sql, shiftID).Scan(&item.ID, &item.PlaceID, &item.Name, &item.StartTime, &item.EndTime, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrShiftNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	const sql = `
		insert into shifts (place_id, name, start_time, end_time, is_active)
		values ($1, $2, $3::time, $4::time, $5)
		returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, input.PlaceID, strings.TrimSpace(input.Name), strings.TrimSpace(input.StartTime), strings.TrimSpace(input.EndTime), input.IsActive).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return "", ErrShiftAlreadyExists
		case isPgCode(err, "23503"):
			return "", ErrShiftPlaceNotFound
		default:
			return "", err
		}
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, shiftID string, input UpdateInput) (*Shift, error) {
	setParts := make([]string, 0, 5)
	args := make([]any, 0, 6)

	if input.PlaceID != nil {
		args = append(args, *input.PlaceID)
		setParts = append(setParts, fmt.Sprintf("place_id = $%d", len(args)))
	}
	if input.Name != nil {
		args = append(args, strings.TrimSpace(*input.Name))
		setParts = append(setParts, fmt.Sprintf("name = $%d", len(args)))
	}
	if input.StartTime != nil {
		args = append(args, strings.TrimSpace(*input.StartTime))
		setParts = append(setParts, fmt.Sprintf("start_time = $%d::time", len(args)))
	}
	if input.EndTime != nil {
		args = append(args, strings.TrimSpace(*input.EndTime))
		setParts = append(setParts, fmt.Sprintf("end_time = $%d::time", len(args)))
	}
	if input.IsActive != nil {
		args = append(args, *input.IsActive)
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrShiftNoFieldsToUpdate
	}

	setParts = append(setParts, "updated_at = now()")
	args = append(args, shiftID)

	sql := fmt.Sprintf(`
		update shifts
		set %s
		where id = $%d
		returning id, place_id, name, start_time::text, end_time::text, is_active, created_at, updated_at
	`, strings.Join(setParts, ", "), len(args))

	var item Shift
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.Name, &item.StartTime, &item.EndTime, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrShiftNotFound
		case isPgCode(err, "23505"):
			return nil, ErrShiftAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrShiftPlaceNotFound
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) Delete(ctx context.Context, shiftID string) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const detachSQL = `
		with target_assignments as (
			select id
			from spot_assignments
			where shift_id = $1
		),
		detached_attendances as (
			update attendances
			set assignment_id = null,
			    shift_id = null,
			    updated_at = now()
			where shift_id = $1
			   or assignment_id in (select id from target_assignments)
			returning id
		),
		detached_leave_requests as (
			update leave_requests
			set assignment_id = null,
			    updated_at = now()
			where assignment_id in (select id from target_assignments)
			returning id
		)
		delete from spot_assignments
		where shift_id = $1
	`
	if _, err := tx.Exec(ctx, detachSQL, shiftID); err != nil {
		return "", err
	}

	const deleteSQL = `delete from shifts where id = $1 returning id`
	var id string
	if err := tx.QueryRow(ctx, deleteSQL, shiftID).Scan(&id); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return "", ErrShiftNotFound
		case isPgCode(err, "23503"):
			return "", ErrShiftStillInUse
		default:
			return "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

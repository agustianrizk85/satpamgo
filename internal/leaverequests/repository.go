package leaverequests

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
	ErrNotFound         = errors.New("leave request not found")
	ErrForeignKey       = errors.New("related row not found")
	ErrNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct{ db *pgxpool.Pool }

type LeaveRequest struct {
	ID           string    `json:"id"`
	PlaceID      string    `json:"place_id"`
	UserID       string    `json:"user_id"`
	AssignmentID *string   `json:"assignment_id"`
	LeaveType    string    `json:"leave_type"`
	StartDate    string    `json:"start_date"`
	EndDate      *string   `json:"end_date"`
	Reason       *string   `json:"reason"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	UserID      string
	Status      string
	Query       listquery.Query
}

type CreateInput struct {
	PlaceID      string
	UserID       string
	AssignmentID *string
	LeaveType    string
	StartDate    string
	EndDate      *string
	Reason       *string
}

type UpdateStatusInput struct {
	ID      string
	PlaceID string
	Status  string
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[LeaveRequest], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"startDate": "start_date",
		"status":    "status",
		"userId":    "user_id",
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
		select id, place_id, user_id, assignment_id, leave_type, start_date::text, end_date::text, reason, status, created_at, updated_at, count(*) over()::int as total_count
		from leave_requests where true
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
	if params.Status != "" {
		args = append(args, params.Status)
		sql += fmt.Sprintf(" and status = $%d", len(args))
	}
	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[LeaveRequest]{}, err
	}
	defer rows.Close()
	data := make([]LeaveRequest, 0)
	total := 0
	for rows.Next() {
		var item LeaveRequest
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.AssignmentID, &item.LeaveType, &item.StartDate, &item.EndDate, &item.Reason, &item.Status, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[LeaveRequest]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	const sql = `insert into leave_requests (place_id, user_id, assignment_id, leave_type, start_date, end_date, reason) values ($1,$2,$3,$4,$5::date,$6::date,$7) returning id`
	var id string
	err := r.db.QueryRow(ctx, sql, input.PlaceID, input.UserID, input.AssignmentID, input.LeaveType, input.StartDate, input.EndDate, input.Reason).Scan(&id)
	if err != nil {
		if isPgCode(err, "23503") {
			return "", ErrForeignKey
		}
		return "", err
	}
	return id, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, input UpdateStatusInput) (string, error) {
	const sql = `update leave_requests set status = $1, updated_at = now() where id = $2 and place_id = $3 returning id`
	var id string
	err := r.db.QueryRow(ctx, sql, input.Status, input.ID, input.PlaceID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return id, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == code
}

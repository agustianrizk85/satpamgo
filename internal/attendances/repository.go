package attendances

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
	ErrNotFound         = errors.New("attendance not found")
	ErrAlreadyExists    = errors.New("attendance already exists")
	ErrForeignKey       = errors.New("related row not found")
	ErrNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct{ db *pgxpool.Pool }

type Attendance struct {
	ID               string     `json:"id"`
	PlaceID          string     `json:"place_id"`
	UserID           string     `json:"user_id"`
	AssignmentID     *string    `json:"assignment_id"`
	ShiftID          *string    `json:"shift_id"`
	AttendanceDate   string     `json:"attendance_date"`
	CheckInAt        *time.Time `json:"check_in_at"`
	CheckOutAt       *time.Time `json:"check_out_at"`
	SubmitAt         *time.Time `json:"submit_at"`
	PhotoURL         *string    `json:"photo_url"`
	CheckInPhotoURL  *string    `json:"check_in_photo_url"`
	CheckOutPhotoURL *string    `json:"check_out_photo_url"`
	Status           string     `json:"status"`
	Note             *string    `json:"note"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ListParams struct {
	ActorUserID    string
	ActorRole      string
	PlaceID        string
	UserID         string
	AttendanceDate string
	Query          listquery.Query
}

type CreateInput struct {
	PlaceID          string
	UserID           string
	AssignmentID     *string
	ShiftID          *string
	AttendanceDate   string
	CheckInAt        *string
	CheckOutAt       *string
	SubmitAt         *string
	PhotoURL         *string
	CheckInPhotoURL  *string
	CheckOutPhotoURL *string
	Status           string
	Note             *string
}

type UpdateInput struct {
	CheckInAt        **string
	CheckOutAt       **string
	SubmitAt         **string
	PhotoURL         **string
	CheckInPhotoURL  **string
	CheckOutPhotoURL **string
	Status           *string
	Note             **string
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[Attendance], error) {
	sortColumn := map[string]string{
		"attendanceDate": "attendance_date",
		"createdAt":      "created_at",
		"updatedAt":      "updated_at",
		"checkInAt":      "check_in_at",
		"checkOutAt":     "check_out_at",
		"submitAt":       "submit_at",
		"status":         "status",
		"userId":         "user_id",
		"placeId":        "place_id",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "attendance_date"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `
		select id, place_id, user_id, assignment_id, shift_id, attendance_date::text, check_in_at, check_out_at, submit_at, photo_url, check_in_photo_url, check_out_photo_url, status, note, created_at, updated_at, count(*) over()::int as total_count
		from attendances
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
	if params.AttendanceDate != "" {
		args = append(args, params.AttendanceDate)
		sql += fmt.Sprintf(" and attendance_date = $%d::date", len(args))
	}
	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Attendance]{}, err
	}
	defer rows.Close()
	data := make([]Attendance, 0)
	total := 0
	for rows.Next() {
		var item Attendance
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.AssignmentID, &item.ShiftID, &item.AttendanceDate, &item.CheckInAt, &item.CheckOutAt, &item.SubmitAt, &item.PhotoURL, &item.CheckInPhotoURL, &item.CheckOutPhotoURL, &item.Status, &item.Note, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Attendance]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	const sql = `
		insert into attendances (place_id, user_id, assignment_id, shift_id, attendance_date, check_in_at, check_out_at, submit_at, photo_url, check_in_photo_url, check_out_photo_url, status, note)
		values ($1,$2,$3,$4,$5::date,$6::timestamptz,$7::timestamptz,coalesce($8::timestamptz, now()),$9,$10,$11,$12,$13)
		returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, input.PlaceID, input.UserID, input.AssignmentID, input.ShiftID, input.AttendanceDate, input.CheckInAt, input.CheckOutAt, input.SubmitAt, input.PhotoURL, input.CheckInPhotoURL, input.CheckOutPhotoURL, input.Status, input.Note).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return "", ErrAlreadyExists
		case isPgCode(err, "23503"):
			return "", ErrForeignKey
		default:
			return "", err
		}
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput) (*Attendance, error) {
	setParts := make([]string, 0, 6)
	args := make([]any, 0, 7)
	addNullableTime := func(column string, value **string) {
		args = append(args, *value)
		setParts = append(setParts, fmt.Sprintf("%s = $%d::timestamptz", column, len(args)))
	}
	addNullableText := func(column string, value **string) {
		args = append(args, *value)
		setParts = append(setParts, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if input.CheckInAt != nil {
		addNullableTime("check_in_at", input.CheckInAt)
	}
	if input.CheckOutAt != nil {
		addNullableTime("check_out_at", input.CheckOutAt)
	}
	if input.SubmitAt != nil {
		addNullableTime("submit_at", input.SubmitAt)
	}
	if input.PhotoURL != nil {
		addNullableText("photo_url", input.PhotoURL)
	}
	if input.CheckInPhotoURL != nil {
		addNullableText("check_in_photo_url", input.CheckInPhotoURL)
	}
	if input.CheckOutPhotoURL != nil {
		addNullableText("check_out_photo_url", input.CheckOutPhotoURL)
	}
	if input.Status != nil {
		args = append(args, *input.Status)
		setParts = append(setParts, fmt.Sprintf("status = $%d", len(args)))
	}
	if input.Note != nil {
		addNullableText("note", input.Note)
	}
	if len(setParts) == 0 {
		return nil, ErrNoFieldsToUpdate
	}
	setParts = append(setParts, "updated_at = now()")
	args = append(args, id)
	sql := fmt.Sprintf(`
		update attendances set %s
		where id = $%d
		returning id, place_id, user_id, assignment_id, shift_id, attendance_date::text, check_in_at, check_out_at, submit_at, photo_url, check_in_photo_url, check_out_photo_url, status, note, created_at, updated_at
	`, strings.Join(setParts, ", "), len(args))
	var item Attendance
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.AssignmentID, &item.ShiftID, &item.AttendanceDate, &item.CheckInAt, &item.CheckOutAt, &item.SubmitAt, &item.PhotoURL, &item.CheckInPhotoURL, &item.CheckOutPhotoURL, &item.Status, &item.Note, &item.CreatedAt, &item.UpdatedAt)
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
	const sql = `delete from attendances where id = $1 returning id`
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

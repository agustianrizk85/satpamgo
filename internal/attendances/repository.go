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
	LateMinutes      *int       `json:"late_minutes"`
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
		"attendanceDate": "a.attendance_date",
		"createdAt":      "a.created_at",
		"updatedAt":      "a.updated_at",
		"checkInAt":      "a.check_in_at",
		"checkOutAt":     "a.check_out_at",
		"submitAt":       "a.submit_at",
		"status":         "a.status",
		"userId":         "a.user_id",
		"placeId":        "a.place_id",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "a.attendance_date"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `
		select
			a.id, a.place_id, a.user_id, a.assignment_id, a.shift_id, a.attendance_date::text,
			a.check_in_at, a.check_out_at, a.submit_at, a.photo_url, a.check_in_photo_url, a.check_out_photo_url,
			a.status,
			case
				when a.check_in_at is null or s.start_time is null then null
				else greatest(
					floor(extract(epoch from ((a.check_in_at at time zone 'Asia/Jakarta') - (a.attendance_date::timestamp + s.start_time))) / 60),
					0
				)::int
			end as late_minutes,
			a.note, a.created_at, a.updated_at, count(*) over()::int as total_count
		from attendances a
		left join shifts s on s.id = a.shift_id
		where true
	`
	args := []any{}
	if params.PlaceID != "" {
		args = append(args, params.PlaceID)
		sql += fmt.Sprintf(" and a.place_id = $%d", len(args))
	} else if !auth.IsGlobalAdminRole(params.ActorRole) {
		args = append(args, params.ActorUserID)
		sql += fmt.Sprintf(` and a.place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	if params.UserID != "" {
		args = append(args, params.UserID)
		sql += fmt.Sprintf(" and a.user_id = $%d", len(args))
	}
	if params.AttendanceDate != "" {
		args = append(args, params.AttendanceDate)
		sql += fmt.Sprintf(" and a.attendance_date = $%d::date", len(args))
	}
	args = append(args, params.Query.PageSize, params.Query.Offset)
	sql += fmt.Sprintf(" order by %s %s nulls last, a.created_at desc, a.id desc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Attendance]{}, err
	}
	defer rows.Close()
	data := make([]Attendance, 0)
	total := 0
	for rows.Next() {
		var item Attendance
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.AssignmentID, &item.ShiftID, &item.AttendanceDate, &item.CheckInAt, &item.CheckOutAt, &item.SubmitAt, &item.PhotoURL, &item.CheckInPhotoURL, &item.CheckOutPhotoURL, &item.Status, &item.LateMinutes, &item.Note, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Attendance]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	if err := r.applyAutomaticCreateDefaults(ctx, &input); err != nil {
		return "", err
	}
	if err := r.deriveLateStatusOnCreate(ctx, &input); err != nil {
		return "", err
	}

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

func (r *Repository) applyAutomaticCreateDefaults(ctx context.Context, input *CreateInput) error {
	if err := r.autoFillCheckInForPlaceAdmin(ctx, input); err != nil {
		return err
	}
	if err := r.autoFillShift(ctx, input); err != nil {
		return err
	}
	if err := r.hydrateAssignmentAndShift(ctx, input); err != nil {
		return err
	}
	return nil
}

func (r *Repository) deriveLateStatusOnCreate(ctx context.Context, input *CreateInput) error {
	if input.CheckInAt == nil || input.ShiftID == nil {
		return nil
	}
	if input.Status != "PRESENT" && input.Status != "LATE" {
		return nil
	}

	late, err := r.computeLateMinutes(ctx, input.AttendanceDate, *input.ShiftID, *input.CheckInAt)
	if err != nil {
		return err
	}
	if late > 0 {
		input.Status = "LATE"
		return nil
	}
	input.Status = "PRESENT"
	return nil
}

func (r *Repository) autoFillCheckInForPlaceAdmin(ctx context.Context, input *CreateInput) error {
	if input.CheckInAt != nil || input.CheckOutAt != nil {
		return nil
	}

	isPlaceAdmin, err := r.hasPlaceRole(ctx, input.UserID, input.PlaceID, auth.PlaceRoleAdmin)
	if err != nil {
		return err
	}
	if !isPlaceAdmin {
		return nil
	}

	nowJakarta := time.Now().In(time.FixedZone("Asia/Jakarta", 7*60*60))
	value := nowJakarta.Format(time.RFC3339)
	input.CheckInAt = &value
	if input.SubmitAt == nil {
		input.SubmitAt = &value
	}
	return nil
}

func (r *Repository) autoFillShift(ctx context.Context, input *CreateInput) error {
	if input.CheckInAt == nil && input.SubmitAt == nil && input.ShiftID != nil {
		return nil
	}

	referenceAt, err := resolveAttendanceReferenceTime(input)
	if err != nil {
		return err
	}

	shiftID, err := r.findBestShiftForAttendance(ctx, input.PlaceID, referenceAt)
	if err != nil {
		return err
	}
	if shiftID != nil {
		input.ShiftID = shiftID
	}
	return nil
}

func (r *Repository) hydrateAssignmentAndShift(ctx context.Context, input *CreateInput) error {
	if input.AssignmentID != nil && input.ShiftID != nil {
		return nil
	}

	if input.AssignmentID != nil && input.ShiftID == nil {
		const sql = `select shift_id from spot_assignments where id = $1 limit 1`
		var shiftID string
		err := r.db.QueryRow(ctx, sql, *input.AssignmentID).Scan(&shiftID)
		switch {
		case err == nil:
			input.ShiftID = &shiftID
			return nil
		case errors.Is(err, pgx.ErrNoRows):
			return ErrForeignKey
		default:
			return err
		}
	}

	const sql = `
		select id, shift_id
		from spot_assignments
		where place_id = $1
		  and user_id = $2
		  and is_active = true
		  and ($3::uuid is null or shift_id = $3::uuid)
		order by updated_at desc, created_at desc, id desc
		limit 1
	`
	var assignmentID, shiftID string
	err := r.db.QueryRow(ctx, sql, input.PlaceID, input.UserID, input.ShiftID).Scan(&assignmentID, &shiftID)
	switch {
	case err == nil:
		if input.AssignmentID == nil {
			input.AssignmentID = &assignmentID
		}
		if input.ShiftID == nil {
			input.ShiftID = &shiftID
		}
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	default:
		return err
	}
}

func (r *Repository) hasPlaceRole(ctx context.Context, userID, placeID, roleCode string) (bool, error) {
	const sql = `
		select 1
		from user_place_roles upr
		join roles r on r.id = upr.role_id
		where upr.user_id = $1
		  and upr.place_id = $2
		  and upr.is_active = true
		  and r.code = $3
		limit 1
	`
	var ok int
	err := r.db.QueryRow(ctx, sql, userID, placeID, roleCode).Scan(&ok)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	default:
		return false, err
	}
}

func resolveAttendanceReferenceTime(input *CreateInput) (time.Time, error) {
	if input.CheckInAt != nil {
		return time.Parse(time.RFC3339, *input.CheckInAt)
	}
	if input.SubmitAt != nil {
		return time.Parse(time.RFC3339, *input.SubmitAt)
	}
	return time.Now(), nil
}

func (r *Repository) findBestShiftForAttendance(ctx context.Context, placeID string, referenceAt time.Time) (*string, error) {
	const sql = `
		with ref as (
			select
				extract(epoch from (($2::timestamptz at time zone 'Asia/Jakarta')::time))::int as local_sec,
				7200 as shift_grace_sec
		),
		candidate as (
			select
				s.id,
				s.start_time,
				((extract(epoch from s.end_time)::int - extract(epoch from s.start_time)::int + 86400) % 86400) as duration_sec,
				((ref.local_sec - extract(epoch from s.start_time)::int + 86400) % 86400) as since_start_sec,
				((extract(epoch from s.start_time)::int - ref.local_sec + 86400) % 86400) as until_start_sec,
				((ref.local_sec - extract(epoch from s.end_time)::int + 86400) % 86400) as since_end_sec,
				ref.shift_grace_sec
			from shifts s
			cross join ref
			where s.place_id = $1
			  and s.is_active = true
		)
		select id
		from candidate
		order by
			case
				when until_start_sec > 0 and until_start_sec <= shift_grace_sec then 0
				when since_start_sec < duration_sec then 1
				when since_end_sec <= shift_grace_sec then 2
				else 3
			end asc,
			case
				when until_start_sec > 0 and until_start_sec <= shift_grace_sec then until_start_sec
				when since_start_sec < duration_sec then since_start_sec
				when since_end_sec <= shift_grace_sec then since_end_sec
				else until_start_sec
			end asc,
			start_time asc,
			id asc
		limit 1
	`
	var shiftID string
	err := r.db.QueryRow(ctx, sql, placeID, referenceAt.Format(time.RFC3339)).Scan(&shiftID)
	switch {
	case err == nil:
		return &shiftID, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	default:
		return nil, err
	}
}

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput) (*Attendance, error) {
	input, err := r.deriveLateStatusOnUpdate(ctx, id, input)
	if err != nil {
		return nil, err
	}

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
	err = r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.AssignmentID, &item.ShiftID, &item.AttendanceDate, &item.CheckInAt, &item.CheckOutAt, &item.SubmitAt, &item.PhotoURL, &item.CheckInPhotoURL, &item.CheckOutPhotoURL, &item.Status, &item.Note, &item.CreatedAt, &item.UpdatedAt)
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

func (r *Repository) deriveLateStatusOnUpdate(ctx context.Context, id string, input UpdateInput) (UpdateInput, error) {
	if input.CheckInAt == nil && input.Status == nil {
		return input, nil
	}

	const sql = `select attendance_date::text, shift_id, status from attendances where id = $1 limit 1`
	var attendanceDate string
	var shiftID *string
	var currentStatus string
	err := r.db.QueryRow(ctx, sql, id).Scan(&attendanceDate, &shiftID, &currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return input, ErrNotFound
		}
		return input, err
	}

	if shiftID == nil {
		return input, nil
	}

	if input.CheckInAt == nil {
		return input, nil
	}
	if *input.CheckInAt == nil {
		return input, nil
	}

	effectiveStatus := currentStatus
	if input.Status != nil {
		effectiveStatus = *input.Status
	}
	if effectiveStatus != "PRESENT" && effectiveStatus != "LATE" {
		return input, nil
	}

	late, err := r.computeLateMinutes(ctx, attendanceDate, *shiftID, **input.CheckInAt)
	if err != nil {
		return input, err
	}
	status := "PRESENT"
	if late > 0 {
		status = "LATE"
	}
	input.Status = &status
	return input, nil
}

func (r *Repository) computeLateMinutes(ctx context.Context, attendanceDate, shiftID, checkInAt string) (int, error) {
	const sql = `
		select case
			when s.start_time is null then 0
			else greatest(
				floor(extract(epoch from (($3::timestamptz at time zone 'Asia/Jakarta') - ($1::date::timestamp + s.start_time))) / 60),
				0
			)::int
		end
		from shifts s
		where s.id = $2
		limit 1
	`
	var late int
	err := r.db.QueryRow(ctx, sql, attendanceDate, shiftID, checkInAt).Scan(&late)
	switch {
	case err == nil:
		return late, nil
	case errors.Is(err, pgx.ErrNoRows):
		return 0, ErrForeignKey
	default:
		return 0, err
	}
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

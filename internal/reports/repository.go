package reports

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
)

const defaultAttendanceTimezone = "Asia/Jakarta"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type AttendanceStatus string
type FacilityStatus string

const (
	AttendancePresent AttendanceStatus = "PRESENT"
	AttendanceLate    AttendanceStatus = "LATE"
	AttendanceAbsent  AttendanceStatus = "ABSENT"
	AttendanceOff     AttendanceStatus = "OFF"
	AttendanceSick    AttendanceStatus = "SICK"
	AttendanceLeave   AttendanceStatus = "LEAVE"

	FacilityOK      FacilityStatus = "OK"
	FacilityNotOK   FacilityStatus = "NOT_OK"
	FacilityPartial FacilityStatus = "PARTIAL"
)

type AttendanceReportRow struct {
	ID               string  `json:"id"`
	PlaceID          string  `json:"place_id"`
	PlaceName        string  `json:"place_name"`
	UserID           string  `json:"user_id"`
	FullName         string  `json:"full_name"`
	AssignmentID     *string `json:"assignment_id"`
	ShiftID          *string `json:"shift_id"`
	ShiftName        *string `json:"shift_name"`
	AttendanceDate   string  `json:"attendance_date"`
	CheckInAt        *string `json:"check_in_at"`
	CheckOutAt       *string `json:"check_out_at"`
	Status           string  `json:"status"`
	LateMinutes      *int    `json:"late_minutes"`
	Note             *string `json:"note"`
	CheckInPhotoURL  *string `json:"check_in_photo_url"`
	CheckOutPhotoURL *string `json:"check_out_photo_url"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type AttendanceReportSummary struct {
	TotalData    int `json:"total_data"`
	PresentCount int `json:"present_count"`
	LateCount    int `json:"late_count"`
	AbsentCount  int `json:"absent_count"`
	OffCount     int `json:"off_count"`
	SickCount    int `json:"sick_count"`
	LeaveCount   int `json:"leave_count"`
}

type PatrolScanReportRow struct {
	ID          string  `json:"id"`
	PlaceID     string  `json:"place_id"`
	PlaceName   string  `json:"place_name"`
	UserID      string  `json:"user_id"`
	FullName    string  `json:"full_name"`
	SpotID      string  `json:"spot_id"`
	SpotCode    string  `json:"spot_code"`
	SpotName    string  `json:"spot_name"`
	PatrolRunID string  `json:"patrol_run_id"`
	ScannedAt   string  `json:"scanned_at"`
	PhotoURL    *string `json:"photo_url"`
	Note        *string `json:"note"`
}

type PatrolScanReportSummary struct {
	TotalData        int `json:"total_data"`
	UniquePatrolRuns int `json:"unique_patrol_runs"`
	UniqueSpots      int `json:"unique_spots"`
	UniqueUsers      int `json:"unique_users"`
}

type FacilityScanReportRow struct {
	ID        string  `json:"id"`
	PlaceID   string  `json:"place_id"`
	PlaceName string  `json:"place_name"`
	SpotID    string  `json:"spot_id"`
	SpotCode  string  `json:"spot_code"`
	SpotName  string  `json:"spot_name"`
	ItemID    *string `json:"item_id"`
	ItemName  *string `json:"item_name"`
	UserID    string  `json:"user_id"`
	FullName  string  `json:"full_name"`
	ScannedAt string  `json:"scanned_at"`
	Status    string  `json:"status"`
	Note      *string `json:"note"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type FacilityScanReportSummary struct {
	TotalData    int `json:"total_data"`
	OKCount      int `json:"ok_count"`
	NotOKCount   int `json:"not_ok_count"`
	PartialCount int `json:"partial_count"`
	UniqueSpots  int `json:"unique_spots"`
	UniqueItems  int `json:"unique_items"`
	UniqueUsers  int `json:"unique_users"`
}

type AttendanceFilters struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	UserID      string
	Status      string
	FromDate    string
	ToDate      string
}

type PatrolScanFilters struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	UserID      string
	SpotID      string
	PatrolRunID string
	FromDate    string
	ToDate      string
}

type FacilityScanFilters struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	UserID      string
	SpotID      string
	ItemID      string
	Status      string
	FromDate    string
	ToDate      string
}

func (r *Repository) ListAttendance(ctx context.Context, filters AttendanceFilters, query listquery.Query) (listquery.Response[AttendanceReportRow], AttendanceReportSummary, error) {
	rows, total, err := r.queryAttendance(ctx, filters, query, true)
	if err != nil {
		return listquery.Response[AttendanceReportRow]{}, AttendanceReportSummary{}, err
	}
	summary, err := r.attendanceSummary(ctx, filters)
	if err != nil {
		return listquery.Response[AttendanceReportRow]{}, AttendanceReportSummary{}, err
	}
	return listquery.BuildResponse(rows, query, total), summary, nil
}

func (r *Repository) DownloadAttendance(ctx context.Context, filters AttendanceFilters, sortBy string, sortOrder listquery.SortOrder) ([]AttendanceReportRow, AttendanceReportSummary, error) {
	query := listquery.Query{Page: 1, PageSize: 100000, SortBy: sortBy, SortOrder: sortOrder}
	rows, _, err := r.queryAttendance(ctx, filters, query, false)
	if err != nil {
		return nil, AttendanceReportSummary{}, err
	}
	summary, err := r.attendanceSummary(ctx, filters)
	if err != nil {
		return nil, AttendanceReportSummary{}, err
	}
	return rows, summary, nil
}

func (r *Repository) queryAttendance(ctx context.Context, filters AttendanceFilters, query listquery.Query, paged bool) ([]AttendanceReportRow, int, error) {
	sortColumn := map[string]string{
		"attendanceDate": "a.attendance_date",
		"checkInAt":      "a.check_in_at",
		"checkOutAt":     "a.check_out_at",
		"status":         "a.status",
		"lateMinutes":    "late_minutes",
		"userName":       "u.full_name",
		"placeName":      "p.place_name",
		"createdAt":      "a.created_at",
	}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "a.attendance_date"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	whereSQL, args := buildAttendanceWhere(filters)
	limitOffset := ""
	if paged {
		args = append(args, defaultAttendanceTimezone, query.PageSize, query.Offset)
		limitOffset = fmt.Sprintf(" limit $%d offset $%d", len(args)-1, len(args))
	} else {
		args = append(args, defaultAttendanceTimezone)
	}
	tzArg := fmt.Sprintf("$%d", len(args)-2)
	if !paged {
		tzArg = fmt.Sprintf("$%d", len(args))
	}

	sql := fmt.Sprintf(`
		select
			a.id, a.place_id, p.place_name, a.user_id, u.full_name, a.assignment_id, a.shift_id, s.name as shift_name,
			a.attendance_date::text,
			case when a.check_in_at is null then null else to_char(a.check_in_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS') end,
			case when a.check_out_at is null then null else to_char(a.check_out_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS') end,
			a.status,
			case
				when a.check_in_at is null or s.start_time is null then null
				else greatest(
					floor(extract(epoch from ((a.check_in_at at time zone %s) - (a.attendance_date::timestamp + s.start_time))) / 60),
					0
				)::int
			end as late_minutes,
			a.note,
			coalesce(a.check_in_photo_url, a.photo_url) as check_in_photo_url,
			a.check_out_photo_url,
			to_char(a.created_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS'),
			to_char(a.updated_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS'),
			count(*) over()::int as total_count
		from attendances a
		join users u on u.id = a.user_id
		join places p on p.id = a.place_id
		left join shifts s on s.id = a.shift_id
		%s
		order by %s %s, a.id asc
		%s
	`, tzArg, tzArg, tzArg, tzArg, tzArg, whereSQL, sortColumn, sortDirection, limitOffset)

	rowsDB, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rowsDB.Close()

	data := make([]AttendanceReportRow, 0)
	total := 0
	for rowsDB.Next() {
		var item AttendanceReportRow
		if err := rowsDB.Scan(
			&item.ID, &item.PlaceID, &item.PlaceName, &item.UserID, &item.FullName, &item.AssignmentID, &item.ShiftID, &item.ShiftName,
			&item.AttendanceDate, &item.CheckInAt, &item.CheckOutAt, &item.Status, &item.LateMinutes, &item.Note,
			&item.CheckInPhotoURL, &item.CheckOutPhotoURL, &item.CreatedAt, &item.UpdatedAt, &total,
		); err != nil {
			return nil, 0, err
		}
		data = append(data, item)
	}
	return data, total, rowsDB.Err()
}

func (r *Repository) attendanceSummary(ctx context.Context, filters AttendanceFilters) (AttendanceReportSummary, error) {
	whereSQL, args := buildAttendanceWhere(filters)
	const sqlBody = `
		select
			count(*)::int as total_data,
			count(*) filter (where a.status = 'PRESENT')::int as present_count,
			count(*) filter (where a.status = 'LATE')::int as late_count,
			count(*) filter (where a.status = 'ABSENT')::int as absent_count,
			count(*) filter (where a.status = 'OFF')::int as off_count,
			count(*) filter (where a.status = 'SICK')::int as sick_count,
			count(*) filter (where a.status = 'LEAVE')::int as leave_count
		from attendances a
	`
	var out AttendanceReportSummary
	err := r.db.QueryRow(ctx, sqlBody+whereSQL, args...).Scan(
		&out.TotalData, &out.PresentCount, &out.LateCount, &out.AbsentCount, &out.OffCount, &out.SickCount, &out.LeaveCount,
	)
	return out, err
}

func buildAttendanceWhere(filters AttendanceFilters) (string, []any) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 8)
	applyPlaceScope(&args, &where, filters.ActorRole, filters.ActorUserID, filters.PlaceID, "a.place_id")
	if filters.UserID != "" {
		args = append(args, filters.UserID)
		where = append(where, fmt.Sprintf("a.user_id = $%d", len(args)))
	}
	if filters.Status != "" {
		args = append(args, filters.Status)
		where = append(where, fmt.Sprintf("a.status = $%d", len(args)))
	}
	if filters.FromDate != "" {
		args = append(args, filters.FromDate)
		where = append(where, fmt.Sprintf("a.attendance_date >= $%d::date", len(args)))
	}
	if filters.ToDate != "" {
		args = append(args, filters.ToDate)
		where = append(where, fmt.Sprintf("a.attendance_date <= $%d::date", len(args)))
	}
	return buildWhereSQL(where), args
}

func (r *Repository) ListPatrolScans(ctx context.Context, filters PatrolScanFilters, query listquery.Query) (listquery.Response[PatrolScanReportRow], PatrolScanReportSummary, error) {
	rows, total, err := r.queryPatrolScans(ctx, filters, query, true)
	if err != nil {
		return listquery.Response[PatrolScanReportRow]{}, PatrolScanReportSummary{}, err
	}
	summary, err := r.patrolScanSummary(ctx, filters)
	if err != nil {
		return listquery.Response[PatrolScanReportRow]{}, PatrolScanReportSummary{}, err
	}
	return listquery.BuildResponse(rows, query, total), summary, nil
}

func (r *Repository) DownloadPatrolScans(ctx context.Context, filters PatrolScanFilters, sortBy string, sortOrder listquery.SortOrder) ([]PatrolScanReportRow, PatrolScanReportSummary, error) {
	query := listquery.Query{Page: 1, PageSize: 100000, SortBy: sortBy, SortOrder: sortOrder}
	rows, _, err := r.queryPatrolScans(ctx, filters, query, false)
	if err != nil {
		return nil, PatrolScanReportSummary{}, err
	}
	summary, err := r.patrolScanSummary(ctx, filters)
	if err != nil {
		return nil, PatrolScanReportSummary{}, err
	}
	return rows, summary, nil
}

func (r *Repository) queryPatrolScans(ctx context.Context, filters PatrolScanFilters, query listquery.Query, paged bool) ([]PatrolScanReportRow, int, error) {
	sortColumn := map[string]string{
		"scannedAt":   "ps.scanned_at",
		"patrolRunId": "ps.patrol_run_id",
		"userName":    "u.full_name",
		"placeName":   "p.place_name",
		"spotName":    "s.spot_name",
	}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "ps.scanned_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	whereSQL, args := buildPatrolScanWhere(filters)
	limitOffset := ""
	if paged {
		args = append(args, defaultAttendanceTimezone, query.PageSize, query.Offset)
		limitOffset = fmt.Sprintf(" limit $%d offset $%d", len(args)-1, len(args))
	} else {
		args = append(args, defaultAttendanceTimezone)
	}
	tzArg := fmt.Sprintf("$%d", len(args)-2)
	if !paged {
		tzArg = fmt.Sprintf("$%d", len(args))
	}
	sql := fmt.Sprintf(`
		select
			ps.id, ps.place_id, p.place_name, ps.user_id, u.full_name, ps.spot_id, s.spot_code, s.spot_name,
			ps.patrol_run_id, to_char(ps.scanned_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS'), ps.photo_url, ps.note, count(*) over()::int as total_count
		from patrol_scans ps
		join users u on u.id = ps.user_id
		join places p on p.id = ps.place_id
		join spots s on s.id = ps.spot_id
		%s
		order by %s %s, ps.id asc
		%s
	`, tzArg, whereSQL, sortColumn, sortDirection, limitOffset)
	rowsDB, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rowsDB.Close()
	data := make([]PatrolScanReportRow, 0)
	total := 0
	for rowsDB.Next() {
		var item PatrolScanReportRow
		if err := rowsDB.Scan(&item.ID, &item.PlaceID, &item.PlaceName, &item.UserID, &item.FullName, &item.SpotID, &item.SpotCode, &item.SpotName, &item.PatrolRunID, &item.ScannedAt, &item.PhotoURL, &item.Note, &total); err != nil {
			return nil, 0, err
		}
		data = append(data, item)
	}
	return data, total, rowsDB.Err()
}

func (r *Repository) patrolScanSummary(ctx context.Context, filters PatrolScanFilters) (PatrolScanReportSummary, error) {
	whereSQL, args := buildPatrolScanWhere(filters)
	sql := `
		select
			count(*)::int as total_data,
			count(distinct ps.patrol_run_id)::int as unique_patrol_runs,
			count(distinct ps.spot_id)::int as unique_spots,
			count(distinct ps.user_id)::int as unique_users
		from patrol_scans ps
	` + whereSQL
	var out PatrolScanReportSummary
	err := r.db.QueryRow(ctx, sql, args...).Scan(&out.TotalData, &out.UniquePatrolRuns, &out.UniqueSpots, &out.UniqueUsers)
	return out, err
}

func buildPatrolScanWhere(filters PatrolScanFilters) (string, []any) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 8)
	applyPlaceScope(&args, &where, filters.ActorRole, filters.ActorUserID, filters.PlaceID, "ps.place_id")
	if filters.UserID != "" {
		args = append(args, filters.UserID)
		where = append(where, fmt.Sprintf("ps.user_id = $%d", len(args)))
	}
	if filters.SpotID != "" {
		args = append(args, filters.SpotID)
		where = append(where, fmt.Sprintf("ps.spot_id = $%d", len(args)))
	}
	if filters.PatrolRunID != "" {
		args = append(args, filters.PatrolRunID)
		where = append(where, fmt.Sprintf("ps.patrol_run_id = $%d", len(args)))
	}
	if filters.FromDate != "" {
		args = append(args, filters.FromDate)
		where = append(where, fmt.Sprintf("ps.scanned_at >= $%d::date::timestamptz", len(args)))
	}
	if filters.ToDate != "" {
		args = append(args, filters.ToDate)
		where = append(where, fmt.Sprintf("ps.scanned_at < ($%d::date + interval '1 day')::timestamptz", len(args)))
	}
	return buildWhereSQL(where), args
}

func (r *Repository) ListFacilityScans(ctx context.Context, filters FacilityScanFilters, query listquery.Query) (listquery.Response[FacilityScanReportRow], FacilityScanReportSummary, error) {
	rows, total, err := r.queryFacilityScans(ctx, filters, query, true)
	if err != nil {
		return listquery.Response[FacilityScanReportRow]{}, FacilityScanReportSummary{}, err
	}
	summary, err := r.facilityScanSummary(ctx, filters)
	if err != nil {
		return listquery.Response[FacilityScanReportRow]{}, FacilityScanReportSummary{}, err
	}
	return listquery.BuildResponse(rows, query, total), summary, nil
}

func (r *Repository) DownloadFacilityScans(ctx context.Context, filters FacilityScanFilters, sortBy string, sortOrder listquery.SortOrder) ([]FacilityScanReportRow, FacilityScanReportSummary, error) {
	query := listquery.Query{Page: 1, PageSize: 100000, SortBy: sortBy, SortOrder: sortOrder}
	rows, _, err := r.queryFacilityScans(ctx, filters, query, false)
	if err != nil {
		return nil, FacilityScanReportSummary{}, err
	}
	summary, err := r.facilityScanSummary(ctx, filters)
	if err != nil {
		return nil, FacilityScanReportSummary{}, err
	}
	return rows, summary, nil
}

func (r *Repository) queryFacilityScans(ctx context.Context, filters FacilityScanFilters, query listquery.Query, paged bool) ([]FacilityScanReportRow, int, error) {
	sortColumn := map[string]string{
		"scannedAt": "fs.scanned_at",
		"status":    "fs.status",
		"userName":  "u.full_name",
		"placeName": "p.place_name",
		"spotName":  "sp.spot_name",
		"itemName":  "i.item_name",
		"createdAt": "fs.created_at",
	}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "fs.scanned_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	whereSQL, args := buildFacilityScanWhere(filters)
	limitOffset := ""
	if paged {
		args = append(args, defaultAttendanceTimezone, query.PageSize, query.Offset)
		limitOffset = fmt.Sprintf(" limit $%d offset $%d", len(args)-1, len(args))
	} else {
		args = append(args, defaultAttendanceTimezone)
	}
	tzArg := fmt.Sprintf("$%d", len(args)-2)
	if !paged {
		tzArg = fmt.Sprintf("$%d", len(args))
	}
	sql := fmt.Sprintf(`
		select
			fs.id, fs.place_id, p.place_name, fs.spot_id, sp.spot_code, sp.spot_name, fs.item_id, i.item_name, fs.user_id, u.full_name,
			to_char(fs.scanned_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS'),
			fs.status,
			fs.note,
			to_char(fs.created_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS'),
			to_char(fs.updated_at at time zone %s, 'YYYY-MM-DD HH24:MI:SS'),
			count(*) over()::int as total_count
		from facility_check_scans fs
		join users u on u.id = fs.user_id
		join places p on p.id = fs.place_id
		join facility_check_spots sp on sp.id = fs.spot_id
		left join facility_check_items i on i.id = fs.item_id
		%s
		order by %s %s, fs.id asc
		%s
	`, tzArg, tzArg, tzArg, whereSQL, sortColumn, sortDirection, limitOffset)
	rowsDB, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rowsDB.Close()
	data := make([]FacilityScanReportRow, 0)
	total := 0
	for rowsDB.Next() {
		var item FacilityScanReportRow
		if err := rowsDB.Scan(&item.ID, &item.PlaceID, &item.PlaceName, &item.SpotID, &item.SpotCode, &item.SpotName, &item.ItemID, &item.ItemName, &item.UserID, &item.FullName, &item.ScannedAt, &item.Status, &item.Note, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return nil, 0, err
		}
		data = append(data, item)
	}
	return data, total, rowsDB.Err()
}

func (r *Repository) facilityScanSummary(ctx context.Context, filters FacilityScanFilters) (FacilityScanReportSummary, error) {
	whereSQL, args := buildFacilityScanWhere(filters)
	sql := `
		select
			count(*)::int as total_data,
			count(*) filter (where fs.status = 'OK')::int as ok_count,
			count(*) filter (where fs.status = 'NOT_OK')::int as not_ok_count,
			count(*) filter (where fs.status = 'PARTIAL')::int as partial_count,
			count(distinct fs.spot_id)::int as unique_spots,
			count(distinct fs.item_id)::int as unique_items,
			count(distinct fs.user_id)::int as unique_users
		from facility_check_scans fs
	` + whereSQL
	var out FacilityScanReportSummary
	err := r.db.QueryRow(ctx, sql, args...).Scan(&out.TotalData, &out.OKCount, &out.NotOKCount, &out.PartialCount, &out.UniqueSpots, &out.UniqueItems, &out.UniqueUsers)
	return out, err
}

func buildFacilityScanWhere(filters FacilityScanFilters) (string, []any) {
	args := make([]any, 0, 8)
	where := make([]string, 0, 8)
	applyPlaceScope(&args, &where, filters.ActorRole, filters.ActorUserID, filters.PlaceID, "fs.place_id")
	if filters.UserID != "" {
		args = append(args, filters.UserID)
		where = append(where, fmt.Sprintf("fs.user_id = $%d", len(args)))
	}
	if filters.SpotID != "" {
		args = append(args, filters.SpotID)
		where = append(where, fmt.Sprintf("fs.spot_id = $%d", len(args)))
	}
	if filters.ItemID != "" {
		args = append(args, filters.ItemID)
		where = append(where, fmt.Sprintf("fs.item_id = $%d", len(args)))
	}
	if filters.Status != "" {
		args = append(args, filters.Status)
		where = append(where, fmt.Sprintf("fs.status = $%d", len(args)))
	}
	if filters.FromDate != "" {
		args = append(args, filters.FromDate)
		where = append(where, fmt.Sprintf("fs.scanned_at >= $%d::date::timestamptz", len(args)))
	}
	if filters.ToDate != "" {
		args = append(args, filters.ToDate)
		where = append(where, fmt.Sprintf("fs.scanned_at < ($%d::date + interval '1 day')::timestamptz", len(args)))
	}
	return buildWhereSQL(where), args
}

func buildWhereSQL(where []string) string {
	if len(where) == 0 {
		return ""
	}
	return " where " + strings.Join(where, " and ")
}

func applyPlaceScope(args *[]any, where *[]string, actorRole, actorUserID, explicitPlaceID, column string) {
	if explicitPlaceID != "" {
		*args = append(*args, explicitPlaceID)
		*where = append(*where, fmt.Sprintf("%s = $%d", column, len(*args)))
		return
	}
	if auth.IsGlobalAdminRole(actorRole) {
		return
	}
	*args = append(*args, actorUserID)
	*where = append(*where, fmt.Sprintf(`%s in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, column, len(*args)))
}

package recentactivities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
)

const defaultActivityTimezone = "Asia/Jakarta"

type Repository struct {
	db *pgxpool.Pool
}

type Activity struct {
	ActivityID   string         `json:"activity_id"`
	ActivityType string         `json:"activity_type"`
	ActivityAt   string         `json:"activity_at"`
	PlaceID      string         `json:"place_id"`
	PlaceName    *string        `json:"place_name"`
	UserID       string         `json:"user_id"`
	UserName     *string        `json:"user_name"`
	SourceID     string         `json:"source_id"`
	Metadata     map[string]any `json:"metadata"`
}

type Summary struct {
	TotalToday              int `json:"total_today"`
	TotalMonth              int `json:"total_month"`
	TotalYear               int `json:"total_year"`
	FacilityActive          int `json:"facility_active"`
	SpotActive              int `json:"spot_active"`
	PointActive             int `json:"point_active"`
	PatrolSpotToday         int `json:"patrol_spot_today"`
	PatrolSpotMonth         int `json:"patrol_spot_month"`
	PatrolSpotYear          int `json:"patrol_spot_year"`
	PatrolFacilityToday     int `json:"patrol_facility_today"`
	PatrolFacilityMonth     int `json:"patrol_facility_month"`
	PatrolFacilityYear      int `json:"patrol_facility_year"`
	AttendanceCheckInToday  int `json:"attendance_check_in_today"`
	AttendanceCheckInMonth  int `json:"attendance_check_in_month"`
	AttendanceCheckInYear   int `json:"attendance_check_in_year"`
	AttendanceCheckOutToday int `json:"attendance_check_out_today"`
	AttendanceCheckOutMonth int `json:"attendance_check_out_month"`
	AttendanceCheckOutYear  int `json:"attendance_check_out_year"`
}

type Response struct {
	Data       []Activity `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		TotalData  int `json:"totalData"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
	Sort struct {
		SortBy    string              `json:"sortBy"`
		SortOrder listquery.SortOrder `json:"sortOrder"`
	} `json:"sort"`
	Summary Summary `json:"summary"`
}

type ListParams struct {
	ActorUserID  string
	ActorRole    string
	PlaceID      string
	UserID       string
	ActivityType string
	FromDate     string
	ToDate       string
	Query        listquery.Query
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, params ListParams) (Response, error) {
	sortColumn := map[string]string{
		"activityAt":   "r.activity_at",
		"activityType": "r.activity_type",
		"userId":       "r.user_id",
		"placeId":      "r.place_id",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "r.activity_at"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	baseParams := make([]any, 0, 4)
	baseWhere := make([]string, 0, 4)
	var scopedPlaceID string
	var scopedPlaceIDs []string

	if params.PlaceID != "" {
		scopedPlaceID = params.PlaceID
		baseParams = append(baseParams, params.PlaceID)
		baseWhere = append(baseWhere, fmt.Sprintf("r.place_id = $%d::uuid", len(baseParams)))
	} else if !auth.IsGlobalAdminRole(params.ActorRole) {
		scopedPlaceIDs = []string{params.ActorUserID}
		baseParams = append(baseParams, params.ActorUserID)
		baseWhere = append(baseWhere, fmt.Sprintf(`r.place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d
			  and upr.is_active = true
			  and p.deleted_at is null
		)`, len(baseParams)))
	}

	if params.UserID != "" {
		baseParams = append(baseParams, params.UserID)
		baseWhere = append(baseWhere, fmt.Sprintf("r.user_id = $%d::uuid", len(baseParams)))
	}
	if params.ActivityType != "" {
		baseParams = append(baseParams, params.ActivityType)
		baseWhere = append(baseWhere, fmt.Sprintf("r.activity_type = $%d", len(baseParams)))
	}

	listParams := append([]any{}, baseParams...)
	listWhere := append([]string{}, baseWhere...)
	if params.FromDate != "" {
		listParams = append(listParams, params.FromDate)
		listWhere = append(listWhere, fmt.Sprintf("r.activity_at >= $%d::date::timestamptz", len(listParams)))
	}
	if params.ToDate != "" {
		listParams = append(listParams, params.ToDate)
		listWhere = append(listWhere, fmt.Sprintf("r.activity_at < ($%d::date + interval '1 day')::timestamptz", len(listParams)))
	}

	listWhereSQL := ""
	if len(listWhere) > 0 {
		listWhereSQL = "where " + strings.Join(listWhere, " and ")
	}
	summaryWhereSQL := ""
	if len(baseWhere) > 0 {
		summaryWhereSQL = "where " + strings.Join(baseWhere, " and ")
	}

	sqlBase := recentUnionSQL()
	listSQL := fmt.Sprintf(`
		%s
		select
		  r.activity_id,
		  r.activity_type,
		  r.activity_at::text,
		  r.place_id,
		  p.place_name,
		  r.user_id,
		  coalesce(nullif(u.full_name, ''), nullif(u.username, '')) as user_name,
		  r.source_id,
		  r.metadata,
		  count(*) over()::int as total_count
		from recent_union r
		left join users u on u.id = r.user_id
		left join places p on p.id = r.place_id
		%s
		order by %s %s, r.activity_id asc
		limit $%d offset $%d
	`, sqlBase, listWhereSQL, sortColumn, sortDirection, len(listParams)+1, len(listParams)+2)

	rows, err := r.db.Query(ctx, listSQL, append(listParams, params.Query.PageSize, params.Query.Offset)...)
	if err != nil {
		return Response{}, err
	}
	defer rows.Close()

	items := make([]Activity, 0)
	total := 0
	for rows.Next() {
		var item Activity
		var metadataRaw []byte
		if err := rows.Scan(&item.ActivityID, &item.ActivityType, &item.ActivityAt, &item.PlaceID, &item.PlaceName, &item.UserID, &item.UserName, &item.SourceID, &metadataRaw, &total); err != nil {
			return Response{}, err
		}
		if len(metadataRaw) > 0 {
			if err := json.Unmarshal(metadataRaw, &item.Metadata); err != nil {
				return Response{}, err
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Response{}, err
	}

	summary := emptySummary()
	tz := strings.TrimSpace(os.Getenv("ACTIVITY_TIMEZONE"))
	if tz == "" {
		tz = defaultActivityTimezone
	}
	summaryParams := append([]any{}, baseParams...)
	summaryParams = append(summaryParams, tz)
	tzPlaceholder := fmt.Sprintf("$%d", len(summaryParams))
	summarySQL := fmt.Sprintf(`
		%s
		select
		  count(*) filter (
		    where (r.activity_at at time zone %s) >= date_trunc('day', now() at time zone %s)
		  )::int as total_today,
		  count(*) filter (
		    where (r.activity_at at time zone %s) >= date_trunc('month', now() at time zone %s)
		  )::int as total_month,
		  count(*) filter (
		    where (r.activity_at at time zone %s) >= date_trunc('year', now() at time zone %s)
		  )::int as total_year,
		  0::int as facility_active,
		  0::int as spot_active,
		  0::int as point_active,
		  count(*) filter (
		    where r.activity_type = 'PATROL_SPOT_SCAN'
		      and (r.activity_at at time zone %s) >= date_trunc('day', now() at time zone %s)
		  )::int as patrol_spot_today,
		  count(*) filter (
		    where r.activity_type = 'PATROL_SPOT_SCAN'
		      and (r.activity_at at time zone %s) >= date_trunc('month', now() at time zone %s)
		  )::int as patrol_spot_month,
		  count(*) filter (
		    where r.activity_type = 'PATROL_SPOT_SCAN'
		      and (r.activity_at at time zone %s) >= date_trunc('year', now() at time zone %s)
		  )::int as patrol_spot_year,
		  count(*) filter (
		    where r.activity_type = 'PATROL_FACILITY_SCAN'
		      and (r.activity_at at time zone %s) >= date_trunc('day', now() at time zone %s)
		  )::int as patrol_facility_today,
		  count(*) filter (
		    where r.activity_type = 'PATROL_FACILITY_SCAN'
		      and (r.activity_at at time zone %s) >= date_trunc('month', now() at time zone %s)
		  )::int as patrol_facility_month,
		  count(*) filter (
		    where r.activity_type = 'PATROL_FACILITY_SCAN'
		      and (r.activity_at at time zone %s) >= date_trunc('year', now() at time zone %s)
		  )::int as patrol_facility_year,
		  count(*) filter (
		    where r.activity_type = 'ATTENDANCE_CHECK_IN'
		      and (r.activity_at at time zone %s) >= date_trunc('day', now() at time zone %s)
		  )::int as attendance_check_in_today,
		  count(*) filter (
		    where r.activity_type = 'ATTENDANCE_CHECK_IN'
		      and (r.activity_at at time zone %s) >= date_trunc('month', now() at time zone %s)
		  )::int as attendance_check_in_month,
		  count(*) filter (
		    where r.activity_type = 'ATTENDANCE_CHECK_IN'
		      and (r.activity_at at time zone %s) >= date_trunc('year', now() at time zone %s)
		  )::int as attendance_check_in_year,
		  count(*) filter (
		    where r.activity_type = 'ATTENDANCE_CHECK_OUT'
		      and (r.activity_at at time zone %s) >= date_trunc('day', now() at time zone %s)
		  )::int as attendance_check_out_today,
		  count(*) filter (
		    where r.activity_type = 'ATTENDANCE_CHECK_OUT'
		      and (r.activity_at at time zone %s) >= date_trunc('month', now() at time zone %s)
		  )::int as attendance_check_out_month,
		  count(*) filter (
		    where r.activity_type = 'ATTENDANCE_CHECK_OUT'
		      and (r.activity_at at time zone %s) >= date_trunc('year', now() at time zone %s)
		  )::int as attendance_check_out_year
		from recent_union r
		%s
	`, sqlBase,
		tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder,
		tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder,
		tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder,
		tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder,
		tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder, tzPlaceholder,
		summaryWhereSQL,
	)
	_ = scopedPlaceIDs
	if err := r.db.QueryRow(ctx, summarySQL, summaryParams...).Scan(
		&summary.TotalToday,
		&summary.TotalMonth,
		&summary.TotalYear,
		&summary.FacilityActive,
		&summary.SpotActive,
		&summary.PointActive,
		&summary.PatrolSpotToday,
		&summary.PatrolSpotMonth,
		&summary.PatrolSpotYear,
		&summary.PatrolFacilityToday,
		&summary.PatrolFacilityMonth,
		&summary.PatrolFacilityYear,
		&summary.AttendanceCheckInToday,
		&summary.AttendanceCheckInMonth,
		&summary.AttendanceCheckInYear,
		&summary.AttendanceCheckOutToday,
		&summary.AttendanceCheckOutMonth,
		&summary.AttendanceCheckOutYear,
	); err != nil {
		return Response{}, err
	}

	activeScopeParams := make([]any, 0, 1)
	facilityScopeWhere := "where fi.is_active = true"
	spotScopeWhere := "where sp.deleted_at is null and sp.status = 'ACTIVE'"
	if scopedPlaceID != "" {
		activeScopeParams = append(activeScopeParams, scopedPlaceID)
		facilityScopeWhere += fmt.Sprintf(" and fs.place_id = $%d::uuid", len(activeScopeParams))
		spotScopeWhere += fmt.Sprintf(" and sp.place_id = $%d::uuid", len(activeScopeParams))
	} else if !auth.IsGlobalAdminRole(params.ActorRole) {
		activeScopeParams = append(activeScopeParams, params.ActorUserID)
		facilityScopeWhere += fmt.Sprintf(` and fs.place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d
			  and upr.is_active = true
			  and p.deleted_at is null
		)`, len(activeScopeParams))
		spotScopeWhere += fmt.Sprintf(` and sp.place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d
			  and upr.is_active = true
			  and p.deleted_at is null
		)`, len(activeScopeParams))
	}
	activeSQL := fmt.Sprintf(`
		select
		  (
		    select count(*)::int
		    from facility_check_items fi
		    join facility_check_spots fs on fs.id = fi.spot_id
		    %s
		  ) as facility_active,
		  (
		    select count(*)::int
		    from spots sp
		    %s
		  ) as spot_active
	`, facilityScopeWhere, spotScopeWhere)
	var activeFacility, activeSpot int
	if err := r.db.QueryRow(ctx, activeSQL, activeScopeParams...).Scan(&activeFacility, &activeSpot); err != nil {
		return Response{}, err
	}
	summary.FacilityActive = activeFacility
	summary.SpotActive = activeSpot
	summary.PointActive = activeSpot

	baseResponse := listquery.BuildResponse(items, params.Query, total)
	var out Response
	out.Data = baseResponse.Data
	out.Pagination = baseResponse.Pagination
	out.Sort = baseResponse.Sort
	out.Summary = summary
	return out, nil
}

func recentUnionSQL() string {
	return `
		with recent_union as (
		  select
		    concat('attendance-check-in-', a.id) as activity_id,
		    'ATTENDANCE_CHECK_IN'::text as activity_type,
		    a.check_in_at as activity_at,
		    a.place_id,
		    a.user_id,
		    a.id::text as source_id,
		    jsonb_build_object(
		      'attendanceId', a.id,
		      'attendanceDate', a.attendance_date::text,
		      'status', a.status,
		      'shiftId', a.shift_id,
		      'assignmentId', a.assignment_id
		    ) as metadata
		  from attendances a
		  where a.check_in_at is not null

		  union all

		  select
		    concat('attendance-check-out-', a.id) as activity_id,
		    'ATTENDANCE_CHECK_OUT'::text as activity_type,
		    a.check_out_at as activity_at,
		    a.place_id,
		    a.user_id,
		    a.id::text as source_id,
		    jsonb_build_object(
		      'attendanceId', a.id,
		      'attendanceDate', a.attendance_date::text,
		      'status', a.status,
		      'shiftId', a.shift_id,
		      'assignmentId', a.assignment_id
		    ) as metadata
		  from attendances a
		  where a.check_out_at is not null

		  union all

		  select
		    concat('patrol-spot-scan-', ps.id) as activity_id,
		    'PATROL_SPOT_SCAN'::text as activity_type,
		    ps.scanned_at as activity_at,
		    ps.place_id,
		    ps.user_id,
		    ps.id::text as source_id,
		    jsonb_build_object(
		      'spotId', ps.spot_id,
		      'patrolRunId', ps.patrol_run_id,
		      'note', ps.note
		    ) as metadata
		  from patrol_scans ps

		  union all

		  select
		    concat('patrol-facility-scan-', fs.id) as activity_id,
		    'PATROL_FACILITY_SCAN'::text as activity_type,
		    fs.scanned_at as activity_at,
		    fs.place_id,
		    fs.user_id,
		    fs.id::text as source_id,
		    jsonb_build_object(
		      'spotId', fs.spot_id,
		      'itemId', fs.item_id,
		      'status', fs.status,
		      'note', fs.note
		    ) as metadata
		  from facility_check_scans fs
		)
	`
}

func emptySummary() Summary {
	return Summary{}
}

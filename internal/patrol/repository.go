package patrol

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
)

var (
	ErrRoutePointNotFound        = errors.New("route point not found")
	ErrPatrolRoundMasterNotFound = errors.New("patrol round master not found")
	ErrPatrolRoundMasterRequired = errors.New("patrol round master required")
	ErrPatrolRunNotFound         = errors.New("patrol run not found")
	ErrPatrolRunClosed           = errors.New("patrol run closed")
	ErrPatrolScanNotFound        = errors.New("patrol scan not found")
	ErrProgressNotFound          = errors.New("patrol progress not found")
	ErrAlreadyExists             = errors.New("already exists")
	ErrForeignKey                = errors.New("related row not found")
)

type Repository struct{ db *pgxpool.Pool }

type PatrolRoutePoint struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	SpotID    string    `json:"spot_id"`
	Seq       int       `json:"seq"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PatrolRoundMaster struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	RoundNo   int       `json:"round_no"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PatrolRoundStatus struct {
	RoundMasterID    string `json:"round_master_id"`
	PlaceID          string `json:"place_id"`
	UserID           string `json:"user_id"`
	ShiftID          string `json:"shift_id,omitempty"`
	PeriodDate       string `json:"period_date,omitempty"`
	RoundNo          int    `json:"round_no"`
	TotalActiveSpots int    `json:"total_active_spots"`
	ScannedSpots     int    `json:"scanned_spots"`
	ScanCount        int    `json:"scan_count"`
	Status           string `json:"status"`
}

type PatrolScan struct {
	ID           string    `json:"id"`
	PlaceID      string    `json:"place_id"`
	UserID       string    `json:"user_id"`
	SpotID       string    `json:"spot_id"`
	AttendanceID *string   `json:"attendance_id"`
	PatrolRunID  string    `json:"patrol_run_id"`
	RunNo        int       `json:"run_no"`
	ScannedAt    time.Time `json:"scanned_at"`
	SubmitAt     time.Time `json:"submit_at"`
	PhotoURL     *string   `json:"photo_url"`
	Note         *string   `json:"note"`
}

type PatrolRun struct {
	ID                 string     `json:"id"`
	PlaceID            string     `json:"place_id"`
	UserID             string     `json:"user_id"`
	AttendanceID       *string    `json:"attendance_id"`
	ShiftID            *string    `json:"shift_id"`
	ShiftName          *string    `json:"shift_name"`
	RunNo              int        `json:"run_no"`
	TotalActiveSpots   int        `json:"total_active_spots"`
	Status             string     `json:"status"`
	StartedAt          time.Time  `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ScanCount          int        `json:"scan_count"`
	UniqueScannedSpots int        `json:"unique_scanned_spots"`
}

type patrolShiftScope struct {
	ShiftID     string
	PeriodDate  string
	WindowStart time.Time
	WindowEnd   time.Time
}

type CreateScanResult struct {
	ID                 string
	PatrolRunID        string
	PatrolRunNo        int
	IsNewPatrolRun     bool
	PatrolRunCompleted bool
}

type PatrolProgress struct {
	AttendanceID     string               `json:"attendance_id"`
	PlaceID          string               `json:"place_id"`
	UserID           string               `json:"user_id"`
	ShiftID          *string              `json:"shift_id"`
	AttendanceDate   string               `json:"attendance_date"`
	CheckInAt        *time.Time           `json:"check_in_at"`
	CheckOutAt       *time.Time           `json:"check_out_at"`
	TotalRouteSpots  int                  `json:"total_route_spots"`
	PatrolledSpots   int                  `json:"patrolled_spots"`
	UnpatrolledSpots int                  `json:"unpatrolled_spots"`
	TotalScans       int                  `json:"total_scans"`
	TotalPatrolRuns  int                  `json:"total_patrol_runs"`
	Spots            []PatrolProgressSpot `json:"spots"`
}

type PatrolProgressSpot struct {
	SpotID          string     `json:"spot_id"`
	SpotCode        string     `json:"spot_code"`
	SpotName        string     `json:"spot_name"`
	Seq             int        `json:"seq"`
	ScanCount       int        `json:"scan_count"`
	IsPatrolled     bool       `json:"is_patrolled"`
	LastScannedAt   *time.Time `json:"last_scanned_at"`
	LastPatrolRunID *string    `json:"last_patrol_run_id"`
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) ListRoutePoints(ctx context.Context, actorUserID, actorRole, placeID string, query listquery.Query) (listquery.Response[PatrolRoutePoint], error) {
	sortColumn := map[string]string{"seq": "seq", "createdAt": "created_at", "updatedAt": "updated_at", "spotId": "spot_id", "isActive": "is_active"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "seq"
	}
	sortDirection := "asc"
	if query.SortOrder == listquery.SortDesc {
		sortDirection = "desc"
	}
	sql := `select id, place_id, spot_id, seq, is_active, created_at, updated_at, count(*) over()::int as total_count from patrol_route_points where place_id = $1`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[PatrolRoutePoint]{}, err
	}
	defer rows.Close()
	data := make([]PatrolRoutePoint, 0)
	total := 0
	for rows.Next() {
		var item PatrolRoutePoint
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.SpotID, &item.Seq, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[PatrolRoutePoint]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) CreateRoutePoint(ctx context.Context, placeID, spotID string, seq int, isActive bool) (string, error) {
	const sql = `insert into patrol_route_points (place_id, spot_id, seq, is_active) values ($1,$2,$3,$4) returning id`
	var id string
	err := r.db.QueryRow(ctx, sql, placeID, spotID, seq, isActive).Scan(&id)
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

func (r *Repository) DeleteRoutePoint(ctx context.Context, id, placeID string) (string, error) {
	const sql = `delete from patrol_route_points where id = $1 and place_id = $2 returning id`
	var out string
	err := r.db.QueryRow(ctx, sql, id, placeID).Scan(&out)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRoutePointNotFound
		}
		return "", err
	}
	return out, nil
}

func (r *Repository) ListRoundMasters(ctx context.Context, actorUserID, actorRole, placeID string, query listquery.Query) (listquery.Response[PatrolRoundMaster], error) {
	sortColumn := map[string]string{"roundNo": "round_no", "isActive": "is_active", "createdAt": "created_at", "updatedAt": "updated_at"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "round_no"
	}
	sortDirection := "asc"
	if query.SortOrder == listquery.SortDesc {
		sortDirection = "desc"
	}
	sql := `select id, place_id, round_no, is_active, created_at, updated_at, count(*) over()::int as total_count from patrol_round_masters where place_id = $1`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[PatrolRoundMaster]{}, err
	}
	defer rows.Close()
	data := make([]PatrolRoundMaster, 0)
	total := 0
	for rows.Next() {
		var item PatrolRoundMaster
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.RoundNo, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[PatrolRoundMaster]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) CreateRoundMaster(ctx context.Context, placeID string, roundNo int, isActive bool) (*PatrolRoundMaster, error) {
	const sql = `
		insert into patrol_round_masters (id, place_id, round_no, is_active)
		values ($1,$2,$3,$4)
		returning id, place_id, round_no, is_active, created_at, updated_at
	`
	id := newPatrolRunID()
	var item PatrolRoundMaster
	err := r.db.QueryRow(ctx, sql, id, placeID, roundNo, isActive).Scan(
		&item.ID,
		&item.PlaceID,
		&item.RoundNo,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) GetRoundMaster(ctx context.Context, id string) (*PatrolRoundMaster, error) {
	const sql = `
		select id, place_id, round_no, is_active, created_at, updated_at
		from patrol_round_masters
		where id = $1
	`
	var item PatrolRoundMaster
	if err := r.db.QueryRow(ctx, sql, id).Scan(
		&item.ID,
		&item.PlaceID,
		&item.RoundNo,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPatrolRoundMasterNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateRoundMaster(ctx context.Context, id string, roundNo *int, isActive *bool) (*PatrolRoundMaster, error) {
	setParts := make([]string, 0)
	args := make([]any, 0)
	argPos := 1
	if roundNo != nil {
		setParts = append(setParts, fmt.Sprintf("round_no = $%d", argPos))
		args = append(args, *roundNo)
		argPos++
	}
	if isActive != nil {
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *isActive)
		argPos++
	}
	if len(setParts) == 0 {
		return nil, ErrPatrolRoundMasterNotFound
	}
	args = append(args, id)
	sql := fmt.Sprintf(`
		update patrol_round_masters
		set %s,
		    updated_at = now()
		where id = $%d
		returning id, place_id, round_no, is_active, created_at, updated_at
	`, strings.Join(setParts, ", "), argPos)
	var item PatrolRoundMaster
	if err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.PlaceID,
		&item.RoundNo,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrPatrolRoundMasterNotFound
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) DeleteRoundMaster(ctx context.Context, id, placeID string) (string, error) {
	const sql = `delete from patrol_round_masters where id = $1 and place_id = $2 returning id`
	var out string
	if err := r.db.QueryRow(ctx, sql, id, placeID).Scan(&out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPatrolRoundMasterNotFound
		}
		return "", err
	}
	return out, nil
}

func (r *Repository) ListRoundStatuses(ctx context.Context, actorUserID, actorRole, placeID, userID, shiftID, date, fromDate, toDate string) ([]PatrolRoundStatus, error) {
	if !auth.IsGlobalAdminRole(actorRole) {
		var hasAccess bool
		if err := r.db.QueryRow(
			ctx,
			`select exists(
				select 1
				from user_place_roles upr
				join places p on p.id = upr.place_id
				where upr.user_id = $1
				  and upr.place_id = $2
				  and upr.is_active = true
				  and p.deleted_at is null
			)`,
			actorUserID,
			placeID,
		).Scan(&hasAccess); err != nil {
			return nil, err
		}
		if !hasAccess {
			return []PatrolRoundStatus{}, nil
		}
	}

	totalActiveSpots, err := r.countActiveRouteSpotsDirect(ctx, placeID)
	if err != nil {
		return nil, err
	}

	args := []any{placeID, userID}
	periodFilter := ""

	if fromDate != "" {
		args = append(args, fromDate)
		periodFilter += fmt.Sprintf(" and (ps.scanned_at at time zone 'Asia/Jakarta')::date >= $%d::date", len(args))
	}
	if toDate != "" {
		args = append(args, toDate)
		periodFilter += fmt.Sprintf(" and (ps.scanned_at at time zone 'Asia/Jakarta')::date <= $%d::date", len(args))
	}
	if date != "" {
		args = append(args, date)
		periodFilter += fmt.Sprintf(" and (ps.scanned_at at time zone 'Asia/Jakarta')::date = $%d::date", len(args))
	}
	if shiftID != "" && date != "" {
		startAt, endAt, err := r.resolveShiftWindow(ctx, shiftID, date)
		if err != nil {
			return nil, err
		}
		args = append(args, startAt, endAt)
		periodFilter += fmt.Sprintf(" and ps.scanned_at >= $%d and ps.scanned_at < $%d", len(args)-1, len(args))
	}

	sql := fmt.Sprintf(`
		with master_scans as (
			select
				ps.id,
				ps.spot_id,
				row_number() over (
					partition by ps.spot_id
					order by coalesce(ps.submit_at, ps.scanned_at) asc, ps.id asc
				)::int as occurrence_no
			from patrol_scans ps
			join patrol_runs pr
			  on pr.id = ps.patrol_run_id
			where pr.place_id = $1
			  and pr.user_id = $2
			  and pr.run_no = 0
			  %s
		)
		select
			prm.id,
			prm.place_id,
			prm.round_no,
			count(distinct case when prp.is_active then ms.spot_id end)::int as scanned_spots,
			count(ms.id)::int as scan_count
		from patrol_round_masters prm
		left join master_scans ms
		  on ms.occurrence_no = prm.round_no
		left join patrol_route_points prp
		  on prp.place_id = prm.place_id
		 and prp.spot_id = ms.spot_id
		 and prp.is_active = true
		where prm.place_id = $1
		  and prm.is_active = true
		group by prm.id, prm.place_id, prm.round_no
		order by prm.round_no asc, prm.id asc
	`, periodFilter)

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PatrolRoundStatus, 0)
	for rows.Next() {
		var item PatrolRoundStatus
		item.UserID = userID
		item.ShiftID = shiftID
		item.PeriodDate = date
		item.TotalActiveSpots = totalActiveSpots
		if err := rows.Scan(
			&item.RoundMasterID,
			&item.PlaceID,
			&item.RoundNo,
			&item.ScannedSpots,
			&item.ScanCount,
		); err != nil {
			return nil, err
		}
		switch {
		case item.ScannedSpots <= 0:
			item.Status = "empty"
		case totalActiveSpots > 0 && item.ScannedSpots >= totalActiveSpots:
			item.Status = "full"
		default:
			item.Status = "partial"
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListRuns(ctx context.Context, actorUserID, actorRole, placeID, userID, attendanceID, shiftID string, runNo *int, status, fromDate, toDate string, query listquery.Query) (listquery.Response[PatrolRun], error) {
	sortColumn := map[string]string{
		"runNo":            "pr.run_no",
		"status":           "pr.status",
		"startedAt":        "pr.started_at",
		"completedAt":      "pr.completed_at",
		"createdAt":        "pr.created_at",
		"updatedAt":        "pr.updated_at",
		"userId":           "pr.user_id",
		"attendanceId":     "pr.attendance_id",
		"totalActiveSpots": "pr.total_active_spots",
	}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "pr.started_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select
			pr.id,
			pr.place_id,
			pr.user_id,
			pr.attendance_id,
			inferred_shift.id as shift_id,
			inferred_shift.name as shift_name,
			pr.run_no,
			pr.total_active_spots,
			pr.status,
			pr.started_at,
			pr.completed_at,
			pr.created_at,
			pr.updated_at,
			count(ps.id)::int as scan_count,
			count(distinct ps.spot_id)::int as unique_scanned_spots,
			count(*) over()::int as total_count
		from patrol_runs pr
		left join patrol_scans ps on ps.patrol_run_id = pr.id
		left join lateral (
			select shf.id, shf.name
			from shifts shf
			where shf.place_id = pr.place_id
			  and shf.is_active = true
			  and (
				(shf.start_time <= shf.end_time and (pr.started_at at time zone 'Asia/Jakarta')::time >= shf.start_time and (pr.started_at at time zone 'Asia/Jakarta')::time < shf.end_time)
				or
				(shf.start_time > shf.end_time and ((pr.started_at at time zone 'Asia/Jakarta')::time >= shf.start_time or (pr.started_at at time zone 'Asia/Jakarta')::time < shf.end_time))
			  )
			order by shf.start_time asc, shf.created_at asc, shf.id asc
			limit 1
		) inferred_shift on true
		where pr.place_id = $1
	`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and pr.place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	if userID != "" {
		args = append(args, userID)
		sql += fmt.Sprintf(" and pr.user_id = $%d", len(args))
	}
	if attendanceID != "" {
		args = append(args, attendanceID)
		sql += fmt.Sprintf(" and pr.attendance_id = $%d", len(args))
	}
	if shiftID != "" {
		args = append(args, shiftID)
		sql += fmt.Sprintf(" and inferred_shift.id = $%d", len(args))
	}
	if runNo != nil {
		args = append(args, *runNo)
		sql += fmt.Sprintf(" and pr.run_no = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		sql += fmt.Sprintf(" and lower(pr.status) = lower($%d)", len(args))
	}
	if fromDate != "" {
		args = append(args, fromDate)
		sql += fmt.Sprintf(" and (pr.started_at at time zone 'Asia/Jakarta')::date >= $%d::date", len(args))
	}
	if toDate != "" {
		args = append(args, toDate)
		sql += fmt.Sprintf(" and (pr.started_at at time zone 'Asia/Jakarta')::date <= $%d::date", len(args))
	}

	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(`
		group by pr.id, inferred_shift.id, inferred_shift.name
		order by %s %s, pr.id asc
		limit $%d offset $%d
	`, sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[PatrolRun]{}, err
	}
	defer rows.Close()

	data := make([]PatrolRun, 0)
	total := 0
	for rows.Next() {
		var item PatrolRun
		if err := rows.Scan(
			&item.ID,
			&item.PlaceID,
			&item.UserID,
			&item.AttendanceID,
			&item.ShiftID,
			&item.ShiftName,
			&item.RunNo,
			&item.TotalActiveSpots,
			&item.Status,
			&item.StartedAt,
			&item.CompletedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.ScanCount,
			&item.UniqueScannedSpots,
			&total,
		); err != nil {
			return listquery.Response[PatrolRun]{}, err
		}
		if err := r.applyCurrentRunProgress(ctx, &item); err != nil {
			return listquery.Response[PatrolRun]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) GetRun(ctx context.Context, actorUserID, actorRole, runID string) (*PatrolRun, error) {
	sql := `
		select
			pr.id,
			pr.place_id,
			pr.user_id,
			pr.attendance_id,
			inferred_shift.id as shift_id,
			inferred_shift.name as shift_name,
			pr.run_no,
			pr.total_active_spots,
			pr.status,
			pr.started_at,
			pr.completed_at,
			pr.created_at,
			pr.updated_at,
			count(ps.id)::int as scan_count,
			count(distinct ps.spot_id)::int as unique_scanned_spots
		from patrol_runs pr
		left join patrol_scans ps on ps.patrol_run_id = pr.id
		left join lateral (
			select shf.id, shf.name
			from shifts shf
			where shf.place_id = pr.place_id
			  and shf.is_active = true
			  and (
				(shf.start_time <= shf.end_time and (pr.started_at at time zone 'Asia/Jakarta')::time >= shf.start_time and (pr.started_at at time zone 'Asia/Jakarta')::time < shf.end_time)
				or
				(shf.start_time > shf.end_time and ((pr.started_at at time zone 'Asia/Jakarta')::time >= shf.start_time or (pr.started_at at time zone 'Asia/Jakarta')::time < shf.end_time))
			  )
			order by shf.start_time asc, shf.created_at asc, shf.id asc
			limit 1
		) inferred_shift on true
		where pr.id = $1
	`
	args := []any{runID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and pr.place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	sql += ` group by pr.id, inferred_shift.id, inferred_shift.name`

	var item PatrolRun
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.PlaceID,
		&item.UserID,
		&item.AttendanceID,
		&item.ShiftID,
		&item.ShiftName,
		&item.RunNo,
		&item.TotalActiveSpots,
		&item.Status,
		&item.StartedAt,
		&item.CompletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.ScanCount,
		&item.UniqueScannedSpots,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPatrolRunNotFound
		}
		return nil, err
	}
	if err := r.applyCurrentRunProgress(ctx, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) applyCurrentRunProgress(ctx context.Context, run *PatrolRun) error {
	const sql = `
		with active_route_spots as (
			select spot_id
			from patrol_route_points
			where place_id = $1
			  and is_active = true
		)
		select
			(select count(*)::int from active_route_spots) as total_active_spots,
			(
				select coalesce(count(distinct ps.spot_id), 0)::int
				from patrol_scans ps
				join active_route_spots ars on ars.spot_id = ps.spot_id
				where ps.patrol_run_id = $2
			) as unique_scanned_spots,
			(
				select max(coalesce(ps.submit_at, ps.scanned_at))
				from patrol_scans ps
				join active_route_spots ars on ars.spot_id = ps.spot_id
				where ps.patrol_run_id = $2
			) as last_active_scan_at
	`

	var currentTotalActiveSpots int
	var currentUniqueScannedSpots int
	var lastActiveScanAt *time.Time
	if err := r.db.QueryRow(ctx, sql, run.PlaceID, run.ID).Scan(&currentTotalActiveSpots, &currentUniqueScannedSpots, &lastActiveScanAt); err != nil {
		return err
	}

	// Prefer current active route-point totals when the place still has active spots.
	if currentTotalActiveSpots > 0 {
		run.TotalActiveSpots = currentTotalActiveSpots
		run.UniqueScannedSpots = currentUniqueScannedSpots

		if strings.EqualFold(run.Status, "active") && currentUniqueScannedSpots >= currentTotalActiveSpots {
			run.Status = "completed"
			if run.CompletedAt == nil {
				if lastActiveScanAt != nil {
					run.CompletedAt = lastActiveScanAt
				} else {
					completedAt := run.UpdatedAt
					run.CompletedAt = &completedAt
				}
			}
		}
	}

	return nil
}

func (r *Repository) CreateRun(ctx context.Context, placeID, userID string, attendanceID *string, runNo, totalActiveSpots *int, status *string) (*PatrolRun, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Patrol runs are now scoped only by place + user, not attendance.
	attendanceID = nil

	resolvedRunNo := 0
	if runNo != nil && *runNo > 0 {
		resolvedRunNo = *runNo
		if err := r.ensureRoundNoAllowed(ctx, tx, placeID, resolvedRunNo); err != nil {
			return nil, err
		}
	} else {
		hasMasters, err := r.hasRoundMasters(ctx, tx, placeID)
		if err != nil {
			return nil, err
		}
		if hasMasters {
			return nil, ErrPatrolRoundMasterRequired
		}
		resolvedRunNo, err = r.nextRunNo(ctx, tx, placeID, userID, attendanceID)
		if err != nil {
			return nil, err
		}
	}

	resolvedTotalActiveSpots := 0
	if totalActiveSpots != nil && *totalActiveSpots >= 0 {
		resolvedTotalActiveSpots = *totalActiveSpots
	} else {
		resolvedTotalActiveSpots, err = r.countActiveRouteSpots(ctx, tx, placeID)
		if err != nil {
			return nil, err
		}
	}

	resolvedStatus := "active"
	if status != nil && strings.TrimSpace(*status) != "" {
		resolvedStatus = strings.ToLower(strings.TrimSpace(*status))
	}
	if resolvedStatus != "active" && resolvedStatus != "completed" {
		return nil, fmt.Errorf("invalid patrol run status")
	}

	runID := newPatrolRunID()
	const sql = `
		insert into patrol_runs (id, place_id, user_id, attendance_id, run_no, total_active_spots, status, started_at, completed_at)
		values ($1,$2,$3,$4,$5,$6,$7,now(),case when $7 = 'completed' then now() else null end)
	`
	if _, err := tx.Exec(ctx, sql, runID, placeID, userID, attendanceID, resolvedRunNo, resolvedTotalActiveSpots, resolvedStatus); err != nil {
		switch {
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetRun(ctx, "", auth.GlobalRoleSuperUser, runID)
}

func (r *Repository) UpdateRun(ctx context.Context, id string, runNo, totalActiveSpots *int, status *string) (*PatrolRun, error) {
	if runNo != nil {
		runPlaceID, err := r.getRunPlaceID(ctx, id)
		if err != nil {
			return nil, err
		}
		tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if err := r.ensureRoundNoAllowed(ctx, tx, runPlaceID, *runNo); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
	}

	setParts := make([]string, 0)
	args := make([]any, 0)
	argPos := 1

	if runNo != nil {
		setParts = append(setParts, fmt.Sprintf("run_no = $%d", argPos))
		args = append(args, *runNo)
		argPos++
	}
	if totalActiveSpots != nil {
		setParts = append(setParts, fmt.Sprintf("total_active_spots = $%d", argPos))
		args = append(args, *totalActiveSpots)
		argPos++
	}
	if status != nil {
		value := strings.ToLower(strings.TrimSpace(*status))
		setParts = append(setParts, fmt.Sprintf("status = $%d", argPos))
		args = append(args, value)
		argPos++
		if value == "completed" {
			setParts = append(setParts, "completed_at = coalesce(completed_at, now())")
		}
		if value == "active" {
			setParts = append(setParts, "completed_at = null")
		}
	}
	if len(setParts) == 0 {
		return nil, ErrPatrolRunNotFound
	}

	args = append(args, id)
	sql := fmt.Sprintf(`
		update patrol_runs
		set %s,
		    updated_at = now()
		where id = $%d
		returning id
	`, strings.Join(setParts, ", "), argPos)
	var out string
	if err := r.db.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrPatrolRunNotFound
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}
	return r.GetRun(ctx, "", auth.GlobalRoleSuperUser, out)
}

func (r *Repository) DeleteRun(ctx context.Context, id string) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from patrol_scans where patrol_run_id = $1`, id); err != nil {
		return "", err
	}

	var out string
	if err := tx.QueryRow(ctx, `delete from patrol_runs where id = $1 returning id`, id).Scan(&out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPatrolRunNotFound
		}
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return out, nil
}

func (r *Repository) ListScans(ctx context.Context, actorUserID, actorRole, placeID, patrolRunID, userID, attendanceID, fromDate, toDate string, query listquery.Query) (listquery.Response[PatrolScan], error) {
	sortColumn := map[string]string{"scannedAt": "scanned_at", "submitAt": "submit_at", "placeId": "place_id", "userId": "user_id", "spotId": "spot_id", "patrolRunId": "patrol_run_id"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "scanned_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `select ps.id, ps.place_id, ps.user_id, ps.spot_id, ps.attendance_id, ps.patrol_run_id, coalesce(pr.run_no, 0) as run_no, ps.scanned_at, ps.submit_at, ps.photo_url, ps.note, count(*) over()::int as total_count from patrol_scans ps left join patrol_runs pr on pr.id = ps.patrol_run_id where ps.place_id = $1`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and ps.place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	if patrolRunID != "" {
		args = append(args, patrolRunID)
		sql += fmt.Sprintf(" and ps.patrol_run_id = $%d", len(args))
	}
	if userID != "" {
		args = append(args, userID)
		sql += fmt.Sprintf(" and ps.user_id = $%d", len(args))
	}
	if attendanceID != "" {
		args = append(args, attendanceID)
		sql += fmt.Sprintf(" and ps.attendance_id = $%d", len(args))
	}
	if fromDate != "" {
		args = append(args, fromDate)
		sql += fmt.Sprintf(" and (ps.scanned_at at time zone 'Asia/Jakarta')::date >= $%d::date", len(args))
	}
	if toDate != "" {
		args = append(args, toDate)
		sql += fmt.Sprintf(" and (ps.scanned_at at time zone 'Asia/Jakarta')::date <= $%d::date", len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, ps.id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[PatrolScan]{}, err
	}
	defer rows.Close()
	data := make([]PatrolScan, 0)
	total := 0
	for rows.Next() {
		var item PatrolScan
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.SpotID, &item.AttendanceID, &item.PatrolRunID, &item.RunNo, &item.ScannedAt, &item.SubmitAt, &item.PhotoURL, &item.Note, &total); err != nil {
			return listquery.Response[PatrolScan]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) GetScan(ctx context.Context, actorUserID, actorRole, scanID string) (*PatrolScan, error) {
	sql := `select id, place_id, user_id, spot_id, attendance_id, patrol_run_id, scanned_at, submit_at, photo_url, note from patrol_scans where id = $1`
	args := []any{scanID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	var item PatrolScan
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.PlaceID,
		&item.UserID,
		&item.SpotID,
		&item.AttendanceID,
		&item.PatrolRunID,
		&item.ScannedAt,
		&item.SubmitAt,
		&item.PhotoURL,
		&item.Note,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPatrolScanNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateScan(ctx context.Context, scanID string, patrolRunID, spotID, scannedAt, submitAt, photoURL, note *string) (*PatrolScan, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var existing PatrolScan
	if err := tx.QueryRow(ctx, `
		select id, place_id, user_id, spot_id, attendance_id, patrol_run_id, scanned_at, submit_at, photo_url, note
		from patrol_scans
		where id = $1
		for update
	`, scanID).Scan(
		&existing.ID,
		&existing.PlaceID,
		&existing.UserID,
		&existing.SpotID,
		&existing.AttendanceID,
		&existing.PatrolRunID,
		&existing.ScannedAt,
		&existing.SubmitAt,
		&existing.PhotoURL,
		&existing.Note,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPatrolScanNotFound
		}
		return nil, err
	}

	targetRunID := existing.PatrolRunID
	if patrolRunID != nil && strings.TrimSpace(*patrolRunID) != "" {
		targetRunID = strings.TrimSpace(*patrolRunID)
		var runPlaceID, runUserID string
		if err := tx.QueryRow(ctx, `select place_id, user_id from patrol_runs where id = $1`, targetRunID).Scan(&runPlaceID, &runUserID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPatrolRunNotFound
			}
			return nil, err
		}
		if runPlaceID != existing.PlaceID || runUserID != existing.UserID {
			return nil, ErrForeignKey
		}
	}

	setParts := make([]string, 0)
	args := make([]any, 0)
	argPos := 1

	if patrolRunID != nil && strings.TrimSpace(*patrolRunID) != "" {
		setParts = append(setParts, fmt.Sprintf("patrol_run_id = $%d", argPos))
		args = append(args, targetRunID)
		argPos++
	}
	if spotID != nil && strings.TrimSpace(*spotID) != "" {
		setParts = append(setParts, fmt.Sprintf("spot_id = $%d", argPos))
		args = append(args, strings.TrimSpace(*spotID))
		argPos++
	}
	if scannedAt != nil && strings.TrimSpace(*scannedAt) != "" {
		setParts = append(setParts, fmt.Sprintf("scanned_at = $%d::timestamptz", argPos))
		args = append(args, strings.TrimSpace(*scannedAt))
		argPos++
	}
	if submitAt != nil && strings.TrimSpace(*submitAt) != "" {
		setParts = append(setParts, fmt.Sprintf("submit_at = $%d::timestamptz", argPos))
		args = append(args, strings.TrimSpace(*submitAt))
		argPos++
	}
	if photoURL != nil {
		setParts = append(setParts, fmt.Sprintf("photo_url = nullif($%d, '')", argPos))
		args = append(args, strings.TrimSpace(*photoURL))
		argPos++
	}
	if note != nil {
		setParts = append(setParts, fmt.Sprintf("note = nullif($%d, '')", argPos))
		args = append(args, strings.TrimSpace(*note))
		argPos++
	}
	if len(setParts) == 0 {
		return &existing, nil
	}

	args = append(args, scanID)
	sql := fmt.Sprintf(`
		update patrol_scans
		set %s
		where id = $%d
		returning id, place_id, user_id, spot_id, attendance_id, patrol_run_id, scanned_at, submit_at, photo_url, note
	`, strings.Join(setParts, ", "), argPos)

	var updated PatrolScan
	if err := tx.QueryRow(ctx, sql, args...).Scan(
		&updated.ID,
		&updated.PlaceID,
		&updated.UserID,
		&updated.SpotID,
		&updated.AttendanceID,
		&updated.PatrolRunID,
		&updated.ScannedAt,
		&updated.SubmitAt,
		&updated.PhotoURL,
		&updated.Note,
	); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrPatrolScanNotFound
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}

	if err := r.syncRunState(ctx, tx, existing.PatrolRunID); err != nil {
		return nil, err
	}
	if updated.PatrolRunID != existing.PatrolRunID {
		if err := r.syncRunState(ctx, tx, updated.PatrolRunID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *Repository) DeleteScan(ctx context.Context, scanID string) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var runID string
	if err := tx.QueryRow(ctx, `delete from patrol_scans where id = $1 returning id, patrol_run_id`, scanID).Scan(&scanID, &runID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPatrolScanNotFound
		}
		return "", err
	}

	if err := r.syncRunState(ctx, tx, runID); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return scanID, nil
}

func (r *Repository) CreateScan(ctx context.Context, placeID, userID, spotID, patrolRunID string, explicitRunNo *int, attendanceID *string, scannedAt, submitAt, photoURL, note *string) (*CreateScanResult, error) {
	// Patrol scans are stored independently from attendance.
	attendanceID = nil
	patrolRunID = strings.TrimSpace(patrolRunID)

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	runNo := 0
	totalActiveSpots := 0
	isNewPatrolRun := false
	hasMasters, err := r.hasRoundMasters(ctx, tx, placeID)
	if err != nil {
		return nil, err
	}
	if hasMasters {
		totalActiveSpots, err = r.countActiveRouteSpots(ctx, tx, placeID)
		if err != nil {
			return nil, err
		}
		if explicitRunNo != nil && *explicitRunNo > 0 {
			if err := r.ensureRoundNoAllowed(ctx, tx, placeID, *explicitRunNo); err != nil {
				return nil, err
			}
			runNo = *explicitRunNo
			patrolRunID, _, isNewPatrolRun, err = r.getOrCreateRunByNo(ctx, tx, placeID, userID, runNo, totalActiveSpots, startedAtOrSubmit(scannedAt, submitAt))
			if err != nil {
				return nil, err
			}
		} else {
			patrolRunID, isNewPatrolRun, err = r.getOrCreateMasterRun(ctx, tx, placeID, userID, startedAtOrSubmit(scannedAt, submitAt))
			if err != nil {
				return nil, err
			}
			runNo, err = r.resolveProjectedRoundNoForSpot(ctx, tx, placeID, patrolRunID, spotID)
			if err != nil {
				return nil, err
			}
		}
	} else if patrolRunID != "" {
		runNo, totalActiveSpots, err = r.validateManualRun(ctx, tx, patrolRunID, placeID, userID)
		if err != nil {
			return nil, err
		}
	} else {
		totalActiveSpots, err = r.countActiveRouteSpots(ctx, tx, placeID)
		if err != nil {
			return nil, err
		}
		patrolRunID, runNo, isNewPatrolRun, err = r.resolveAutomaticRunForSpot(ctx, tx, placeID, userID, spotID, totalActiveSpots, scannedAt)
		if err != nil {
			return nil, err
		}
	}

	const sql = `insert into patrol_scans (place_id, user_id, spot_id, attendance_id, patrol_run_id, scanned_at, submit_at, photo_url, note) values ($1,$2,$3,$4,$5,coalesce($6::timestamptz, now()),coalesce($7::timestamptz, now()),$8,$9) returning id`
	var id string
	err = tx.QueryRow(ctx, sql, placeID, userID, spotID, attendanceID, patrolRunID, scannedAt, submitAt, photoURL, note).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return nil, ErrAlreadyExists
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}

	runCompleted := false
	if !hasMasters || runNo == 0 {
		if err := r.syncRunState(ctx, tx, patrolRunID); err != nil {
			return nil, err
		}
		runCompleted, err = r.isRunMarkedCompleted(ctx, tx, patrolRunID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateScanResult{
		ID:                 id,
		PatrolRunID:        patrolRunID,
		PatrolRunNo:        runNo,
		IsNewPatrolRun:     isNewPatrolRun,
		PatrolRunCompleted: runCompleted,
	}, nil
}

func startedAtOrSubmit(scannedAt, submitAt *string) *string {
	if submitAt != nil && strings.TrimSpace(*submitAt) != "" {
		return submitAt
	}
	if scannedAt != nil && strings.TrimSpace(*scannedAt) != "" {
		return scannedAt
	}
	return nil
}

func (r *Repository) resolveAutomaticRunForSpot(ctx context.Context, tx pgx.Tx, placeID, userID, spotID string, totalActiveSpots int, startedAt *string) (string, int, bool, error) {
	roundNos, err := r.listActiveRoundNos(ctx, tx, placeID)
	if err != nil {
		return "", 0, false, err
	}
	if len(roundNos) == 0 {
		runID, runNo, isNew, err := r.ensureActiveRun(ctx, tx, placeID, userID, totalActiveSpots, startedAt)
		return runID, runNo, isNew, err
	}

	for _, roundNo := range roundNos {
		runID, found, createdNow, err := r.getOrCreateRunByNo(ctx, tx, placeID, userID, roundNo, totalActiveSpots, startedAt)
		if err != nil {
			return "", 0, false, err
		}
		if !found || createdNow {
			return runID, roundNo, true, nil
		}
		spotExists, err := r.runAlreadyHasSpot(ctx, tx, runID, spotID)
		if err != nil {
			return "", 0, false, err
		}
		if !spotExists {
			return runID, roundNo, false, nil
		}
	}

	runID, _, createdNow, err := r.getOrCreateRunByNo(ctx, tx, placeID, userID, 0, 0, startedAt)
	if err != nil {
		return "", 0, false, err
	}
	return runID, 0, createdNow, nil
}

func (r *Repository) validateManualRun(ctx context.Context, tx pgx.Tx, patrolRunID, placeID, userID string) (int, int, error) {
	const sql = `
		select run_no, total_active_spots, status
		from patrol_runs
		where id = $1
		  and place_id = $2
		  and user_id = $3
		for update
	`
	var runNo, totalActiveSpots int
	var status string
	if err := tx.QueryRow(ctx, sql, patrolRunID, placeID, userID).Scan(&runNo, &totalActiveSpots, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, ErrPatrolRunNotFound
		}
		return 0, 0, err
	}
	if strings.EqualFold(strings.TrimSpace(status), "completed") {
		return 0, 0, ErrPatrolRunClosed
	}
	return runNo, totalActiveSpots, nil
}

func (r *Repository) isRunMarkedCompleted(ctx context.Context, tx pgx.Tx, runID string) (bool, error) {
	var status string
	if err := tx.QueryRow(ctx, `select status from patrol_runs where id = $1`, runID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrPatrolRunNotFound
		}
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(status), "completed"), nil
}

func (r *Repository) getOrCreateMasterRun(ctx context.Context, tx pgx.Tx, placeID, userID string, startedAt *string) (string, bool, error) {
	scope, err := r.resolveShiftScopeForScan(ctx, placeID, startedAt)
	if err != nil {
		return "", false, err
	}
	if scope != nil {
		runID, found, createdNow, err := r.getOrCreateMasterRunForScope(ctx, tx, placeID, userID, scope)
		if err != nil {
			return "", false, err
		}
		if found {
			return runID, false, nil
		}
		return runID, createdNow, nil
	}
	runID, found, createdNow, err := r.getOrCreateRunByNo(ctx, tx, placeID, userID, 0, 0, startedAt)
	if err != nil {
		return "", false, err
	}
	if found {
		return runID, false, nil
	}
	return runID, createdNow, nil
}

func (r *Repository) resolveProjectedRoundNoForSpot(ctx context.Context, tx pgx.Tx, placeID, masterRunID, spotID string) (int, error) {
	roundNos, err := r.listActiveRoundNos(ctx, tx, placeID)
	if err != nil {
		return 0, err
	}
	if len(roundNos) == 0 {
		return 0, nil
	}

	var currentCount int
	if err := tx.QueryRow(ctx, `
		select count(*)::int
		from patrol_scans
		where patrol_run_id = $1
		  and spot_id = $2
	`, masterRunID, spotID).Scan(&currentCount); err != nil {
		return 0, err
	}
	nextOrdinal := currentCount + 1
	if nextOrdinal > len(roundNos) {
		return 0, nil
	}
	return roundNos[nextOrdinal-1], nil
}

func (r *Repository) getOrCreateMasterRunForScope(ctx context.Context, tx pgx.Tx, placeID, userID string, scope *patrolShiftScope) (string, bool, bool, error) {
	const selectSQL = `
		select id
		from patrol_runs
		where place_id = $1
		  and user_id = $2
		  and run_no = 0
		  and started_at >= $3
		  and started_at < $4
		order by created_at desc, id desc
		limit 1
		for update
	`
	var runID string
	err := tx.QueryRow(ctx, selectSQL, placeID, userID, scope.WindowStart, scope.WindowEnd).Scan(&runID)
	if err == nil {
		return runID, true, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, err
	}

	runID = newPatrolRunID()
	const insertSQL = `
		insert into patrol_runs (id, place_id, user_id, attendance_id, run_no, total_active_spots, status, started_at)
		values ($1,$2,$3,$4,$5,$6,'active',$7)
	`
	if _, err := tx.Exec(ctx, insertSQL, runID, placeID, userID, nil, 0, 0, scope.WindowStart); err != nil {
		return "", false, false, err
	}
	return runID, false, true, nil
}

func (r *Repository) resolveShiftScopeForScan(ctx context.Context, placeID string, referenceAt *string) (*patrolShiftScope, error) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return nil, err
	}
	refTime := time.Now().In(loc)
	if referenceAt != nil && strings.TrimSpace(*referenceAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*referenceAt))
		if err != nil {
			return nil, err
		}
		refTime = parsed.In(loc)
	}

	const sql = `
		with ref as (
			select (($2::timestamptz at time zone 'Asia/Jakarta')::time) as local_time
		)
		select s.id, s.start_time::text, s.end_time::text
		from shifts s
		cross join ref
		where s.place_id = $1
		  and s.is_active = true
		order by
			case
				when s.start_time <= s.end_time and ref.local_time >= s.start_time and ref.local_time < s.end_time then 0
				when s.start_time > s.end_time and (ref.local_time >= s.start_time or ref.local_time < s.end_time) then 0
				when ref.local_time >= s.start_time then 1
				else 2
			end asc,
			case
				when s.start_time <= s.end_time and ref.local_time >= s.start_time and ref.local_time < s.end_time then (ref.local_time - s.start_time)
				when s.start_time > s.end_time and (ref.local_time >= s.start_time or ref.local_time < s.end_time) then
					case
						when ref.local_time >= s.start_time then (ref.local_time - s.start_time)
						else (interval '24 hours' - (s.start_time - ref.local_time))
					end
				when ref.local_time >= s.start_time then (ref.local_time - s.start_time)
				else (s.start_time - ref.local_time)
			end asc,
			s.start_time asc,
			s.id asc
		limit 1
	`
	var shiftID, startRaw, endRaw string
	if err := r.db.QueryRow(ctx, sql, placeID, refTime.Format(time.RFC3339)).Scan(&shiftID, &startRaw, &endRaw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	parseClock := func(raw string) (int, int, int, error) {
		layouts := []string{"15:04:05", "15:04"}
		for _, layout := range layouts {
			if t, err := time.ParseInLocation(layout, strings.TrimSpace(raw), loc); err == nil {
				return t.Hour(), t.Minute(), t.Second(), nil
			}
		}
		return 0, 0, 0, fmt.Errorf("invalid shift time")
	}
	startH, startM, startS, err := parseClock(startRaw)
	if err != nil {
		return nil, err
	}
	endH, endM, endS, err := parseClock(endRaw)
	if err != nil {
		return nil, err
	}

	periodDate := time.Date(refTime.Year(), refTime.Month(), refTime.Day(), 0, 0, 0, 0, loc)
	windowStart := time.Date(periodDate.Year(), periodDate.Month(), periodDate.Day(), startH, startM, startS, 0, loc)
	windowEnd := time.Date(periodDate.Year(), periodDate.Month(), periodDate.Day(), endH, endM, endS, 0, loc)
	if !windowEnd.After(windowStart) {
		if refTime.Hour() < endH || (refTime.Hour() == endH && (refTime.Minute() < endM || (refTime.Minute() == endM && refTime.Second() < endS))) {
			periodDate = periodDate.AddDate(0, 0, -1)
		}
		windowStart = time.Date(periodDate.Year(), periodDate.Month(), periodDate.Day(), startH, startM, startS, 0, loc)
		windowEnd = time.Date(periodDate.Year(), periodDate.Month(), periodDate.Day(), endH, endM, endS, 0, loc).Add(24 * time.Hour)
	}

	return &patrolShiftScope{
		ShiftID:     shiftID,
		PeriodDate:  periodDate.Format("2006-01-02"),
		WindowStart: windowStart.UTC(),
		WindowEnd:   windowEnd.UTC(),
	}, nil
}

func (r *Repository) syncRunState(ctx context.Context, tx pgx.Tx, runID string) error {
	var totalActiveSpots int
	if err := tx.QueryRow(ctx, `select total_active_spots from patrol_runs where id = $1`, runID).Scan(&totalActiveSpots); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	isComplete, err := r.isRunComplete(ctx, tx, runID, totalActiveSpots)
	if err != nil {
		return err
	}
	if isComplete {
		return r.markRunCompleted(ctx, tx, runID, nil)
	}
	_, err = tx.Exec(ctx, `
		update patrol_runs
		set status = 'active',
		    completed_at = null,
		    updated_at = now()
		where id = $1
	`, runID)
	return err
}

func (r *Repository) countActiveRouteSpots(ctx context.Context, tx pgx.Tx, placeID string) (int, error) {
	const sql = `
		select count(*)::int
		from patrol_route_points
		where place_id = $1
		  and is_active = true
	`
	var total int
	err := tx.QueryRow(ctx, sql, placeID).Scan(&total)
	return total, err
}

func (r *Repository) countActiveRouteSpotsDirect(ctx context.Context, placeID string) (int, error) {
	const sql = `
		select count(*)::int
		from patrol_route_points
		where place_id = $1
		  and is_active = true
	`
	var total int
	err := r.db.QueryRow(ctx, sql, placeID).Scan(&total)
	return total, err
}

func (r *Repository) ensureActiveRun(ctx context.Context, tx pgx.Tx, placeID, userID string, totalActiveSpots int, startedAt *string) (string, int, bool, error) {
	activeRunID, activeRunNo, activeRunTotal, found, err := r.findActiveRun(ctx, tx, placeID, userID)
	if err != nil {
		return "", 0, false, err
	}
	if found {
		isComplete, err := r.isRunComplete(ctx, tx, activeRunID, activeRunTotal)
		if err != nil {
			return "", 0, false, err
		}
		if !isComplete {
			return activeRunID, activeRunNo, false, nil
		}
		if err := r.markRunCompleted(ctx, tx, activeRunID, startedAt); err != nil {
			return "", 0, false, err
		}
	}

	runNo, err := r.nextRunNo(ctx, tx, placeID, userID, nil)
	if err != nil {
		return "", 0, false, err
	}
	runID := newPatrolRunID()
	const sql = `
		insert into patrol_runs (id, place_id, user_id, attendance_id, run_no, total_active_spots, status, started_at)
		values ($1,$2,$3,$4,$5,$6,'active',coalesce($7::timestamptz, now()))
	`
	if _, err := tx.Exec(ctx, sql, runID, placeID, userID, nil, runNo, totalActiveSpots, startedAt); err != nil {
		return "", 0, false, err
	}
	return runID, runNo, true, nil
}

func (r *Repository) findActiveRun(ctx context.Context, tx pgx.Tx, placeID, userID string) (string, int, int, bool, error) {
	const sql = `
		select id, run_no, total_active_spots
		from patrol_runs
		where place_id = $1
		  and user_id = $2
		  and status = 'active'
		order by run_no desc, created_at desc, id desc
		limit 1
		for update
	`
	var runID string
	var runNo, totalActiveSpots int
	err := tx.QueryRow(ctx, sql, placeID, userID).Scan(&runID, &runNo, &totalActiveSpots)
	switch {
	case err == nil:
		return runID, runNo, totalActiveSpots, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return "", 0, 0, false, nil
	default:
		return "", 0, 0, false, err
	}
}

func (r *Repository) nextRunNo(ctx context.Context, tx pgx.Tx, placeID, userID string, attendanceID *string) (int, error) {
	const sql = `
		select coalesce(max(run_no), 0)::int + 1
		from patrol_runs
		where place_id = $1
		  and user_id = $2
	`
	var runNo int
	err := tx.QueryRow(ctx, sql, placeID, userID).Scan(&runNo)
	return runNo, err
}

func (r *Repository) listActiveRoundNos(ctx context.Context, tx pgx.Tx, placeID string) ([]int, error) {
	rows, err := tx.Query(ctx, `
		select round_no
		from patrol_round_masters
		where place_id = $1
		  and is_active = true
		order by round_no asc, id asc
	`, placeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int, 0)
	for rows.Next() {
		var roundNo int
		if err := rows.Scan(&roundNo); err != nil {
			return nil, err
		}
		out = append(out, roundNo)
	}
	return out, rows.Err()
}

func (r *Repository) getOrCreateRunByNo(ctx context.Context, tx pgx.Tx, placeID, userID string, runNo, totalActiveSpots int, startedAt *string) (string, bool, bool, error) {
	const selectSQL = `
		select id
		from patrol_runs
		where place_id = $1
		  and user_id = $2
		  and run_no = $3
		order by created_at desc, id desc
		limit 1
		for update
	`
	var runID string
	err := tx.QueryRow(ctx, selectSQL, placeID, userID, runNo).Scan(&runID)
	if err == nil {
		return runID, true, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, err
	}

	runID = newPatrolRunID()
	const insertSQL = `
		insert into patrol_runs (id, place_id, user_id, attendance_id, run_no, total_active_spots, status, started_at)
		values ($1,$2,$3,$4,$5,$6,'active',coalesce($7::timestamptz, now()))
	`
	if _, err := tx.Exec(ctx, insertSQL, runID, placeID, userID, nil, runNo, totalActiveSpots, startedAt); err != nil {
		return "", false, false, err
	}
	return runID, false, true, nil
}

func (r *Repository) runAlreadyHasSpot(ctx context.Context, tx pgx.Tx, runID, spotID string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `
		select exists(
			select 1
			from patrol_scans
			where patrol_run_id = $1
			  and spot_id = $2
		)
	`, runID, spotID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) hasRoundMasters(ctx context.Context, tx pgx.Tx, placeID string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(
		ctx,
		`select exists(select 1 from patrol_round_masters where place_id = $1 and is_active = true)`,
		placeID,
	).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) resolveShiftWindow(ctx context.Context, shiftID, date string) (time.Time, time.Time, error) {
	var startRaw, endRaw string
	if err := r.db.QueryRow(
		ctx,
		`select start_time::text, end_time::text from shifts where id = $1 and is_active = true`,
		shiftID,
	).Scan(&startRaw, &endRaw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, time.Time{}, ErrForeignKey
		}
		return time.Time{}, time.Time{}, err
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	parseClock := func(raw string) (int, int, int, error) {
		layouts := []string{"15:04:05", "15:04"}
		for _, layout := range layouts {
			if t, err := time.ParseInLocation(layout, strings.TrimSpace(raw), loc); err == nil {
				return t.Hour(), t.Minute(), t.Second(), nil
			}
		}
		return 0, 0, 0, fmt.Errorf("invalid shift time")
	}

	startH, startM, startS, err := parseClock(startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endH, endM, endS, err := parseClock(endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	baseDate, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	startAt := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), startH, startM, startS, 0, loc)
	endAt := time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), endH, endM, endS, 0, loc)
	if !endAt.After(startAt) {
		endAt = endAt.Add(24 * time.Hour)
	}
	return startAt.UTC(), endAt.UTC(), nil
}

func (r *Repository) ensureRoundNoAllowed(ctx context.Context, tx pgx.Tx, placeID string, roundNo int) error {
	var exists bool
	if err := tx.QueryRow(
		ctx,
		`select exists(select 1 from patrol_round_masters where place_id = $1 and round_no = $2 and is_active = true)`,
		placeID,
		roundNo,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrPatrolRoundMasterNotFound
	}
	return nil
}

func (r *Repository) getRunPlaceID(ctx context.Context, id string) (string, error) {
	var placeID string
	if err := r.db.QueryRow(ctx, `select place_id from patrol_runs where id = $1`, id).Scan(&placeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrPatrolRunNotFound
		}
		return "", err
	}
	return placeID, nil
}

func (r *Repository) isRunComplete(ctx context.Context, tx pgx.Tx, runID string, totalActiveSpots int) (bool, error) {
	if totalActiveSpots <= 0 {
		return false, nil
	}
	const sql = `
		select count(distinct ps.spot_id)::int
		from patrol_scans ps
		join patrol_runs pr
		  on pr.id = ps.patrol_run_id
		join patrol_route_points prp
		  on prp.place_id = pr.place_id
		 and prp.spot_id = ps.spot_id
		 and prp.is_active = true
		where ps.patrol_run_id = $1
	`
	var scannedSpots int
	if err := tx.QueryRow(ctx, sql, runID).Scan(&scannedSpots); err != nil {
		return false, err
	}
	return scannedSpots >= totalActiveSpots, nil
}

func (r *Repository) syncRunCompletion(ctx context.Context, tx pgx.Tx, runID string, totalActiveSpots int, submitAt, scannedAt *string) (bool, error) {
	isComplete, err := r.isRunComplete(ctx, tx, runID, totalActiveSpots)
	if err != nil {
		return false, err
	}
	if !isComplete {
		return false, nil
	}
	if err := r.markRunCompleted(ctx, tx, runID, firstNonNil(submitAt, scannedAt)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) markRunCompleted(ctx context.Context, tx pgx.Tx, runID string, completedAt *string) error {
	const sql = `
		update patrol_runs
		set status = 'completed',
		    completed_at = coalesce(completed_at, coalesce($2::timestamptz, now())),
		    updated_at = now()
		where id = $1
	`
	_, err := tx.Exec(ctx, sql, runID, completedAt)
	return err
}

func (r *Repository) resolveAttendanceID(ctx context.Context, placeID, userID string, provided, scannedAt *string) (*string, error) {
	if provided != nil && strings.TrimSpace(*provided) != "" {
		value := strings.TrimSpace(*provided)
		return &value, nil
	}

	const sql = `
		select a.id
		from attendances a
		where a.place_id = $1
		  and a.user_id = $2
		  and a.check_in_at is not null
		  and a.check_in_at <= coalesce($3::timestamptz, now())
		  and (a.check_out_at is null or a.check_out_at >= coalesce($3::timestamptz, now()))
		order by a.check_in_at desc, a.created_at desc, a.id desc
		limit 1
	`
	var attendanceID string
	err := r.db.QueryRow(ctx, sql, placeID, userID, scannedAt).Scan(&attendanceID)
	switch {
	case err == nil:
		return &attendanceID, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	default:
		return nil, err
	}
}

func (r *Repository) GetProgress(ctx context.Context, actorUserID, actorRole, attendanceID string) (*PatrolProgress, error) {
	const attendanceSQL = `
		select a.id, a.place_id, a.user_id, a.shift_id, a.attendance_date::text, a.check_in_at, a.check_out_at
		from attendances a
		where a.id = $1
		  and (
		    $2 = true
		    or a.place_id in (
		      select distinct upr.place_id
		      from user_place_roles upr
		      join places p on p.id = upr.place_id
		      where upr.user_id = $3 and upr.is_active = true and p.deleted_at is null
		    )
		  )
		limit 1
	`
	progress := &PatrolProgress{}
	err := r.db.QueryRow(ctx, attendanceSQL, attendanceID, auth.IsGlobalAdminRole(actorRole), actorUserID).Scan(
		&progress.AttendanceID,
		&progress.PlaceID,
		&progress.UserID,
		&progress.ShiftID,
		&progress.AttendanceDate,
		&progress.CheckInAt,
		&progress.CheckOutAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProgressNotFound
		}
		return nil, err
	}

	const totalsSQL = `
		with route_spots as (
			select prp.spot_id
			from patrol_route_points prp
			where prp.place_id = $1
			  and prp.is_active = true
		),
		scan_rows as (
			select ps.spot_id, ps.patrol_run_id
			from patrol_scans ps
			where ps.attendance_id = $2
		)
		select
			(select count(*)::int from route_spots) as total_route_spots,
			(select count(distinct sr.spot_id)::int from scan_rows sr join route_spots rs on rs.spot_id = sr.spot_id) as patrolled_spots,
			(select count(*)::int from scan_rows) as total_scans,
			(select count(distinct patrol_run_id)::int from scan_rows) as total_patrol_runs
	`
	if err := r.db.QueryRow(ctx, totalsSQL, progress.PlaceID, progress.AttendanceID).Scan(
		&progress.TotalRouteSpots,
		&progress.PatrolledSpots,
		&progress.TotalScans,
		&progress.TotalPatrolRuns,
	); err != nil {
		return nil, err
	}
	progress.UnpatrolledSpots = progress.TotalRouteSpots - progress.PatrolledSpots
	if progress.UnpatrolledSpots < 0 {
		progress.UnpatrolledSpots = 0
	}

	const spotsSQL = `
		select
			prp.spot_id,
			s.spot_code,
			s.spot_name,
			prp.seq,
			coalesce(count(ps.id), 0)::int as scan_count,
			max(ps.scanned_at) as last_scanned_at,
			(
				array_remove(
					array_agg(ps.patrol_run_id order by ps.scanned_at desc, ps.id desc),
					null
				)
			)[1] as last_patrol_run_id
		from patrol_route_points prp
		join spots s on s.id = prp.spot_id
		left join patrol_scans ps
		  on ps.spot_id = prp.spot_id
		 and ps.attendance_id = $2
		where prp.place_id = $1
		  and prp.is_active = true
		group by prp.spot_id, s.spot_code, s.spot_name, prp.seq
		order by prp.seq asc, s.spot_code asc, prp.spot_id asc
	`
	rows, err := r.db.Query(ctx, spotsSQL, progress.PlaceID, progress.AttendanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	progress.Spots = make([]PatrolProgressSpot, 0)
	for rows.Next() {
		var item PatrolProgressSpot
		if err := rows.Scan(&item.SpotID, &item.SpotCode, &item.SpotName, &item.Seq, &item.ScanCount, &item.LastScannedAt, &item.LastPatrolRunID); err != nil {
			return nil, err
		}
		item.IsPatrolled = item.ScanCount > 0
		progress.Spots = append(progress.Spots, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return progress, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == code
}

func firstNonNil(values ...*string) *string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return value
		}
	}
	return nil
}

func newPatrolRunID() string {
	var raw [16]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		panic(err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	)
}

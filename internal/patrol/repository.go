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
	ErrRoutePointNotFound = errors.New("route point not found")
	ErrPatrolScanNotFound = errors.New("patrol scan not found")
	ErrProgressNotFound   = errors.New("patrol progress not found")
	ErrAlreadyExists      = errors.New("already exists")
	ErrForeignKey         = errors.New("related row not found")
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

type PatrolScan struct {
	ID           string    `json:"id"`
	PlaceID      string    `json:"place_id"`
	UserID       string    `json:"user_id"`
	SpotID       string    `json:"spot_id"`
	AttendanceID *string   `json:"attendance_id"`
	PatrolRunID  string    `json:"patrol_run_id"`
	ScannedAt    time.Time `json:"scanned_at"`
	SubmitAt     time.Time `json:"submit_at"`
	PhotoURL     *string   `json:"photo_url"`
	Note         *string   `json:"note"`
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

func (r *Repository) ListScans(ctx context.Context, actorUserID, actorRole, placeID, patrolRunID, userID, attendanceID string, query listquery.Query) (listquery.Response[PatrolScan], error) {
	sortColumn := map[string]string{"scannedAt": "scanned_at", "submitAt": "submit_at", "placeId": "place_id", "userId": "user_id", "spotId": "spot_id", "patrolRunId": "patrol_run_id"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "scanned_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `select id, place_id, user_id, spot_id, attendance_id, patrol_run_id, scanned_at, submit_at, photo_url, note, count(*) over()::int as total_count from patrol_scans where place_id = $1`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	if patrolRunID != "" {
		args = append(args, patrolRunID)
		sql += fmt.Sprintf(" and patrol_run_id = $%d", len(args))
	}
	if userID != "" {
		args = append(args, userID)
		sql += fmt.Sprintf(" and user_id = $%d", len(args))
	}
	if attendanceID != "" {
		args = append(args, attendanceID)
		sql += fmt.Sprintf(" and attendance_id = $%d", len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[PatrolScan]{}, err
	}
	defer rows.Close()
	data := make([]PatrolScan, 0)
	total := 0
	for rows.Next() {
		var item PatrolScan
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.SpotID, &item.AttendanceID, &item.PatrolRunID, &item.ScannedAt, &item.SubmitAt, &item.PhotoURL, &item.Note, &total); err != nil {
			return listquery.Response[PatrolScan]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) CreateScan(ctx context.Context, placeID, userID, spotID string, attendanceID *string, scannedAt, submitAt, photoURL, note *string) (*CreateScanResult, error) {
	resolvedAttendanceID, err := r.resolveAttendanceID(ctx, placeID, userID, attendanceID, scannedAt)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	totalActiveSpots, err := r.countActiveRouteSpots(ctx, tx, placeID)
	if err != nil {
		return nil, err
	}

	runID, runNo, isNewRun, err := r.ensureActiveRun(ctx, tx, placeID, userID, resolvedAttendanceID, totalActiveSpots, scannedAt)
	if err != nil {
		return nil, err
	}

	const sql = `insert into patrol_scans (place_id, user_id, spot_id, attendance_id, patrol_run_id, scanned_at, submit_at, photo_url, note) values ($1,$2,$3,$4,$5,coalesce($6::timestamptz, now()),coalesce($7::timestamptz, now()),$8,$9) returning id`
	var id string
	err = tx.QueryRow(ctx, sql, placeID, userID, spotID, resolvedAttendanceID, runID, scannedAt, submitAt, photoURL, note).Scan(&id)
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

	runCompleted, err := r.syncRunCompletion(ctx, tx, runID, totalActiveSpots, submitAt, scannedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateScanResult{
		ID:                 id,
		PatrolRunID:        runID,
		PatrolRunNo:        runNo,
		IsNewPatrolRun:     isNewRun,
		PatrolRunCompleted: runCompleted,
	}, nil
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

func (r *Repository) ensureActiveRun(ctx context.Context, tx pgx.Tx, placeID, userID string, attendanceID *string, totalActiveSpots int, startedAt *string) (string, int, bool, error) {
	activeRunID, activeRunNo, activeRunTotal, found, err := r.findActiveRun(ctx, tx, placeID, userID, attendanceID)
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

	runNo, err := r.nextRunNo(ctx, tx, placeID, userID, attendanceID)
	if err != nil {
		return "", 0, false, err
	}
	runID := newPatrolRunID()
	const sql = `
		insert into patrol_runs (id, place_id, user_id, attendance_id, run_no, total_active_spots, status, started_at)
		values ($1,$2,$3,$4,$5,$6,'active',coalesce($7::timestamptz, now()))
	`
	if _, err := tx.Exec(ctx, sql, runID, placeID, userID, attendanceID, runNo, totalActiveSpots, startedAt); err != nil {
		return "", 0, false, err
	}
	return runID, runNo, true, nil
}

func (r *Repository) findActiveRun(ctx context.Context, tx pgx.Tx, placeID, userID string, attendanceID *string) (string, int, int, bool, error) {
	const sql = `
		select id, run_no, total_active_spots
		from patrol_runs
		where place_id = $1
		  and user_id = $2
		  and attendance_id is not distinct from $3::uuid
		  and status = 'active'
		order by run_no desc, created_at desc, id desc
		limit 1
		for update
	`
	var runID string
	var runNo, totalActiveSpots int
	err := tx.QueryRow(ctx, sql, placeID, userID, attendanceID).Scan(&runID, &runNo, &totalActiveSpots)
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
		  and attendance_id is not distinct from $3::uuid
	`
	var runNo int
	err := tx.QueryRow(ctx, sql, placeID, userID, attendanceID).Scan(&runNo)
	return runNo, err
}

func (r *Repository) isRunComplete(ctx context.Context, tx pgx.Tx, runID string, totalActiveSpots int) (bool, error) {
	if totalActiveSpots <= 0 {
		return false, nil
	}
	const sql = `
		select count(distinct ps.spot_id)::int
		from patrol_scans ps
		where ps.patrol_run_id = $1
		  and exists (
		    select 1
		    from patrol_route_points prp
		    where prp.place_id = ps.place_id
		      and prp.spot_id = ps.spot_id
		  )
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

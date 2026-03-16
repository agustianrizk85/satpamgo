package patrol

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
	ErrRoutePointNotFound = errors.New("route point not found")
	ErrPatrolScanNotFound = errors.New("patrol scan not found")
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
	ID          string    `json:"id"`
	PlaceID     string    `json:"place_id"`
	UserID      string    `json:"user_id"`
	SpotID      string    `json:"spot_id"`
	PatrolRunID string    `json:"patrol_run_id"`
	ScannedAt   time.Time `json:"scanned_at"`
	SubmitAt    time.Time `json:"submit_at"`
	PhotoURL    *string   `json:"photo_url"`
	Note        *string   `json:"note"`
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

func (r *Repository) ListScans(ctx context.Context, actorUserID, actorRole, placeID, patrolRunID, userID string, query listquery.Query) (listquery.Response[PatrolScan], error) {
	sortColumn := map[string]string{"scannedAt": "scanned_at", "submitAt": "submit_at", "placeId": "place_id", "userId": "user_id", "spotId": "spot_id", "patrolRunId": "patrol_run_id"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "scanned_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `select id, place_id, user_id, spot_id, patrol_run_id, scanned_at, submit_at, photo_url, note, count(*) over()::int as total_count from patrol_scans where place_id = $1`
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
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.SpotID, &item.PatrolRunID, &item.ScannedAt, &item.SubmitAt, &item.PhotoURL, &item.Note, &total); err != nil {
			return listquery.Response[PatrolScan]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) CreateScan(ctx context.Context, placeID, userID, spotID, patrolRunID string, scannedAt, submitAt, photoURL, note *string) (string, error) {
	const sql = `insert into patrol_scans (place_id, user_id, spot_id, patrol_run_id, scanned_at, submit_at, photo_url, note) values ($1,$2,$3,$4,coalesce($5::timestamptz, now()),coalesce($6::timestamptz, now()),$7,$8) returning id`
	var id string
	err := r.db.QueryRow(ctx, sql, placeID, userID, spotID, patrolRunID, scannedAt, submitAt, photoURL, note).Scan(&id)
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

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == code
}

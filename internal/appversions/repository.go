package appversions

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
	ErrNotFound       = errors.New("app version not found")
	ErrAlready        = errors.New("app version already exists")
	ErrForeignKey     = errors.New("related place/user not found")
	ErrMasterNotFound = errors.New("app version master not found")
	ErrMasterAlready  = errors.New("app version master already exists")
)

type Repository struct{ db *pgxpool.Pool }

type AppVersion struct {
	ID          string    `json:"id"`
	PlaceID     string    `json:"place_id"`
	UserID      string    `json:"user_id"`
	VersionName string    `json:"version_name"`
	DownloadURL string    `json:"download_url"`
	IsMandatory bool      `json:"is_mandatory"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type VersionCheckResult struct {
	PlaceID        string  `json:"place_id"`
	UserID         string  `json:"user_id"`
	CurrentVersion string  `json:"current_version"`
	LatestVersion  *string `json:"latest_version"`
	ShouldDownload bool    `json:"should_download"`
	IsMandatory    bool    `json:"is_mandatory"`
	DownloadURL    *string `json:"download_url"`
}

type AppVersionMaster struct {
	ID          string    `json:"id"`
	VersionName string    `json:"version_name"`
	DownloadURL string    `json:"download_url"`
	IsMandatory bool      `json:"is_mandatory"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) ListMasters(ctx context.Context, query listquery.Query, isActive *bool) (listquery.Response[AppVersionMaster], error) {
	sortColumn := map[string]string{
		"versionName": "version_name",
		"createdAt":   "created_at",
		"updatedAt":   "updated_at",
		"isActive":    "is_active",
		"isMandatory": "is_mandatory",
	}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `select id, version_name, download_url, is_mandatory, is_active, created_at, updated_at, count(*) over()::int as total_count from app_version_masters where true`
	args := make([]any, 0)
	if isActive != nil {
		args = append(args, *isActive)
		sql += fmt.Sprintf(" and is_active = $%d", len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[AppVersionMaster]{}, err
	}
	defer rows.Close()

	data := make([]AppVersionMaster, 0)
	total := 0
	for rows.Next() {
		var item AppVersionMaster
		if err := rows.Scan(&item.ID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[AppVersionMaster]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) GetMaster(ctx context.Context, id string) (*AppVersionMaster, error) {
	const sql = `select id, version_name, download_url, is_mandatory, is_active, created_at, updated_at from app_version_masters where id = $1`
	var item AppVersionMaster
	err := r.db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMasterNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CreateMaster(ctx context.Context, versionName, downloadURL string, isMandatory, isActive bool) (*AppVersionMaster, error) {
	const sql = `
		insert into app_version_masters (version_name, download_url, is_mandatory, is_active)
		values ($1,$2,$3,$4)
		returning id, version_name, download_url, is_mandatory, is_active, created_at, updated_at
	`
	var item AppVersionMaster
	err := r.db.QueryRow(ctx, sql, versionName, downloadURL, isMandatory, isActive).Scan(
		&item.ID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if isPgCode(err, "23505") {
			return nil, ErrMasterAlready
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateMaster(ctx context.Context, id string, versionName, downloadURL *string, isMandatory, isActive *bool) (*AppVersionMaster, error) {
	setParts := make([]string, 0)
	args := make([]any, 0)
	argPos := 1

	if versionName != nil {
		setParts = append(setParts, fmt.Sprintf("version_name = $%d", argPos))
		args = append(args, strings.TrimSpace(*versionName))
		argPos++
	}
	if downloadURL != nil {
		setParts = append(setParts, fmt.Sprintf("download_url = $%d", argPos))
		args = append(args, strings.TrimSpace(*downloadURL))
		argPos++
	}
	if isMandatory != nil {
		setParts = append(setParts, fmt.Sprintf("is_mandatory = $%d", argPos))
		args = append(args, *isMandatory)
		argPos++
	}
	if isActive != nil {
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *isActive)
		argPos++
	}
	if len(setParts) == 0 {
		return r.GetMaster(ctx, id)
	}

	args = append(args, id)
	sql := fmt.Sprintf(`
		update app_version_masters
		set %s,
		    updated_at = now()
		where id = $%d
		returning id, version_name, download_url, is_mandatory, is_active, created_at, updated_at
	`, strings.Join(setParts, ", "), argPos)

	var item AppVersionMaster
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrMasterNotFound
		case isPgCode(err, "23505"):
			return nil, ErrMasterAlready
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) DeleteMaster(ctx context.Context, id string) (string, error) {
	const sql = `delete from app_version_masters where id = $1 returning id`
	var out string
	if err := r.db.QueryRow(ctx, sql, id).Scan(&out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrMasterNotFound
		}
		return "", err
	}
	return out, nil
}

func (r *Repository) List(ctx context.Context, actorUserID, actorRole, placeID, userID string, isActive *bool, query listquery.Query) (listquery.Response[AppVersion], error) {
	sortColumn := map[string]string{
		"versionName": "version_name",
		"createdAt":   "created_at",
		"updatedAt":   "updated_at",
		"isActive":    "is_active",
		"isMandatory": "is_mandatory",
		"userId":      "user_id",
	}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `select id, place_id, user_id, version_name, download_url, is_mandatory, is_active, created_at, updated_at, count(*) over()::int as total_count from app_versions where place_id = $1`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	if userID != "" {
		args = append(args, userID)
		sql += fmt.Sprintf(" and user_id = $%d", len(args))
	}
	if isActive != nil {
		args = append(args, *isActive)
		sql += fmt.Sprintf(" and is_active = $%d", len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[AppVersion]{}, err
	}
	defer rows.Close()

	data := make([]AppVersion, 0)
	total := 0
	for rows.Next() {
		var item AppVersion
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.UserID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[AppVersion]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) Get(ctx context.Context, actorUserID, actorRole, id string) (*AppVersion, error) {
	sql := `select id, place_id, user_id, version_name, download_url, is_mandatory, is_active, created_at, updated_at from app_versions where id = $1`
	args := []any{id}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	var item AppVersion
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.UserID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) Create(ctx context.Context, placeID, userID, versionName, downloadURL string, isMandatory, isActive bool) (*AppVersion, error) {
	const sql = `
		insert into app_versions (place_id, user_id, version_name, download_url, is_mandatory, is_active)
		values ($1,$2,$3,$4,$5,$6)
		returning id, place_id, user_id, version_name, download_url, is_mandatory, is_active, created_at, updated_at
	`
	var item AppVersion
	err := r.db.QueryRow(ctx, sql, placeID, userID, versionName, downloadURL, isMandatory, isActive).Scan(
		&item.ID, &item.PlaceID, &item.UserID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return nil, ErrAlready
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) Update(ctx context.Context, id string, versionName, downloadURL *string, isMandatory, isActive *bool) (*AppVersion, error) {
	setParts := make([]string, 0)
	args := make([]any, 0)
	argPos := 1

	if versionName != nil {
		setParts = append(setParts, fmt.Sprintf("version_name = $%d", argPos))
		args = append(args, strings.TrimSpace(*versionName))
		argPos++
	}
	if downloadURL != nil {
		setParts = append(setParts, fmt.Sprintf("download_url = $%d", argPos))
		args = append(args, strings.TrimSpace(*downloadURL))
		argPos++
	}
	if isMandatory != nil {
		setParts = append(setParts, fmt.Sprintf("is_mandatory = $%d", argPos))
		args = append(args, *isMandatory)
		argPos++
	}
	if isActive != nil {
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *isActive)
		argPos++
	}
	if len(setParts) == 0 {
		return nil, ErrNotFound
	}

	args = append(args, id)
	sql := fmt.Sprintf(`
		update app_versions
		set %s,
		    updated_at = now()
		where id = $%d
		returning id, place_id, user_id, version_name, download_url, is_mandatory, is_active, created_at, updated_at
	`, strings.Join(setParts, ", "), argPos)

	var item AppVersion
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID, &item.PlaceID, &item.UserID, &item.VersionName, &item.DownloadURL, &item.IsMandatory, &item.IsActive, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrNotFound
		case isPgCode(err, "23505"):
			return nil, ErrAlready
		case isPgCode(err, "23503"):
			return nil, ErrForeignKey
		default:
			return nil, err
		}
	}
	return &item, nil
}

func (r *Repository) Delete(ctx context.Context, id string) (string, error) {
	const sql = `delete from app_versions where id = $1 returning id`
	var out string
	if err := r.db.QueryRow(ctx, sql, id).Scan(&out); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return out, nil
}

func (r *Repository) Check(ctx context.Context, actorUserID, actorRole, placeID, userID, currentVersion string) (*VersionCheckResult, error) {
	sql := `
		select place_id, user_id, version_name, download_url, is_mandatory
		from app_versions
		where place_id = $1
		  and user_id = $2
		  and is_active = true
	`
	args := []any{placeID, userID}
	if !auth.IsGlobalAdminRole(actorRole) && strings.TrimSpace(actorUserID) != strings.TrimSpace(userID) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (
			select distinct upr.place_id
			from user_place_roles upr join places p on p.id = upr.place_id
			where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null
		)`, len(args))
	}
	sql += ` order by created_at desc, updated_at desc, id desc limit 1`

	var latestVersion, downloadURL string
	var isMandatory bool
	err := r.db.QueryRow(ctx, sql, args...).Scan(&placeID, &userID, &latestVersion, &downloadURL, &isMandatory)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &VersionCheckResult{
				PlaceID:        placeID,
				UserID:         userID,
				CurrentVersion: currentVersion,
				LatestVersion:  nil,
				ShouldDownload: false,
				IsMandatory:    false,
				DownloadURL:    nil,
			}, nil
		}
		return nil, err
	}

	shouldDownload := strings.TrimSpace(currentVersion) == "" || strings.TrimSpace(currentVersion) != strings.TrimSpace(latestVersion)
	return &VersionCheckResult{
		PlaceID:        placeID,
		UserID:         userID,
		CurrentVersion: currentVersion,
		LatestVersion:  &latestVersion,
		ShouldDownload: shouldDownload,
		IsMandatory:    isMandatory,
		DownloadURL:    &downloadURL,
	}, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == code
}

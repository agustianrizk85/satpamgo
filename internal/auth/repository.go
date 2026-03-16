package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	GlobalRoleSuperUser = "SUPER_USER"
	PlaceRoleAdmin      = "PLACE_ADMIN"
)

type Repository struct {
	db *pgxpool.Pool
}

type LoginUser struct {
	ID         string
	FullName   string
	Username   string
	Status     string
	RoleCode   string
	RoleName   string
	DefaultPID *string
}

type PlaceAccess struct {
	PlaceID   string `json:"placeId"`
	PlaceCode string `json:"placeCode"`
	PlaceName string `json:"placeName"`
	RoleCode  string `json:"roleCode"`
	RoleName  string `json:"roleName"`
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Login(ctx context.Context, username, password string) (*LoginUser, error) {
	const query = `
		select
			u.id,
			u.full_name,
			u.username,
			u.status,
			r.code as role_code,
			r.name as role_name
		from users u
		join roles r on r.id = u.role_id
		where u.username = $1
		  and u.deleted_at is null
		  and u.password_hash = crypt($2, u.password_hash)
		limit 1
	`

	var user LoginUser
	err := r.db.QueryRow(ctx, query, username, password).Scan(
		&user.ID,
		&user.FullName,
		&user.Username,
		&user.Status,
		&user.RoleCode,
		&user.RoleName,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *Repository) FindUserByID(ctx context.Context, userID string) (*LoginUser, error) {
	const query = `
		select
			u.id,
			u.full_name,
			u.username,
			u.status,
			r.code as role_code,
			r.name as role_name
		from users u
		join roles r on r.id = u.role_id
		where u.id = $1
		  and u.deleted_at is null
		limit 1
	`

	var user LoginUser
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.FullName,
		&user.Username,
		&user.Status,
		&user.RoleCode,
		&user.RoleName,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *Repository) ListUserPlaceAccess(ctx context.Context, userID string) ([]PlaceAccess, error) {
	const query = `
		select
			upr.place_id,
			p.place_code,
			p.place_name,
			r.code as role_code,
			r.name as role_name
		from user_place_roles upr
		join roles r on r.id = upr.role_id
		join places p on p.id = upr.place_id
		where upr.user_id = $1
		  and upr.is_active = true
		  and p.deleted_at is null
		order by p.place_name asc, r.name asc
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]PlaceAccess, 0)
	for rows.Next() {
		var item PlaceAccess
		if err := rows.Scan(&item.PlaceID, &item.PlaceCode, &item.PlaceName, &item.RoleCode, &item.RoleName); err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, rows.Err()
}

func IsGlobalAdminRole(role string) bool {
	switch role {
	case "SUPER_USER", "SUPER_ADMIN", "ADMIN":
		return true
	default:
		return false
	}
}

func IsSuperUserRole(role string) bool {
	return role == GlobalRoleSuperUser || role == "SUPER_ADMIN"
}

func (r *Repository) HasAnyPlaceRole(ctx context.Context, userID string, roleCodes []string) (bool, error) {
	query := `
		select 1
		from user_place_roles upr
		join roles r on r.id = upr.role_id
		join places p on p.id = upr.place_id
		where upr.user_id = $1
		  and upr.is_active = true
		  and p.deleted_at is null
	`
	args := []any{userID}
	if len(roleCodes) > 0 {
		query += " and r.code = any($2::text[])"
		args = append(args, roleCodes)
	}
	query += " limit 1"

	var ok int
	err := r.db.QueryRow(ctx, query, args...).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) ListUserPlaceIDs(ctx context.Context, userID string, roleCodes []string) ([]string, error) {
	query := `
		select distinct upr.place_id
		from user_place_roles upr
		join roles r on r.id = upr.role_id
		join places p on p.id = upr.place_id
		where upr.user_id = $1
		  and upr.is_active = true
		  and p.deleted_at is null
	`
	args := []any{userID}
	if len(roleCodes) > 0 {
		query += " and r.code = any($2::text[])"
		args = append(args, roleCodes)
	}
	query += " order by upr.place_id"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var placeID string
		if err := rows.Scan(&placeID); err != nil {
			return nil, err
		}
		out = append(out, placeID)
	}

	return out, rows.Err()
}

func (r *Repository) HasPlaceAccess(ctx context.Context, userID, placeID string, roleCodes []string) (bool, error) {
	query := `
		select 1
		from user_place_roles upr
		join roles r on r.id = upr.role_id
		join places p on p.id = upr.place_id
		where upr.user_id = $1
		  and upr.place_id = $2
		  and upr.is_active = true
		  and p.deleted_at is null
	`
	args := []any{userID, placeID}
	if len(roleCodes) > 0 {
		query += " and r.code = any($3::text[])"
		args = append(args, roleCodes)
	}
	query += " limit 1"

	var ok int
	err := r.db.QueryRow(ctx, query, args...).Scan(&ok)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

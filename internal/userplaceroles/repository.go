package userplaceroles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
)

var (
	ErrNotFound      = errors.New("user place role not found")
	ErrAlreadyExists = errors.New("user place role already exists")
	ErrForeignKey    = errors.New("foreign key not found")
)

type Repository struct {
	db *pgxpool.Pool
}

type UserPlaceRole struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	PlaceID   string    `json:"place_id"`
	RoleID    string    `json:"role_id"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	Query       listquery.Query
}

type UpsertInput struct {
	UserID   string
	PlaceID  string
	RoleID   string
	IsActive bool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[UserPlaceRole], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"userId":    "user_id",
		"username":  "user_id",
		"fullName":  "user_id",
		"placeId":   "place_id",
		"placeCode": "place_id",
		"placeName": "place_id",
		"roleId":    "role_id",
		"roleCode":  "role_id",
		"roleName":  "role_id",
		"isActive":  "is_active",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := `
		select id, user_id, place_id, role_id, is_active, created_at, updated_at, count(*) over()::int as total_count
		from user_place_roles
	`
	args := []any{params.Query.PageSize, params.Query.Offset}
	if !auth.IsSuperUserRole(params.ActorRole) {
		sql += `
		where place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join roles r on r.id = upr.role_id
			join places p on p.id = upr.place_id
			where upr.user_id = $1
			  and upr.is_active = true
			  and r.code = $2
			  and p.deleted_at is null
		)
		`
		args = []any{params.ActorUserID, auth.PlaceRoleAdmin, params.Query.PageSize, params.Query.Offset}
	}

	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[UserPlaceRole]{}, err
	}
	defer rows.Close()

	data := make([]UserPlaceRole, 0)
	total := 0
	for rows.Next() {
		var item UserPlaceRole
		if err := rows.Scan(&item.ID, &item.UserID, &item.PlaceID, &item.RoleID, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[UserPlaceRole]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, params.Query, total), rows.Err()
}

func (r *Repository) Upsert(ctx context.Context, input UpsertInput) (string, error) {
	const sql = `
		insert into user_place_roles (user_id, place_id, role_id, is_active)
		values ($1, $2, $3, $4)
		on conflict (user_id, place_id)
		do update set role_id = excluded.role_id, is_active = excluded.is_active, updated_at = now()
		returning id
	`
	var id string
	err := r.db.QueryRow(ctx, sql, input.UserID, input.PlaceID, input.RoleID, input.IsActive).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23503"):
			return "", ErrForeignKey
		case isPgCode(err, "23505"):
			return "", ErrAlreadyExists
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

package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/listquery"
)

const placeRoleAdmin = "PLACE_ADMIN"

var (
	ErrUserNotFound         = errors.New("user not found")
	ErrUsernameExists       = errors.New("username already exists")
	ErrUserRoleNotFound     = errors.New("role not found")
	ErrUserNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct {
	db *pgxpool.Pool
}

type User struct {
	ID        string    `json:"id"`
	RoleID    string    `json:"role_id"`
	RoleCode  string    `json:"role_code"`
	FullName  string    `json:"full_name"`
	Username  string    `json:"username"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	RoleID   string
	FullName string
	Username string
	Password string
	Status   string
}

type CreateWithPlaceInput struct {
	RoleID      string
	FullName    string
	Username    string
	Password    string
	Status      string
	PlaceID     string
	PlaceRoleID string
}

type UpdateInput struct {
	RoleID   *string
	FullName *string
	Username *string
	Password *string
	Status   *string
}

type ListUsersParams struct {
	ActorUserID string
	ActorRole   string
	Query       listquery.Query
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListUsers(ctx context.Context, params ListUsersParams) (listquery.Response[User], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"fullName":  "full_name",
		"username":  "username",
		"status":    "status",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}

	sortOrder := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortOrder = "asc"
	}

	offset := params.Query.Offset

	baseSelect := `
		select
			u.id,
			u.role_id,
			r.code as role_code,
			u.full_name,
			u.username,
			u.status,
			u.created_at,
			u.updated_at,
			count(*) over()::int as total_count
		from users u
		join roles r on r.id = u.role_id
		where u.deleted_at is null
	`

	query := baseSelect + fmt.Sprintf(`
		order by u.%s %s, u.id asc
		limit $1 offset $2
	`, sortColumn, sortOrder)
	args := []any{params.Query.PageSize, offset}

	if !isGlobalAdminRole(params.ActorRole) {
		query = baseSelect + fmt.Sprintf(`
			and exists (
				select 1
				from user_place_roles upr_target
				where upr_target.user_id = u.id
				  and upr_target.is_active = true
				  and upr_target.place_id in (
					select upr_admin.place_id
					from user_place_roles upr_admin
					join roles r_admin on r_admin.id = upr_admin.role_id
					join places p_admin on p_admin.id = upr_admin.place_id
					where upr_admin.user_id = $1
					  and upr_admin.is_active = true
					  and r_admin.code = $2
					  and p_admin.deleted_at is null
				  )
			)
			order by u.%s %s, u.id asc
			limit $3 offset $4
		`, sortColumn, sortOrder)
		args = []any{params.ActorUserID, placeRoleAdmin, params.Query.PageSize, offset}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return listquery.Response[User]{}, err
	}
	defer rows.Close()

	data := make([]User, 0)
	total := 0
	for rows.Next() {
		var item User
		var totalCount int
		if err := rows.Scan(
			&item.ID,
			&item.RoleID,
			&item.RoleCode,
			&item.FullName,
			&item.Username,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
			&totalCount,
		); err != nil {
			return listquery.Response[User]{}, err
		}
		total = totalCount
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		return listquery.Response[User]{}, err
	}

	return listquery.BuildResponse(data, params.Query, total), nil
}

func isGlobalAdminRole(role string) bool {
	switch role {
	case "SUPER_USER", "SUPER_ADMIN", "ADMIN":
		return true
	default:
		return false
	}
}

func (r *Repository) FindByID(ctx context.Context, params ListUsersParams, userID string) (*User, error) {
	sql := `
		select
			u.id,
			u.role_id,
			r.code as role_code,
			u.full_name,
			u.username,
			u.status,
			u.created_at,
			u.updated_at
		from users u
		join roles r on r.id = u.role_id
		where u.id = $1
		  and u.deleted_at is null
	`
	args := []any{userID}

	if !isGlobalAdminRole(params.ActorRole) {
		sql += `
		  and exists (
			select 1
			from user_place_roles upr_target
			where upr_target.user_id = u.id
			  and upr_target.is_active = true
			  and upr_target.place_id in (
				select upr_admin.place_id
				from user_place_roles upr_admin
				join roles r_admin on r_admin.id = upr_admin.role_id
				join places p_admin on p_admin.id = upr_admin.place_id
				where upr_admin.user_id = $2
				  and upr_admin.is_active = true
				  and r_admin.code = $3
				  and p_admin.deleted_at is null
			  )
		  )
		`
		args = append(args, params.ActorUserID, placeRoleAdmin)
	}

	sql += ` limit 1`

	var item User
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.RoleID,
		&item.RoleCode,
		&item.FullName,
		&item.Username,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &item, nil
}

func (r *Repository) FindRoleCodeByID(ctx context.Context, roleID string) (string, error) {
	const sql = `
		select code
		from roles
		where id = $1
		limit 1
	`

	var code string
	if err := r.db.QueryRow(ctx, sql, roleID).Scan(&code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrUserRoleNotFound
		}
		return "", err
	}

	return strings.TrimSpace(code), nil
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (string, error) {
	const sql = `
		insert into users (role_id, full_name, username, password_hash, status)
		values ($1, $2, $3, crypt($4, gen_salt('bf')), $5)
		returning id
	`

	var id string
	err := r.db.QueryRow(
		ctx,
		sql,
		input.RoleID,
		strings.TrimSpace(input.FullName),
		strings.TrimSpace(input.Username),
		input.Password,
		input.Status,
	).Scan(&id)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return "", ErrUsernameExists
		case isPgCode(err, "23503"):
			return "", ErrUserRoleNotFound
		default:
			return "", err
		}
	}

	return id, nil
}

func (r *Repository) CreateWithPlaceRole(ctx context.Context, input CreateWithPlaceInput) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	const insertUserSQL = `
		insert into users (role_id, full_name, username, password_hash, status)
		values ($1, $2, $3, crypt($4, gen_salt('bf')), $5)
		returning id
	`

	var userID string
	err = tx.QueryRow(
		ctx,
		insertUserSQL,
		input.RoleID,
		strings.TrimSpace(input.FullName),
		strings.TrimSpace(input.Username),
		input.Password,
		input.Status,
	).Scan(&userID)
	if err != nil {
		switch {
		case isPgCode(err, "23505"):
			return "", ErrUsernameExists
		case isPgCode(err, "23503"):
			return "", ErrUserRoleNotFound
		default:
			return "", err
		}
	}

	const insertPlaceRoleSQL = `
		insert into user_place_roles (user_id, place_id, role_id, is_active)
		values ($1, $2, $3, true)
	`

	if _, err := tx.Exec(ctx, insertPlaceRoleSQL, userID, input.PlaceID, input.PlaceRoleID); err != nil {
		switch {
		case isPgCode(err, "23503"):
			return "", ErrUserRoleNotFound
		default:
			return "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return userID, nil
}

func (r *Repository) Update(ctx context.Context, userID string, input UpdateInput) (*User, error) {
	setParts := make([]string, 0, 5)
	args := make([]any, 0, 6)

	if input.RoleID != nil {
		args = append(args, *input.RoleID)
		setParts = append(setParts, fmt.Sprintf("role_id = $%d", len(args)))
	}
	if input.FullName != nil {
		args = append(args, strings.TrimSpace(*input.FullName))
		setParts = append(setParts, fmt.Sprintf("full_name = $%d", len(args)))
	}
	if input.Username != nil {
		args = append(args, strings.TrimSpace(*input.Username))
		setParts = append(setParts, fmt.Sprintf("username = $%d", len(args)))
	}
	if input.Password != nil {
		args = append(args, *input.Password)
		setParts = append(setParts, fmt.Sprintf("password_hash = crypt($%d, gen_salt('bf'))", len(args)))
	}
	if input.Status != nil {
		args = append(args, *input.Status)
		setParts = append(setParts, fmt.Sprintf("status = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrUserNoFieldsToUpdate
	}

	setParts = append(setParts, "updated_at = now()")
	args = append(args, userID)

	sql := fmt.Sprintf(`
		update users
		set %s
		where id = $%d
		  and deleted_at is null
		returning id, role_id, (select code from roles where id = users.role_id), full_name, username, status, created_at, updated_at
	`, strings.Join(setParts, ", "), len(args))

	var item User
	err := r.db.QueryRow(ctx, sql, args...).Scan(
		&item.ID,
		&item.RoleID,
		&item.RoleCode,
		&item.FullName,
		&item.Username,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return nil, ErrUserNotFound
		case isPgCode(err, "23505"):
			return nil, ErrUsernameExists
		case isPgCode(err, "23503"):
			return nil, ErrUserRoleNotFound
		default:
			return nil, err
		}
	}

	return &item, nil
}

func (r *Repository) SoftDelete(ctx context.Context, userID string) (string, error) {
	const sql = `
		update users
		set deleted_at = now(),
			updated_at = now()
		where id = $1
		  and deleted_at is null
		returning id
	`

	var id string
	err := r.db.QueryRow(ctx, sql, userID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", err
	}
	return id, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

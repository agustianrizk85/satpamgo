package roles

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

var (
	ErrRoleNotFound         = errors.New("role not found")
	ErrRoleCodeExists       = errors.New("role code already exists")
	ErrRoleStillInUse       = errors.New("role is still in use")
	ErrRoleNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct {
	db *pgxpool.Pool
}

type Role struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateInput struct {
	Code *string
	Name *string
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, query listquery.Query) (listquery.Response[Role], error) {
	sortColumn := map[string]string{
		"createdAt": "created_at",
		"updatedAt": "updated_at",
		"name":      "name",
		"code":      "code",
	}[query.SortBy]
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := fmt.Sprintf(`
		select
			id, code, name, created_at, updated_at,
			count(*) over()::int as total_count
		from roles
		order by %s %s, id asc
		limit $1 offset $2
	`, sortColumn, sortDirection)

	rows, err := r.db.Query(ctx, sql, query.PageSize, query.Offset)
	if err != nil {
		return listquery.Response[Role]{}, err
	}
	defer rows.Close()

	data := make([]Role, 0)
	totalData := 0
	for rows.Next() {
		var item Role
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt, &totalData); err != nil {
			return listquery.Response[Role]{}, err
		}
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		return listquery.Response[Role]{}, err
	}

	return listquery.BuildResponse(data, query, totalData), nil
}

func (r *Repository) FindByID(ctx context.Context, roleID string) (*Role, error) {
	const sql = `
		select id, code, name, created_at, updated_at
		from roles
		where id = $1
		limit 1
	`

	var item Role
	err := r.db.QueryRow(ctx, sql, roleID).Scan(&item.ID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	return &item, nil
}

func (r *Repository) Create(ctx context.Context, code, name string) (string, error) {
	const sql = `insert into roles (code, name) values ($1, $2) returning id`

	var id string
	err := r.db.QueryRow(ctx, sql, strings.TrimSpace(code), strings.TrimSpace(name)).Scan(&id)
	if err != nil {
		if isPgCode(err, "23505") {
			return "", ErrRoleCodeExists
		}
		return "", err
	}

	return id, nil
}

func (r *Repository) Update(ctx context.Context, roleID string, input UpdateInput) (*Role, error) {
	setParts := make([]string, 0, 2)
	args := make([]any, 0, 3)

	if input.Code != nil {
		args = append(args, strings.TrimSpace(*input.Code))
		setParts = append(setParts, fmt.Sprintf("code = $%d", len(args)))
	}
	if input.Name != nil {
		args = append(args, strings.TrimSpace(*input.Name))
		setParts = append(setParts, fmt.Sprintf("name = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrRoleNoFieldsToUpdate
	}

	setParts = append(setParts, "updated_at = now()")
	args = append(args, roleID)

	sql := fmt.Sprintf(`
		update roles
		set %s
		where id = $%d
		returning id, code, name, created_at, updated_at
	`, strings.Join(setParts, ", "), len(args))

	var item Role
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleNotFound
		}
		if isPgCode(err, "23505") {
			return nil, ErrRoleCodeExists
		}
		return nil, err
	}

	return &item, nil
}

func (r *Repository) Delete(ctx context.Context, roleID string) (string, error) {
	const sql = `delete from roles where id = $1 returning id`

	var id string
	err := r.db.QueryRow(ctx, sql, roleID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRoleNotFound
		}
		if isPgCode(err, "23503") {
			return "", ErrRoleStillInUse
		}
		return "", err
	}

	return id, nil
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

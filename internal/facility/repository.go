package facility

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
	ErrSpotNotFound     = errors.New("facility spot not found")
	ErrItemNotFound     = errors.New("facility item not found")
	ErrScanNotFound     = errors.New("facility scan not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrForeignKey       = errors.New("related row not found")
	ErrNoFieldsToUpdate = errors.New("no fields to update")
)

type Repository struct{ db *pgxpool.Pool }

type Spot struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	SpotCode  string    `json:"spot_code"`
	SpotName  string    `json:"spot_name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Item struct {
	ID         string    `json:"id"`
	SpotID     string    `json:"spot_id"`
	ItemName   string    `json:"item_name"`
	QRToken    *string   `json:"qr_token"`
	IsRequired bool      `json:"is_required"`
	SortNo     int       `json:"sort_no"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Scan struct {
	ID        string    `json:"id"`
	PlaceID   string    `json:"place_id"`
	SpotID    string    `json:"spot_id"`
	ItemID    *string   `json:"item_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	ScannedAt time.Time `json:"scanned_at"`
	SubmitAt  time.Time `json:"submit_at"`
	Status    string    `json:"status"`
	Note      *string   `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) ListSpots(ctx context.Context, actorUserID, actorRole, placeID string, query listquery.Query) (listquery.Response[Spot], error) {
	sortColumn := map[string]string{"createdAt": "created_at", "updatedAt": "updated_at", "spotCode": "spot_code", "spotName": "spot_name", "isActive": "is_active", "placeId": "place_id"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "created_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `select id, place_id, spot_code, spot_name, is_active, created_at, updated_at, count(*) over()::int as total_count from facility_check_spots where place_id = $1`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and place_id in (select distinct upr.place_id from user_place_roles upr join places p on p.id = upr.place_id where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null)`, len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Spot]{}, err
	}
	defer rows.Close()
	data := make([]Spot, 0)
	total := 0
	for rows.Next() {
		var item Spot
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.SpotCode, &item.SpotName, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Spot]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) GetSpot(ctx context.Context, id string) (*Spot, error) {
	const sql = `select id, place_id, spot_code, spot_name, is_active, created_at, updated_at from facility_check_spots where id = $1 limit 1`
	var item Spot
	err := r.db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.PlaceID, &item.SpotCode, &item.SpotName, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSpotNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CreateSpot(ctx context.Context, placeID, spotCode, spotName string, isActive bool) (string, error) {
	const sql = `insert into facility_check_spots (place_id, spot_code, spot_name, is_active) values ($1,$2,$3,$4) returning id`
	return queryID(r.db, ctx, sql, placeID, spotCode, spotName, isActive)
}

func (r *Repository) UpdateSpot(ctx context.Context, id string, placeID, spotCode, spotName *string, isActive *bool) (*Spot, error) {
	setParts := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if placeID != nil {
		args = append(args, *placeID)
		setParts = append(setParts, fmt.Sprintf("place_id = $%d", len(args)))
	}
	if spotCode != nil {
		args = append(args, *spotCode)
		setParts = append(setParts, fmt.Sprintf("spot_code = $%d", len(args)))
	}
	if spotName != nil {
		args = append(args, *spotName)
		setParts = append(setParts, fmt.Sprintf("spot_name = $%d", len(args)))
	}
	if isActive != nil {
		args = append(args, *isActive)
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrNoFieldsToUpdate
	}
	setParts = append(setParts, "updated_at = now()")
	args = append(args, id)
	sql := fmt.Sprintf(`update facility_check_spots set %s where id = $%d returning id, place_id, spot_code, spot_name, is_active, created_at, updated_at`, strings.Join(setParts, ", "), len(args))
	var item Spot
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.PlaceID, &item.SpotCode, &item.SpotName, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, mapPgError(err, ErrSpotNotFound)
	}
	return &item, nil
}

func (r *Repository) DeleteSpot(ctx context.Context, id string) (string, error) {
	return deleteByID(r.db, ctx, "facility_check_spots", id, ErrSpotNotFound)
}

func (r *Repository) ListItems(ctx context.Context, spotID string, query listquery.Query) (listquery.Response[Item], error) {
	sortColumn := map[string]string{"sortNo": "sort_no", "createdAt": "created_at", "updatedAt": "updated_at", "itemName": "item_name", "isRequired": "is_required", "isActive": "is_active"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "sort_no"
	}
	sortDirection := "asc"
	if query.SortOrder == listquery.SortDesc {
		sortDirection = "desc"
	}
	sql := fmt.Sprintf(`select id, spot_id, item_name, qr_token, is_required, sort_no, is_active, created_at, updated_at, count(*) over()::int as total_count from facility_check_items where spot_id = $1 order by %s %s, id asc limit $2 offset $3`, sortColumn, sortDirection)
	rows, err := r.db.Query(ctx, sql, spotID, query.PageSize, query.Offset)
	if err != nil {
		return listquery.Response[Item]{}, err
	}
	defer rows.Close()
	data := make([]Item, 0)
	total := 0
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.SpotID, &item.ItemName, &item.QRToken, &item.IsRequired, &item.SortNo, &item.IsActive, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Item]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) GetItem(ctx context.Context, id string) (*Item, error) {
	const sql = `select id, spot_id, item_name, qr_token, is_required, sort_no, is_active, created_at, updated_at from facility_check_items where id = $1 limit 1`
	var item Item
	err := r.db.QueryRow(ctx, sql, id).Scan(&item.ID, &item.SpotID, &item.ItemName, &item.QRToken, &item.IsRequired, &item.SortNo, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrItemNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CreateItem(ctx context.Context, spotID, itemName string, qrToken *string, isRequired bool, sortNo int, isActive bool) (string, error) {
	const sql = `insert into facility_check_items (spot_id, item_name, qr_token, is_required, sort_no, is_active) values ($1,$2,$3,$4,$5,$6) returning id`
	return queryID(r.db, ctx, sql, spotID, itemName, qrToken, isRequired, sortNo, isActive)
}

func (r *Repository) UpdateItem(ctx context.Context, id string, spotID, itemName, qrToken *string, isRequired *bool, sortNo *int, isActive *bool) (*Item, error) {
	setParts := make([]string, 0, 6)
	args := make([]any, 0, 7)
	if spotID != nil {
		args = append(args, *spotID)
		setParts = append(setParts, fmt.Sprintf("spot_id = $%d", len(args)))
	}
	if itemName != nil {
		args = append(args, *itemName)
		setParts = append(setParts, fmt.Sprintf("item_name = $%d", len(args)))
	}
	if qrToken != nil {
		args = append(args, *qrToken)
		setParts = append(setParts, fmt.Sprintf("qr_token = $%d", len(args)))
	}
	if isRequired != nil {
		args = append(args, *isRequired)
		setParts = append(setParts, fmt.Sprintf("is_required = $%d", len(args)))
	}
	if sortNo != nil {
		args = append(args, *sortNo)
		setParts = append(setParts, fmt.Sprintf("sort_no = $%d", len(args)))
	}
	if isActive != nil {
		args = append(args, *isActive)
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", len(args)))
	}
	if len(setParts) == 0 {
		return nil, ErrNoFieldsToUpdate
	}
	setParts = append(setParts, "updated_at = now()")
	args = append(args, id)
	sql := fmt.Sprintf(`update facility_check_items set %s where id = $%d returning id, spot_id, item_name, qr_token, is_required, sort_no, is_active, created_at, updated_at`, strings.Join(setParts, ", "), len(args))
	var item Item
	err := r.db.QueryRow(ctx, sql, args...).Scan(&item.ID, &item.SpotID, &item.ItemName, &item.QRToken, &item.IsRequired, &item.SortNo, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, mapPgError(err, ErrItemNotFound)
	}
	return &item, nil
}

func (r *Repository) DeleteItem(ctx context.Context, id string) (string, error) {
	return deleteByID(r.db, ctx, "facility_check_items", id, ErrItemNotFound)
}

func (r *Repository) ListScans(ctx context.Context, actorUserID, actorRole, placeID, spotID, userID string, query listquery.Query) (listquery.Response[Scan], error) {
	sortColumn := map[string]string{"scannedAt": "fs.scanned_at", "submitAt": "fs.submit_at", "createdAt": "fs.created_at", "updatedAt": "fs.updated_at", "status": "fs.status", "placeId": "fs.place_id", "spotId": "fs.spot_id", "userId": "fs.user_id"}[query.SortBy]
	if sortColumn == "" {
		sortColumn = "fs.scanned_at"
	}
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}
	sql := `
		select fs.id, fs.place_id, fs.spot_id, fs.item_id, fs.user_id, coalesce(nullif(u.full_name, ''), u.username) as user_name,
			fs.scanned_at, fs.submit_at, fs.status, fs.note, fs.created_at, fs.updated_at, count(*) over()::int as total_count
		from facility_check_scans fs
		join users u on u.id = fs.user_id
		where fs.place_id = $1
	`
	args := []any{placeID}
	if !auth.IsGlobalAdminRole(actorRole) {
		args = append(args, actorUserID)
		sql += fmt.Sprintf(` and fs.place_id in (select distinct upr.place_id from user_place_roles upr join places p on p.id = upr.place_id where upr.user_id = $%d and upr.is_active = true and p.deleted_at is null)`, len(args))
	}
	if spotID != "" {
		args = append(args, spotID)
		sql += fmt.Sprintf(" and fs.spot_id = $%d", len(args))
	}
	if userID != "" {
		args = append(args, userID)
		sql += fmt.Sprintf(" and fs.user_id = $%d", len(args))
	}
	args = append(args, query.PageSize, query.Offset)
	sql += fmt.Sprintf(" order by %s %s, id asc limit $%d offset $%d", sortColumn, sortDirection, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return listquery.Response[Scan]{}, err
	}
	defer rows.Close()
	data := make([]Scan, 0)
	total := 0
	for rows.Next() {
		var item Scan
		if err := rows.Scan(&item.ID, &item.PlaceID, &item.SpotID, &item.ItemID, &item.UserID, &item.UserName, &item.ScannedAt, &item.SubmitAt, &item.Status, &item.Note, &item.CreatedAt, &item.UpdatedAt, &total); err != nil {
			return listquery.Response[Scan]{}, err
		}
		data = append(data, item)
	}
	return listquery.BuildResponse(data, query, total), rows.Err()
}

func (r *Repository) CreateScan(ctx context.Context, placeID, spotID string, itemID *string, userID, status string, note, scannedAt, submitAt *string) (string, error) {
	const sql = `insert into facility_check_scans (place_id, spot_id, item_id, user_id, scanned_at, submit_at, status, note) values ($1,$2,$3,$4,coalesce($5::timestamptz, now()),coalesce($6::timestamptz, now()),$7,$8) returning id`
	return queryID(r.db, ctx, sql, placeID, spotID, itemID, userID, scannedAt, submitAt, status, note)
}

func queryID(db *pgxpool.Pool, ctx context.Context, sql string, args ...any) (string, error) {
	var id string
	err := db.QueryRow(ctx, sql, args...).Scan(&id)
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

func deleteByID(db *pgxpool.Pool, ctx context.Context, table, id string, notFound error) (string, error) {
	sql := fmt.Sprintf("delete from %s where id = $1 returning id", table)
	var out string
	err := db.QueryRow(ctx, sql, id).Scan(&out)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", notFound
		}
		return "", err
	}
	return out, nil
}

func mapPgError(err, notFound error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return notFound
	case isPgCode(err, "23505"):
		return ErrAlreadyExists
	case isPgCode(err, "23503"):
		return ErrForeignKey
	default:
		return err
	}
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.TrimSpace(pgErr.Code) == code
}

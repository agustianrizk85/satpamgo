package apierrorlogs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"satpam-go/internal/auth"
	"satpam-go/internal/listquery"
)

type Repository struct {
	db *pgxpool.Pool
}

type APIErrorLog struct {
	ID           string         `json:"id"`
	OccurredAt   time.Time      `json:"occurred_at"`
	Method       string         `json:"method"`
	Path         string         `json:"path"`
	StatusCode   int            `json:"status_code"`
	Message      *string        `json:"message"`
	PlaceID      *string        `json:"place_id"`
	UserID       *string        `json:"user_id"`
	UserRole     *string        `json:"user_role"`
	ClientIP     *string        `json:"client_ip"`
	UserAgent    *string        `json:"user_agent"`
	RequestQuery map[string]any `json:"request_query"`
	RequestBody  *string        `json:"request_body"`
	ResponseBody *string        `json:"response_body"`
}

type CreateParams struct {
	OccurredAt   time.Time
	Method       string
	Path         string
	StatusCode   int
	Message      *string
	PlaceID      *string
	UserID       *string
	UserRole     *string
	ClientIP     *string
	UserAgent    *string
	RequestQuery map[string]any
	RequestBody  *string
	ResponseBody *string
}

type ListParams struct {
	ActorUserID string
	ActorRole   string
	PlaceID     string
	Method      string
	StatusCode  int
	FromDate    string
	ToDate      string
	Search      string
	Query       listquery.Query
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Insert(ctx context.Context, params CreateParams) error {
	requestQuery := params.RequestQuery
	if requestQuery == nil {
		requestQuery = map[string]any{}
	}
	requestQueryJSON, err := json.Marshal(requestQuery)
	if err != nil {
		requestQueryJSON = []byte("{}")
	}

	const sql = `
		insert into api_error_logs (
			id,
			occurred_at,
			method,
			path,
			status_code,
			message,
			place_id,
			user_id,
			user_role,
			client_ip,
			user_agent,
			request_query,
			request_body,
			response_body
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14
		)
	`

	_, err = r.db.Exec(ctx, sql,
		newLogID(),
		params.OccurredAt,
		strings.ToUpper(strings.TrimSpace(params.Method)),
		strings.TrimSpace(params.Path),
		params.StatusCode,
		trimPtr(params.Message),
		trimPtr(params.PlaceID),
		trimPtr(params.UserID),
		trimPtr(params.UserRole),
		trimPtr(params.ClientIP),
		trimPtr(params.UserAgent),
		string(requestQueryJSON),
		trimPtr(params.RequestBody),
		trimPtr(params.ResponseBody),
	)
	return err
}

func (r *Repository) List(ctx context.Context, params ListParams) (listquery.Response[APIErrorLog], error) {
	sortColumn := map[string]string{
		"occurredAt": "ael.occurred_at",
		"statusCode": "ael.status_code",
		"method":     "ael.method",
		"path":       "ael.path",
	}[params.Query.SortBy]
	if sortColumn == "" {
		sortColumn = "ael.occurred_at"
	}

	sortDirection := "desc"
	if params.Query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	where := make([]string, 0, 8)
	args := make([]any, 0, 8)

	if params.PlaceID != "" {
		args = append(args, params.PlaceID)
		where = append(where, fmt.Sprintf("ael.place_id = $%d::uuid", len(args)))
	} else if !auth.IsGlobalAdminRole(params.ActorRole) {
		args = append(args, params.ActorUserID)
		where = append(where, fmt.Sprintf(`ael.place_id in (
			select distinct upr.place_id
			from user_place_roles upr
			join places p on p.id = upr.place_id
			where upr.user_id = $%d
			  and upr.is_active = true
			  and p.deleted_at is null
		)`, len(args)))
	}

	if params.Method != "" {
		args = append(args, strings.ToUpper(strings.TrimSpace(params.Method)))
		where = append(where, fmt.Sprintf("ael.method = $%d", len(args)))
	}
	if params.StatusCode > 0 {
		args = append(args, params.StatusCode)
		where = append(where, fmt.Sprintf("ael.status_code = $%d", len(args)))
	}
	if params.FromDate != "" {
		args = append(args, params.FromDate)
		where = append(where, fmt.Sprintf("ael.occurred_at >= $%d::date::timestamptz", len(args)))
	}
	if params.ToDate != "" {
		args = append(args, params.ToDate)
		where = append(where, fmt.Sprintf("ael.occurred_at < ($%d::date + interval '1 day')::timestamptz", len(args)))
	}
	if params.Search != "" {
		args = append(args, "%"+strings.TrimSpace(params.Search)+"%")
		where = append(where, fmt.Sprintf("(ael.path ilike $%d or coalesce(ael.message, '') ilike $%d)", len(args), len(args)))
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "where " + strings.Join(where, " and ")
	}

	sql := fmt.Sprintf(`
		select
			ael.id,
			ael.occurred_at,
			ael.method,
			ael.path,
			ael.status_code,
			ael.message,
			ael.place_id,
			ael.user_id,
			ael.user_role,
			ael.client_ip,
			ael.user_agent,
			ael.request_query,
			ael.request_body,
			ael.response_body,
			count(*) over()::int as total_count
		from api_error_logs ael
		%s
		order by %s %s, ael.id desc
		limit $%d offset $%d
	`, whereSQL, sortColumn, sortDirection, len(args)+1, len(args)+2)

	rows, err := r.db.Query(ctx, sql, append(args, params.Query.PageSize, params.Query.Offset)...)
	if err != nil {
		return listquery.Response[APIErrorLog]{}, err
	}
	defer rows.Close()

	data := make([]APIErrorLog, 0)
	total := 0
	for rows.Next() {
		var item APIErrorLog
		var requestQueryRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.OccurredAt,
			&item.Method,
			&item.Path,
			&item.StatusCode,
			&item.Message,
			&item.PlaceID,
			&item.UserID,
			&item.UserRole,
			&item.ClientIP,
			&item.UserAgent,
			&requestQueryRaw,
			&item.RequestBody,
			&item.ResponseBody,
			&total,
		); err != nil {
			return listquery.Response[APIErrorLog]{}, err
		}
		if len(requestQueryRaw) > 0 {
			_ = json.Unmarshal(requestQueryRaw, &item.RequestQuery)
		}
		if item.RequestQuery == nil {
			item.RequestQuery = map[string]any{}
		}
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		return listquery.Response[APIErrorLog]{}, err
	}

	return listquery.BuildResponse(data, params.Query, total), nil
}

func trimPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func newLogID() string {
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

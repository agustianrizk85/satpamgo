package patrol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"satpam-go/internal/listquery"
)

type PatrolFeedItem struct {
	ID             string              `json:"id"`
	PlaceID        string              `json:"place_id"`
	PlaceName      string              `json:"place_name"`
	UserID         string              `json:"user_id"`
	UserName       string              `json:"user_name"`
	SpotID         string              `json:"spot_id"`
	SpotCode       string              `json:"spot_code"`
	SpotName       string              `json:"spot_name"`
	PatrolRunID    string              `json:"patrol_run_id"`
	RunNo          int                 `json:"run_no"`
	ScannedAt      time.Time           `json:"scanned_at"`
	SubmitAt       time.Time           `json:"submit_at"`
	PhotoURL       *string             `json:"photo_url"`
	Note           *string             `json:"note"`
	LikeCount      int                 `json:"like_count"`
	CommentCount   int                 `json:"comment_count"`
	LikedByMe      bool                `json:"liked_by_me"`
	RecentComments []PatrolFeedComment `json:"recent_comments"`
}

type PatrolFeedComment struct {
	ID        string    `json:"id"`
	ScanID    string    `json:"scan_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *Repository) ListFeed(ctx context.Context, actorUserID string, query listquery.Query) (listquery.Response[PatrolFeedItem], error) {
	sortDirection := "desc"
	if query.SortOrder == listquery.SortAsc {
		sortDirection = "asc"
	}

	sql := fmt.Sprintf(`
		select
			ps.id,
			ps.place_id,
			coalesce(p.place_name, 'Place') as place_name,
			ps.user_id,
			coalesce(u.full_name, u.username, 'User') as user_name,
			ps.spot_id,
			coalesce(s.spot_code, ps.spot_id::text) as spot_code,
			coalesce(s.spot_name, 'Patrol Spot') as spot_name,
			ps.patrol_run_id,
			coalesce(pr.run_no, 0) as run_no,
			ps.scanned_at,
			ps.submit_at,
			ps.photo_url,
			ps.note,
			coalesce(l.like_count, 0)::int as like_count,
			coalesce(c.comment_count, 0)::int as comment_count,
			exists(
				select 1
				from patrol_feed_likes pfl
				where pfl.patrol_scan_id = ps.id
				  and pfl.user_id = $1
			) as liked_by_me,
			count(*) over()::int as total_count
		from patrol_scans ps
		left join patrol_runs pr on pr.id = ps.patrol_run_id
		left join places p on p.id = ps.place_id
		left join users u on u.id = ps.user_id
		left join spots s on s.id = ps.spot_id
		left join lateral (
			select count(*)::int as like_count
			from patrol_feed_likes pfl
			where pfl.patrol_scan_id = ps.id
		) l on true
		left join lateral (
			select count(*)::int as comment_count
			from patrol_feed_comments pfc
			where pfc.patrol_scan_id = ps.id
		) c on true
		order by ps.scanned_at %s, ps.id asc
		limit $2 offset $3
	`, sortDirection)

	rows, err := r.db.Query(ctx, sql, actorUserID, query.PageSize, query.Offset)
	if err != nil {
		return listquery.Response[PatrolFeedItem]{}, err
	}
	defer rows.Close()

	data := make([]PatrolFeedItem, 0)
	total := 0
	for rows.Next() {
		var item PatrolFeedItem
		if err := rows.Scan(
			&item.ID,
			&item.PlaceID,
			&item.PlaceName,
			&item.UserID,
			&item.UserName,
			&item.SpotID,
			&item.SpotCode,
			&item.SpotName,
			&item.PatrolRunID,
			&item.RunNo,
			&item.ScannedAt,
			&item.SubmitAt,
			&item.PhotoURL,
			&item.Note,
			&item.LikeCount,
			&item.CommentCount,
			&item.LikedByMe,
			&total,
		); err != nil {
			return listquery.Response[PatrolFeedItem]{}, err
		}
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		return listquery.Response[PatrolFeedItem]{}, err
	}

	for i := range data {
		comments, err := r.ListFeedComments(ctx, data[i].ID, 3)
		if err != nil {
			return listquery.Response[PatrolFeedItem]{}, err
		}
		data[i].RecentComments = comments
	}

	return listquery.BuildResponse(data, query, total), nil
}

func (r *Repository) ToggleFeedLike(ctx context.Context, scanID, userID string) (bool, int, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback(ctx)

	var deleted string
	err = tx.QueryRow(ctx, `
		delete from patrol_feed_likes
		where patrol_scan_id = $1 and user_id = $2
		returning user_id::text
	`, scanID, userID).Scan(&deleted)
	liked := false
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return false, 0, err
		}
		if _, err := tx.Exec(ctx, `
			insert into patrol_feed_likes (patrol_scan_id, user_id)
			values ($1, $2)
			on conflict do nothing
		`, scanID, userID); err != nil {
			return false, 0, err
		}
		liked = true
	}

	var count int
	if err := tx.QueryRow(ctx, `select count(*)::int from patrol_feed_likes where patrol_scan_id = $1`, scanID).Scan(&count); err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return liked, count, nil
}

func (r *Repository) CreateFeedComment(ctx context.Context, scanID, userID, text string) (*PatrolFeedComment, int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, 0, ErrForeignKey
	}
	var item PatrolFeedComment
	err := r.db.QueryRow(ctx, `
		insert into patrol_feed_comments (patrol_scan_id, user_id, comment_text)
		values ($1, $2, $3)
		returning id::text, patrol_scan_id::text, user_id::text, comment_text, created_at
	`, scanID, userID, text).Scan(&item.ID, &item.ScanID, &item.UserID, &item.Text, &item.CreatedAt)
	if err != nil {
		if isPgCode(err, "23503") {
			return nil, 0, ErrForeignKey
		}
		return nil, 0, err
	}

	var name string
	_ = r.db.QueryRow(ctx, `select coalesce(full_name, username, 'User') from users where id = $1`, userID).Scan(&name)
	item.UserName = name

	var count int
	if err := r.db.QueryRow(ctx, `select count(*)::int from patrol_feed_comments where patrol_scan_id = $1`, scanID).Scan(&count); err != nil {
		return nil, 0, err
	}
	return &item, count, nil
}

func (r *Repository) ListFeedComments(ctx context.Context, scanID string, limit int) ([]PatrolFeedComment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(ctx, `
		select
			pfc.id::text,
			pfc.patrol_scan_id::text,
			pfc.user_id::text,
			coalesce(u.full_name, u.username, 'User') as user_name,
			pfc.comment_text,
			pfc.created_at
		from patrol_feed_comments pfc
		left join users u on u.id = pfc.user_id
		where pfc.patrol_scan_id = $1
		order by pfc.created_at desc, pfc.id desc
		limit $2
	`, scanID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PatrolFeedComment, 0)
	for rows.Next() {
		var item PatrolFeedComment
		if err := rows.Scan(&item.ID, &item.ScanID, &item.UserID, &item.UserName, &item.Text, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

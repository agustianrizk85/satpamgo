package attendances

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) AutoCheckoutDue(ctx context.Context, now time.Time, graceMinutes int, systemPhotoURL, systemNote string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const sql = `
		with due_rows as (
			select
				a.id,
				a.place_id,
				a.user_id,
				case
					when s.end_time <= s.start_time
						then (a.attendance_date + s.end_time + interval '1 day' + make_interval(mins => $1))::timestamptz
					else (a.attendance_date + s.end_time + make_interval(mins => $1))::timestamptz
				end as auto_checkout_at
			from attendances a
			join shifts s on s.id = a.shift_id
			where a.check_in_at is not null
			  and a.check_out_at is null
		),
		updated_attendances as (
			update attendances a
			set check_out_at = due_rows.auto_checkout_at,
				submit_at = coalesce(a.submit_at, due_rows.auto_checkout_at),
				check_out_photo_url = case
					when a.check_out_photo_url is null or btrim(a.check_out_photo_url) = '' then $2
					else a.check_out_photo_url
				end,
				note = case
					when a.note is null or btrim(a.note) = '' then $3
					else a.note
				end,
				updated_at = $4
			from due_rows
			where a.id = due_rows.id
			  and due_rows.auto_checkout_at <= $4
			returning due_rows.place_id, due_rows.user_id
		),
		updated_assignments as (
			update spot_assignments sa
			set is_active = false,
				updated_at = $4
			where sa.is_active = true
			  and exists (
				select 1
				from updated_attendances ua
				where ua.place_id = sa.place_id
				  and ua.user_id = sa.user_id
			  )
			returning sa.id
		)
		select count(*)::bigint from updated_attendances
	`

	var updated int64
	if err := tx.QueryRow(ctx, sql, graceMinutes, systemPhotoURL, systemNote, now).Scan(&updated); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return updated, nil
}

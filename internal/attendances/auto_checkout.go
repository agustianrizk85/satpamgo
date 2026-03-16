package attendances

import (
	"context"
	"time"
)

func (r *Repository) AutoCheckoutDue(ctx context.Context, now time.Time, graceMinutes int, systemPhotoURL, systemNote string) (int64, error) {
	const sql = `
		with due_rows as (
			select
				a.id,
				case
					when s.end_time <= s.start_time
						then (a.attendance_date + s.end_time + interval '1 day' + make_interval(mins => $1))::timestamptz
					else (a.attendance_date + s.end_time + make_interval(mins => $1))::timestamptz
				end as auto_checkout_at
			from attendances a
			join shifts s on s.id = a.shift_id
			where a.check_in_at is not null
			  and a.check_out_at is null
		)
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
	`

	result, err := r.db.Exec(ctx, sql, graceMinutes, systemPhotoURL, systemNote, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

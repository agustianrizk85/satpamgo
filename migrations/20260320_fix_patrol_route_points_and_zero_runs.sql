insert into patrol_route_points (place_id, spot_id, seq, is_active)
select
  missing.place_id,
  missing.id as spot_id,
  (
    coalesce(existing.max_seq, 0) + row_number() over (
      partition by missing.place_id
      order by missing.spot_code asc, missing.created_at asc, missing.id asc
    )
  )::int as seq,
  true as is_active
from (
  select s.place_id, s.id, s.spot_code, s.created_at
  from spots s
  where s.status = 'ACTIVE'
    and not exists (
      select 1
      from patrol_route_points prp
      where prp.place_id = s.place_id
        and prp.spot_id = s.id
    )
) missing
left join (
  select place_id, max(seq) as max_seq
  from patrol_route_points
  group by place_id
) existing
  on existing.place_id = missing.place_id;

update patrol_runs pr
set status = 'completed',
    completed_at = coalesce(pr.completed_at, now()),
    updated_at = now()
where pr.status = 'active'
  and coalesce(pr.total_active_spots, 0) = 0
  and exists (
    select 1
    from patrol_route_points prp
    where prp.place_id = pr.place_id
      and prp.is_active = true
  );

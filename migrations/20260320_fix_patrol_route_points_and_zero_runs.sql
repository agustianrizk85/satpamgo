insert into patrol_route_points (place_id, spot_id, seq, is_active)
select
  s.place_id,
  s.id as spot_id,
  row_number() over (
    partition by s.place_id
    order by s.spot_code asc, s.created_at asc, s.id asc
  )::int as seq,
  true as is_active
from spots s
where s.status = 'ACTIVE'
  and not exists (
    select 1
    from patrol_route_points prp
    where prp.place_id = s.place_id
      and prp.spot_id = s.id
  );

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

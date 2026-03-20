create table if not exists patrol_runs (
  id text primary key,
  place_id uuid not null references places(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  attendance_id uuid null references attendances(id) on delete set null,
  run_no integer not null,
  total_active_spots integer not null default 0,
  status text not null default 'active',
  started_at timestamptz not null default now(),
  completed_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint patrol_runs_status_check check (status in ('active', 'completed'))
);

create index if not exists patrol_runs_attendance_idx
  on patrol_runs (attendance_id);

create index if not exists patrol_runs_scope_status_idx
  on patrol_runs (place_id, user_id, attendance_id, status, run_no desc);

create unique index if not exists patrol_runs_scope_run_no_idx
  on patrol_runs (place_id, user_id, attendance_id, run_no);

insert into patrol_runs (
  id,
  place_id,
  user_id,
  attendance_id,
  run_no,
  total_active_spots,
  status,
  started_at,
  completed_at,
  created_at,
  updated_at
)
with grouped as (
  select
    ps.patrol_run_id as id,
    ps.place_id,
    ps.user_id,
    ps.attendance_id,
    min(ps.scanned_at) as started_at,
    max(coalesce(ps.submit_at, ps.scanned_at)) as completed_at
  from patrol_scans ps
  where nullif(trim(ps.patrol_run_id), '') is not null
  group by ps.patrol_run_id, ps.place_id, ps.user_id, ps.attendance_id
),
ranked as (
  select
    g.*,
    dense_rank() over (
      partition by g.place_id, g.user_id, g.attendance_id
      order by g.started_at asc, g.id asc
    )::int as run_no
  from grouped g
)
select
  r.id,
  r.place_id,
  r.user_id,
  r.attendance_id,
  r.run_no,
  coalesce((
    select count(*)::int
    from patrol_route_points prp
    where prp.place_id = r.place_id
      and prp.is_active = true
  ), 0) as total_active_spots,
  'completed' as status,
  r.started_at,
  r.completed_at,
  r.started_at as created_at,
  r.completed_at as updated_at
from ranked r
on conflict (id) do nothing;

create index if not exists patrol_scans_patrol_run_idx
  on patrol_scans (patrol_run_id);

do $$
begin
  if not exists (
    select 1
    from pg_constraint
    where conname = 'patrol_scans_patrol_run_id_fkey'
  ) then
    alter table patrol_scans
      add constraint patrol_scans_patrol_run_id_fkey
      foreign key (patrol_run_id) references patrol_runs(id) on delete restrict;
  end if;
end $$;

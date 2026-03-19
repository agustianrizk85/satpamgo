alter table if exists patrol_scans
  add column if not exists attendance_id uuid;

do $$
begin
  if not exists (
    select 1
    from pg_constraint
    where conname = 'patrol_scans_attendance_id_fkey'
  ) then
    alter table patrol_scans
      add constraint patrol_scans_attendance_id_fkey
      foreign key (attendance_id) references attendances(id) on delete set null;
  end if;
end $$;

create index if not exists patrol_scans_attendance_idx
  on patrol_scans (attendance_id);

create index if not exists patrol_scans_attendance_spot_idx
  on patrol_scans (attendance_id, spot_id);

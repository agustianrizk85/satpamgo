alter table if exists attendances
  add column if not exists submit_at timestamptz;

update attendances
set submit_at = coalesce(submit_at, updated_at, created_at, check_out_at, check_in_at, now())
where submit_at is null;

alter table if exists attendances
  alter column submit_at set default now();

alter table if exists patrol_scans
  add column if not exists submit_at timestamptz;

update patrol_scans
set submit_at = coalesce(submit_at, scanned_at, now())
where submit_at is null;

alter table if exists patrol_scans
  alter column submit_at set default now();

alter table if exists facility_check_scans
  add column if not exists submit_at timestamptz;

update facility_check_scans
set submit_at = coalesce(submit_at, created_at, scanned_at, now())
where submit_at is null;

alter table if exists facility_check_scans
  alter column submit_at set default now();

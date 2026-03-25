create table if not exists app_versions (
  id uuid primary key default gen_random_uuid(),
  place_id uuid not null references places(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  version_name text not null,
  download_url text not null,
  is_mandatory boolean not null default false,
  is_active boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists app_versions_scope_idx
  on app_versions (place_id, user_id, is_active, created_at desc);

create unique index if not exists app_versions_scope_version_idx
  on app_versions (place_id, user_id, version_name);

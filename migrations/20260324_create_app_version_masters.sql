create table if not exists app_version_masters (
    id uuid primary key default gen_random_uuid(),
    version_name text not null unique,
    download_url text not null,
    is_mandatory boolean not null default false,
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists app_version_masters_active_idx
    on app_version_masters (is_active, created_at desc);

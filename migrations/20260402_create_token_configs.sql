create table if not exists token_configs (
  id boolean primary key default true check (id = true),
  access_ttl_seconds integer not null default 28800,
  refresh_ttl_seconds integer not null default 2592000,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (access_ttl_seconds >= 1),
  check (refresh_ttl_seconds >= 1)
);

insert into token_configs (id, access_ttl_seconds, refresh_ttl_seconds)
values (true, 28800, 2592000)
on conflict (id) do nothing;

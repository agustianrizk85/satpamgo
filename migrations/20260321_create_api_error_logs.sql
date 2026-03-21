create table if not exists api_error_logs (
  id uuid primary key,
  occurred_at timestamptz not null default now(),
  method text not null,
  path text not null,
  status_code integer not null,
  message text null,
  place_id uuid null references places(id) on delete set null,
  user_id uuid null references users(id) on delete set null,
  user_role text null,
  client_ip text null,
  user_agent text null,
  request_query jsonb not null default '{}'::jsonb,
  request_body text null,
  response_body text null
);

create index if not exists api_error_logs_occurred_at_idx on api_error_logs (occurred_at desc);
create index if not exists api_error_logs_place_idx on api_error_logs (place_id);
create index if not exists api_error_logs_user_idx on api_error_logs (user_id);
create index if not exists api_error_logs_status_idx on api_error_logs (status_code);
create index if not exists api_error_logs_method_idx on api_error_logs (method);

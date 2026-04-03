create table if not exists patrol_round_masters (
  id uuid primary key,
  place_id uuid not null references places(id) on delete cascade,
  round_no int not null,
  is_active boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint patrol_round_masters_round_no_check check (round_no >= 1)
);

create unique index if not exists patrol_round_masters_place_round_no_uq
  on patrol_round_masters (place_id, round_no);

create index if not exists patrol_round_masters_place_active_idx
  on patrol_round_masters (place_id, is_active, round_no);

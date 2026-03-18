create table if not exists visitors (
    id uuid primary key default gen_random_uuid(),
    place_id uuid not null references places(id) on delete cascade,
    user_id uuid not null references users(id) on delete restrict,
    nik varchar(100) not null,
    nama varchar(200) not null,
    tujuan text,
    catatan text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists idx_visitors_place_id on visitors(place_id);
create index if not exists idx_visitors_user_id on visitors(user_id);
create index if not exists idx_visitors_created_at on visitors(created_at desc);

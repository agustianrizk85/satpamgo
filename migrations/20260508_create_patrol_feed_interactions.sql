create table if not exists patrol_feed_likes (
  patrol_scan_id uuid not null references patrol_scans(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (patrol_scan_id, user_id)
);

create index if not exists patrol_feed_likes_user_idx
  on patrol_feed_likes (user_id);

create table if not exists patrol_feed_comments (
  id uuid primary key default gen_random_uuid(),
  patrol_scan_id uuid not null references patrol_scans(id) on delete cascade,
  user_id uuid not null references users(id) on delete cascade,
  comment_text text not null,
  created_at timestamptz not null default now()
);

create index if not exists patrol_feed_comments_scan_created_idx
  on patrol_feed_comments (patrol_scan_id, created_at desc);

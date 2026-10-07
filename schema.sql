-- Applied automatically at startup; safe to run more than once.
create table if not exists settings (
  user_id    text primary key,
  baseline   numeric(12, 2) not null,
  threshold  numeric(12, 2) not null,
  updated_at timestamptz not null default now()
);

create table if not exists expenses (
  id         text primary key,
  user_id    text not null,
  date       date not null,
  amount     numeric(12, 2) not null,
  title      text not null,
  category   text not null,
  note       text not null default '',
  created_at timestamptz not null default now()
);

create index if not exists expenses_user_date on expenses (user_id, date);

-- The Go server connects as the database owner and bypasses RLS. Enabling RLS
-- with no policies stops Supabase's public REST API (anon key) reading these tables.
alter table settings enable row level security;
alter table expenses enable row level security;

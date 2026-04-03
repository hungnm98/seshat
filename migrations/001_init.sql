create table if not exists projects (
  id text primary key,
  name text not null,
  default_branch text not null,
  description text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists project_versions (
  id text primary key,
  project_id text not null references projects(id),
  commit_sha text not null,
  branch text not null,
  status text not null,
  schema_version text not null,
  scanned_at timestamptz not null,
  files_count integer not null default 0,
  nodes_count integer not null default 0,
  edges_count integer not null default 0
);

create table if not exists files (
  id text primary key,
  project_version_id text not null references project_versions(id),
  path text not null,
  language text not null,
  checksum text not null
);

create table if not exists symbols (
  id text primary key,
  project_version_id text not null references project_versions(id),
  file_id text not null,
  kind text not null,
  name text not null,
  signature text,
  language text not null,
  path text not null,
  line_start integer not null,
  line_end integer not null,
  parent_id text
);

create table if not exists relations (
  id text primary key,
  project_version_id text not null references project_versions(id),
  from_symbol_id text not null,
  to_symbol_id text not null,
  relation_type text not null,
  metadata_json jsonb default '{}'::jsonb
);

create table if not exists context_cache (
  id text primary key,
  project_id text not null references projects(id),
  query_hash text not null,
  context_json jsonb not null,
  expires_at timestamptz
);

create table if not exists admin_users (
  id text primary key,
  username text not null unique,
  name text not null,
  password text not null,
  avatar text default '',
  remember_token text default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  last_login_at timestamptz
);

create table if not exists project_tokens (
  id text primary key,
  project_id text not null references projects(id),
  description text not null,
  token_prefix text not null,
  token_hash text not null,
  status text not null,
  expires_at timestamptz,
  last_used_at timestamptz,
  created_at timestamptz not null default now(),
  created_by text not null,
  revoked_at timestamptz,
  revoked_by text
);

create table if not exists ingestion_runs (
  id text primary key,
  project_id text not null references projects(id),
  project_version_id text not null references project_versions(id),
  commit_sha text not null,
  status text not null,
  error_message text,
  created_at timestamptz not null default now(),
  finished_at timestamptz
);

create table if not exists audit_logs (
  id text primary key,
  actor_id text not null,
  actor_name text not null,
  action text not null,
  resource text not null,
  resource_id text not null,
  metadata_json jsonb default '{}'::jsonb,
  created_at timestamptz not null default now()
);

create table if not exists analysis_batches (
  project_version_id text primary key references project_versions(id) on delete cascade,
  raw_payload bytea not null,
  created_at timestamptz not null default now()
);

create extension if not exists pgcrypto;

create table if not exists projects (
    id uuid primary key default gen_random_uuid(),
    owner_id uuid not null references users(id) on delete cascade,
    slug text not null,
    name text not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

alter table projects add column if not exists owner_id uuid;
alter table projects add column if not exists slug text;
alter table projects add column if not exists name text not null default '';
alter table projects add column if not exists created_at timestamptz not null default now();
alter table projects add column if not exists updated_at timestamptz not null default now();

create unique index if not exists projects_slug_unique_idx on projects (slug);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'projects_owner_id_fkey'
          AND conrelid = 'projects'::regclass
    ) THEN
        ALTER TABLE projects
            ADD CONSTRAINT projects_owner_id_fkey
            FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'projects_slug_format_check'
          AND conrelid = 'projects'::regclass
    ) THEN
        ALTER TABLE projects
            ADD CONSTRAINT projects_slug_format_check
            CHECK (char_length(slug) between 1 and 60 and slug ~ '^[a-z0-9-]+$') NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM projects WHERE owner_id IS NULL) THEN
        ALTER TABLE projects ALTER COLUMN owner_id SET NOT NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM projects WHERE slug IS NULL) THEN
        ALTER TABLE projects ALTER COLUMN slug SET NOT NULL;
    END IF;
END $$;

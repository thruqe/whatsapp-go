# Database & Storage Guide

whatsrook supports both SQLite and PostgreSQL storage backends.

## 1. Storage Drivers

### SQLite (Default)

SQLite is an embedded single-file database (`whatsrook.db`) that requires no external database installation. It is ideal for local development, testing, and single-session bots.

```bash
DATABASE_URL="sqlite"
# or run without the -db flag
```

### PostgreSQL (Production)

PostgreSQL is a high-concurrency, scalable relational database backend. It supports multi-session hosting on a single database instance with strict data isolation, and is compatible with the latest PostgreSQL releases and managed cloud providers such as Supabase, Neon, AWS RDS, and Render.

```bash
# Local PostgreSQL instance
DATABASE_URL="postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
```

## 2. Step-by-Step: Supabase Setup with Session Pooler

Supabase provides a free, fully managed PostgreSQL database with Supavisor connection pooling. whatsrook requires Session Pooler mode for prepared statements, advisory locks, and long-lived socket persistence.

### Step 1: Create a Supabase Project

1. Go to [supabase.com](https://supabase.com) and sign in or create a free account.
2. In the dashboard, click New Project and select your organization.
3. Fill in the project details:
   - Name: e.g., `whatsrook-db`
   - Database Password: A strong password should be set and saved securely.
   - Region: A region geographically close to your bot server should be chosen for optimal latency.
4. Click Create new project and wait a few moments for provisioning to finish.

### Step 2: Obtain the Session Pooler Connection String

1. In the Supabase project dashboard, click on Project Settings (gear icon) in the bottom-left sidebar.
2. Select Database under the Configuration section.
3. Scroll to the Connection Pooling (or Connection string) section.
4. Under the URI tab, the Mode dropdown should be changed to Session. Note the pooler host (e.g., `aws-0-[region].pooler.supabase.com`) and port (`5432` or `6543`).
5. Copy the generated URI:
   ```
   postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require
   ```
6. Replace `[YOUR-PASSWORD]` with the database password set in Step 1.

### Step 3: Configure whatsrook

The copied connection URL should be added to your `.env` file, or exported directly:

```bash
# In your .env file
DATABASE_URL="postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

It can also be passed via the CLI flag:

```bash
whatsrook -s 2348000000000 -db "postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

whatsrook will automatically connect over TLS, apply all required schema migrations, and manage device sessions.

## 3. Multi-Session Data Isolation

When multiple WhatsApp sessions are run on a single PostgreSQL database, whatsrook isolates all session data using composite primary keys on `our_jid`:

- `bot_settings`: `PRIMARY KEY (our_jid, key)`
- `call_media_config`: `PRIMARY KEY (our_jid, jid, kind)`
- `group_stats`: `PRIMARY KEY (our_jid, group_jid, user_jid, date_str)`
- `bot_user_xp`: `PRIMARY KEY (our_jid, user_jid)`
- `bot_group_user_xp`: `PRIMARY KEY (our_jid, group_jid, user_jid)`
- `bot_filters`: Scoped by `our_jid`
- `bot_bgm`: Scoped by `our_jid`
- `bot_sticker_cmds`: Scoped by `our_jid`
- `cached_groups`: Scoped by `our_jid`

This guarantees that Session A's configuration, such as its custom prefix, sudoers list, anticall media, and group stats, cannot leak into or conflict with Session B.

## 4. Schema Migrations

Database migrations are split into two decoupled layers:

1. Protocol Core Migrations (`whatsmeow_version`): These are managed automatically by [sqlstore](../wa-core/store/sqlstore/), and handle WhatsApp identity keys, prekeys, session crypto state, contacts, and message history.

2. whatsrook Migrations (`whatsrook_version`): These are managed automatically by [migrations](../cli/store/migrations.go), and run idempotent versioned schema upgrades (v1 through v7) for bot settings, cached groups, media configs, and composite indexes.

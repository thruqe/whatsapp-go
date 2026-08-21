# Database & Storage Guide

WhatsRook supports both **SQLite** and **PostgreSQL** storage backends.

## 1. Storage Drivers

### SQLite (Default)

- Embedded single-file database (`whatsrook.db`).
- Ideal for local development, testing, and single-session bots.
- Requires no external database installation.

```bash
DATABASE_URL="sqlite"
# or run without -db flag
```

### PostgreSQL (Production)

- High-concurrency, scalable relational database backend.
- Supports multi-session hosting on a single database instance with strict data isolation.
- Compatible with the latest PostgreSQL releases and managed cloud providers (Supabase, Neon, AWS RDS, Render).

```bash
# Local PostgreSQL instance
DATABASE_URL="postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
```

## 2. Step-by-Step: Supabase Setup with Session Pooler

Supabase provides a free, fully managed PostgreSQL database with Supavisor connection pooling. WhatsRook requires **Session Pooler** mode for prepared statements, advisory locks, and long-lived socket persistence.

### Step 1: Create a Supabase Project

1. Go to [supabase.com](https://supabase.com) and sign in or create a free account.
2. In the dashboard, click **New Project** and select your organization.
3. Fill in project details:
   - **Name**: e.g., `whatsrook-db`
   - **Database Password**: Set a strong password and save it securely.
   - **Region**: Choose a region geographically close to your bot server for optimal latency.
4. Click **Create new project** and wait a few moments for provisioning to finish.

### Step 2: Obtain the Session Pooler Connection String

1. In the Supabase project dashboard, click on the **Project Settings** (gear icon) in the bottom-left sidebar.
2. Select **Database** under the Configuration section.
3. Scroll to the **Connection Pooling** (or **Connection string**) section.
4. Under the **URI** tab:
   - Change the **Mode** dropdown to **Session**.
   - Note the pooler host (e.g., `aws-0-[region].pooler.supabase.com`) and port (`5432` or `6543`).
5. Copy the generated URI:
   ```
   postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require
   ```
6. Replace `[YOUR-PASSWORD]` with the database password set in Step 1.

### Step 3: Configure WhatsRook

Add the copied connection URL to your `.env` file or export it directly:

```bash
# In your .env file
DATABASE_URL="postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

Or pass it via the CLI flag:

```bash
./bin/whatsrook -s 2348000000000 -db "postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

WhatsRook will automatically connect over TLS, apply all required schema migrations, and manage device sessions.

## 3. Multi-Session Data Isolation

When running multiple WhatsApp sessions on a single PostgreSQL database, WhatsRook isolates all session data using composite primary keys on `our_jid`:

- `bot_settings`: `PRIMARY KEY (our_jid, key)`
- `call_media_config`: `PRIMARY KEY (our_jid, jid, kind)`
- `group_stats`: `PRIMARY KEY (our_jid, group_jid, user_jid, date_str)`
- `bot_user_xp`: `PRIMARY KEY (our_jid, user_jid)`
- `bot_group_user_xp`: `PRIMARY KEY (our_jid, group_jid, user_jid)`
- `bot_filters`: Scoped by `our_jid`
- `bot_bgm`: Scoped by `our_jid`
- `bot_sticker_cmds`: Scoped by `our_jid`
- `cached_groups`: Scoped by `our_jid`

This guarantees that Session A's configuration (custom prefix, sudoers list, anticall media, group stats) cannot leak into or conflict with Session B.

## 4. Schema Migrations

Database migrations are split into two decoupled layers:

1. **Protocol Core Migrations (`whatsmeow_version`)**:
   - Managed automatically by `wa-core/store/sqlstore`.
   - Handles WhatsApp identity keys, prekeys, session crypto state, contacts, and message history.

2. **WhatsRook CLI Migrations (`whatsrook_version`)**:
   - Managed automatically by `cli/store/migrations.go`.
   - Runs idempotent versioned schema upgrades (v1 through v7) for bot settings, cached groups, media configs, and composite indexes.

# Configuration Guide

whatsrook can be configured using environment variables from a `.env` file or through command line flags. The command line flags override environment variables.

## Configuration Reference

| Variable       | Flag             | Default   | Description                                                                                                           |
| -------------- | ---------------- | --------- | --------------------------------------------------------------------------------------------------------------------- |
| `SESSION`      | `-session`, `-s` | —         | Session identifier / phone number with country code.                                                                  |
| `AUTH`         | `-auth`, `-a`    | `qr`      | Authentication method: `pair` or `qr`.                                                                                |
| `CLIENT`       | `-client`, `-c`  | `default` | Target client identity platform: `default` (chrome), `android`, `ios`.                                                |
| `DATABASE_URL` | `-db-url`, `-db` | `default` | Database: `default` (sqlite) or a PostgreSQL connection string (`postgres://user:pass@host:5432/db?sslmode=disable`). |
| `LOGOUT`       | `-logout`, `-l`  | `false`   | Remove session credentials and exit.                                                                                  |
| —              | `-update`, `-u`  | —         | Check or apply an update. Accepts `check`, `stable`, `beta`, or no value for a direct update.                         |

A `.env` file is loaded automatically from the current directory or the parent directory (`.env`, `../.env`) before flags are parsed. Existing environment variables take precedence over values in `.env`.

## Session Database Management

If you are managing multiple sessions, each session can point to its own database instance, or all sessions can share a single database. whatsrook automatically and primarily handles a shared database connection or file without any extra configuration required.

For cases where a specific session needs its own database, separate from the global `DATABASE_URL`, you can override it by setting `DATABASE_URL_<SESSION_NUMBER>`. This allows one session to use an isolated database while every other session continues to use the shared fallback.

```bash
# Global fallback
DATABASE_URL="postgres://user:pass@localhost:5432/whatsrook_shared?sslmode=disable"

# Dedicated database for session SESSION_NUMBER
DATABASE_URL_2341234567890="postgres://user:pass@db2.example.com:5432/isolated_session?sslmode=disable"
```

When whatsrook resolves which database connection to use for a session, it checks the following, in order:

1. CLI flag `-db-url` / `-db`
2. Session-specific environment variable: `DATABASE_URL_<PHONE>` (phone number from the resolved session, without a leading `+`)
3. Generic environment variables, in order: `DATABASE_URL`, `POSTGRES_URL`, `DB_URL`
4. Default: `sqlite` (`whatsrook.db`)

## Authentication & Pairing Modes

whatsrook supports two ways to authenticate a session: a pairing code, or a QR code. This is controlled with the `-auth` / `-a` flag (or `AUTH` env var), which accepts `pair` or `qr`. Any other or missing value falls back to `qr`.

### 1. Pairing Code

To use pairing-code auth, run with `-auth pair` (or `-a pair`) alongside your session phone number:

```bash
whatsrook -s 2348000000000 -auth pair
```

whatsrook will generate an 8-character pairing code (e.g. `ABCD-1234`). Enter this code on your phone under WhatsApp > Linked Devices > Link with phone number.

### 2. QR Code (Terminal ASCII)

QR code auth is the default, so it's used whenever `-auth` isn't set to `pair`:

```bash
whatsrook -s 2348000000000 -auth qr
```

An ASCII QR code will be displayed in your terminal, ready to scan using the WhatsApp mobile app.

## Session Resolution

The session phone number is resolved in this order:

1. CLI flag `-session` / `-s`
2. A positional argument that looks like a phone number (7-15 digits, optional leading `+`) - e.g. `whatsrook 2348000000000`
3. `SESSION` environment variable

## Client Identity Emulation

whatsrook can emulate different WhatsApp client platforms, which determines how your session appears to WhatsApp. This is controlled using the `CLIENT` variable or the `-client` / `-c` flag. Valid values are `android` and `ios`; anything else (including unset) resolves to `default` (chrome-like behavior).

```bash
./bin/whatsrook -s 2348000000000 -client android
```

## Logging Out

Pass `-logout` / `-l` (or set `LOGOUT=true`/`1`) to remove the session's stored credentials and terminate:

```bash
whatsrook -s 2348000000000 -logout
```

## Updating

whatsrook can check for or apply updates via the `-update` / `-u` flag, or the `update` subcommand:

```bash
# Direct update
whatsrook -update
whatsrook update

# Check for updates only
whatsrook update check
whatsrook -update check

# Update to a specific channel
whatsrook update stable
whatsrook update beta
```

Update operations accept `check`, `stable`, or `beta`; any other value is treated as a direct update.

## Database & Session Isolation

whatsrook isolates session data so that multiple sessions can safely share a single database without interfering with one another:

- **SQLite**: A local single-file database (`whatsrook.db`). This is best suited for local testing and lightweight bots.
- **PostgreSQL**: Production-grade relational storage. All custom tables (`bot_settings`, `call_media_config`, `group_stats`, `bot_user_xp`, `bot_filters`, `bot_bgm`, `bot_sticker_cmds`, `cached_groups`) are scoped by `our_jid` composite primary keys, which allows multiple sessions to share the same database without any risk of data collision.

Example connection URLs:

```bash
# Local PostgreSQL
DATABASE_URL="postgresql://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"

# Managed PostgreSQL (e.g. Supabase, Render, Neon)
DATABASE_URL="postgresql://postgres.[ref]:[password]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

### Storage Drivers

#### SQLite (Default)

SQLite is an embedded single-file database (`whatsrook.db`) that requires no external database installation. It is ideal for local development, testing, and single-session bots.

```bash
DATABASE_URL="sqlite"
# or run without the -db-url flag
```

#### PostgreSQL (Production)

PostgreSQL is a high-concurrency, scalable relational database backend. It supports multi-session hosting on a single database instance with strict data isolation, and is compatible with the latest PostgreSQL releases and managed cloud providers such as Supabase, Neon, AWS RDS, and Render.

```bash
# Local PostgreSQL instance
DATABASE_URL="postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
```

### Step-by-Step: Supabase Setup with Session Pooler

Supabase provides a free, fully managed PostgreSQL database with Supavisor connection pooling. whatsrook requires Session Pooler mode for prepared statements, advisory locks, and long-lived socket persistence.

#### Step 1: Create a Supabase Project

1. Go to supabase.com and sign in or create a free account.
2. In the dashboard, click New Project and select your organization.
3. Fill in the project details:
   - Name: e.g., `whatsrook-db`
   - Database Password: A strong password should be set and saved securely.
   - Region: A region geographically close to your bot server should be chosen for optimal latency.
4. Click Create new project and wait a few moments for provisioning to finish.

#### Step 2: Obtain the Session Pooler Connection String

1. In the Supabase project dashboard, click on Project Settings (gear icon) in the bottom-left sidebar.
2. Select Database under the Configuration section.
3. Scroll to the Connection Pooling (or Connection string) section.
4. Under the URI tab, the Mode dropdown should be changed to Session. Note the pooler host (e.g., `aws-0-[region].pooler.supabase.com`) and port (`5432` or `6543`).
5. Copy the generated URI:
   ```
   postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require
   ```
6. Replace `[YOUR-PASSWORD]` with the database password set in Step 1.

#### Step 3: Configure whatsrook

The copied connection URL should be added to your `.env` file, or exported directly:

```bash
# In your .env file
DATABASE_URL="postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

It can also be passed via the CLI flag:

```bash
whatsrook -s 2348000000000 -db-url "postgresql://postgres.[project-ref]:[YOUR-PASSWORD]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

whatsrook will automatically connect over TLS, apply all required schema migrations, and manage device sessions.

### Multi-Session Data Isolation

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

### Schema Migrations

Database migrations are split into two decoupled layers:

1. Protocol Core Migrations (`whatsmeow_version`): These are managed automatically by sqlstore (../wa-core/store/sqlstore/), and handle WhatsApp identity keys, prekeys, session crypto state, contacts, and message history.

2. whatsrook Migrations (`whatsrook_version`): These are managed automatically by migrations (../cmd/store/migrations.go), and run idempotent versioned schema upgrades (v1 through v7) for bot settings, cached groups, media configs, and composite indexes.

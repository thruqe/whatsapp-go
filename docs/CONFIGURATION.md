# Configuration Guide

whatsrook can be configured using environment variables from a `.env` file or through command line flags. The command line flags override environment variables.

## Configuration Reference

| Variable       | Flag     | Default  | Description                                                                                                       |
| -------------- | -------- | -------- | ----------------------------------------------------------------------------------------------------------------- |
| `SESSION`      | `-s`     | —        | Session identifier / phone number with country code.                                                              |
| `CLIENT`       | `-c`     | `chrome` | Target client identity platform: `chrome`, `android`, `ios`.                                                      |
| `PAIR`         | `-p`     | `false`  | Request an 8-character pairing code instead of scanning a QR code.                                                |
| `QRCODE`       | `-q`     | `false`  | Render a terminal ASCII QR code for initial authentication.                                                       |
| `DATABASE_URL` | `-db`    | `sqlite` | Database connection string (`sqlite` or `postgres://user:pass@host:5432/db?sslmode=disable`).                     |
| `REDIS_URL`    | `-redis` | —        | Redis cache connection string (`redis://[:password@]host:5432/0`). Defaults to a fast in-memory store if omitted. |
| `VERBOSE`      | `-v`     | `false`  | Enable structured debug logging (`slog.LevelDebug`).                                                              |
| `PORT`         | `-P`     | `3000`   | Local HTTP/WebSocket server listening port.                                                                       |

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

1. CLI flag `-db <url>`
2. Session-specific environment variable: `DATABASE_URL_<PHONE>`
3. Generic environment variables: `DATABASE_URL`, `POSTGRES_URL`, `DB_URL`
4. Default: `sqlite` (`whatsrook.db`)

## Authentication & Pairing Modes

whatsrook supports two ways to authenticate a session: a pairing code, or a QR code.

### 1. Pairing Code

The pairing code method does not require scanning anything. To use it, run with `-p` or `PAIR=true` alongside your session phone number:

```bash
whatsrook -s 2348000000000 -p
```

whatsrook will generate an 8-character pairing code (e.g. `ABCD-1234`). Enter this code on your phone under WhatsApp > Linked Devices > Link with phone number.

### 2. QR Code (Terminal ASCII)

The QR code method renders a scannable code directly in your terminal. To use it, run with `-q` or `QRCODE=true`:

```bash
whatsrook -s 2348000000000 -q
```

An ASCII QR code will be displayed in your terminal, ready to scan using the WhatsApp mobile app.

## Client Identity Emulation

whatsrook can emulate different WhatsApp client platforms, which determines how your session appears to WhatsApp. This is controlled using the `CLIENT` variable or the `-c` flag:

- `chrome` (default): WhatsApp Web on Google Chrome.
- `android`: WhatsApp Web paired via Android device identity.
- `ios`: WhatsApp Web paired via iOS device identity.

```bash
./bin/whatsrook -s 2348000000000 -c chrome
```

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

For more detailed information about the database configuration, please check out this [doc](./database.md).

## HTTP & WebSocket API

whatsrook exposes an HTTP and WebSocket server on the configured `PORT` (default `3000`). This server provides the following endpoints:

- `GET /ws`: A WebSocket endpoint for bidirectional real-time event streaming and bot control, using JSON framing.
- `GET /health`: A healthcheck endpoint.

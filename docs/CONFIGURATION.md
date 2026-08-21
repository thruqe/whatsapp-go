# WhatsRook Configuration Guide

WhatsRook can be configured using environment variables (e.g. from a `.env` file) or through command-line interface (CLI) flags. CLI flags take precedence over environment variables.

## Configuration Reference

| Environment Variable | CLI Flag          | Default  | Description |
| -------------------- | ----------------- | -------- | ----------- |
| `SESSION`            | `-s, --session`   | —        | Session identifier / phone number with country code (e.g. `2348000000000`). |
| `CLIENT`             | `-c, --client`    | `chrome` | Target client identity platform: `chrome`, `android`, `ios`. |
| `PAIR`               | `-p, --pair`      | `false`  | Request an 8-character pairing code instead of scanning a QR code. |
| `QRCODE`             | `-q, --qrcode`    | `false`  | Render terminal ASCII QR code for initial authentication. |
| `DATABASE_URL`       | `-db, --database` | `sqlite` | Database connection string (`sqlite` or `postgres://user:pass@host:5432/db?sslmode=disable`). |
| `VERBOSE`            | `-v, --verbose`   | `false`  | Enable structured debug logging (`slog.LevelDebug`). |
| `PORT`               | `-P, --port`      | `3000`   | Local HTTP/WebSocket server listening port. |

## Per-Session Database URL Override

When managing multiple sessions, each session can point to a distinct database instance or share a single PostgreSQL database with isolated schemas.

To override the database connection for a specific session without modifying the global `DATABASE_URL`, set `DATABASE_URL_<PHONE>`:

```bash
# Global fallback
DATABASE_URL="postgres://user:pass@localhost:5432/whatsrook_shared?sslmode=disable"

# Dedicated database for session 2348060000000
DATABASE_URL_2348060000000="postgres://user:pass@db2.example.com:5432/isolated_session?sslmode=disable"
```

Resolution order:
1. CLI flag `-db, --database <url>`
2. Session-specific environment variable: `DATABASE_URL_<PHONE>`
3. Generic environment variables: `DATABASE_URL`, `POSTGRES_URL`, `DB_URL`
4. Default: `sqlite` (`whatsrook.db`)

## Authentication & Pairing Modes

### 1. Pairing Code (Recommended for Headless / Remote Servers)

Run with `--pair` or `PAIR=true` alongside your session phone number:
```bash
./bin/whatsrook -s 2348000000000 -p
```
WhatsRook will output an 8-character pairing code (e.g. `ABCD-1234`) to enter on your phone in WhatsApp > Linked Devices > Link with phone number.

### 2. QR Code (Terminal ASCII)

Run with `--qrcode` or `QRCODE=true`:
```bash
./bin/whatsrook -s 2348000000000 -q
```
An ASCII QR code will be rendered in the terminal for direct scanning.

## Client Identity Emulation

WhatsRook can emulate different WhatsApp client platforms. Set `CLIENT` or `-c, --client`:
- `chrome` (default): WhatsApp Web on Google Chrome.
- `android`: WhatsApp Web paired via Android device identity.
- `ios`: WhatsApp Web paired via iOS device identity.

```bash
./bin/whatsrook -s 2348000000000 -c chrome
```

## Database & Session Isolation

WhatsRook isolates session data when multiple sessions share a database:
- **SQLite**: Local single-file database (`whatsrook.db`). Best for local testing and lightweight bots.
- **PostgreSQL**: Production-grade relational storage. All custom tables (`bot_settings`, `call_media_config`, `group_stats`, `bot_user_xp`, `bot_filters`, `bot_bgm`, `bot_sticker_cmds`, `cached_groups`) are scoped by `our_jid` composite primary keys so multiple sessions can share the database without data collision.

Example connection URLs:
```bash
# Local PostgreSQL
DATABASE_URL="postgresql://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"

# Managed PostgreSQL (e.g. Supabase, Render, Neon)
DATABASE_URL="postgresql://postgres.[ref]:[password]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require"
```

## HTTP & WebSocket API

WhatsRook exposes an HTTP and WebSocket server on the configured `PORT` (default `3000`):
- `GET /ws`: WebSocket endpoint for bidirectional real-time event streaming and bot control using JSON framing.
- `GET /health`: Healthcheck endpoint.

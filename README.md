# WhatsRook

> [!CAUTION]
> **Educational project only.** Review [DISCLAIMER](DISCLAIMER.md) before use.

Connect to your WhatsApp and manage it programmatically.

## Key Links

- [Documentation](https://thruqe.github.io/whatsrook-docs/)
- [Deployment Console](https://wha-console.onrender.com)
- [Telegram Channel](https://t.me/whatsrook)

## Features

- Send and receive messages (text, media, reactions, polls)
- Download and decrypt incoming media (images, audio, video, documents)
- Manage group participants, metadata, and permissions
- Interact with WhatsApp channels and story status updates
- Schedule messages and event-driven automated workflows
- Set presence states, typing indicators, and read receipts

## Configuration

Configure WhatsRook via environment variables (e.g., `.env`) or CLI flags:

| Environment Variable | CLI Flag          | Default  | Description                                                         |
| -------------------- | ----------------- | -------- | ------------------------------------------------------------------- |
| `SESSION`            | `-s, --session`   | —        | Target phone number with country code (e.g., `2348000000000`)       |
| `CLIENT`             | `-c, --client`    | `chrome` | Target client identity platform: `chrome`, `android`, `ios`         |
| `PAIR`               | `-p, --pair`      | `false`  | Request an 8-character pairing code instead of QR code              |
| `QRCODE`             | `-q, --qrcode`    | `false`  | Render terminal ASCII QR code for initial authentication            |
| `DATABASE_URL`       | `-db, --database` | `sqlite` | Connection string (`postgres://user:pass@host:5432/db` or `sqlite`) |
| `VERBOSE`            | `-v, --verbose`   | `false`  | Enable structured debug logging                                     |
| `PORT`               | `-P, --port`      | `3000`   | Local HTTP/WebSocket server listening port                          |

## Database & Storage

WhatsRook defaults to embedded SQLite for local prototyping, but uses PostgreSQL for production workloads.

- To provision managed storage, configure a free tier on [Supabase](https://supabase.com) and assign the connection string to `DATABASE_URL`.
- For advanced schema and migration behavior, review the [Database & Storage Guide](https://thruqe.github.io/whatsrook-docs/DATABASE).

## Contributing & Governance

- **Contributing:** Please review the [Code of Conduct](https://www.google.com/search?q=CODE_OF_CONDUCT.md) before submitting pull requests.
- **Disclaimer:** Review the full liability terms in [DISCLAIMER](DISCLAIMER.md).
- **License:** Distributed under the [MIT License](https://www.google.com/search?q=LICENSE).

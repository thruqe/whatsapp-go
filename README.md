> [!CAUTION]
> Educational project only. See [DISCLAIMER.md](DISCLAIMER.md) before use.

Connect your app to WhatsApp and receive live events — messages, groups, stories, channels — then send actions back programmatically.

Read [Documentation](https://thruqe.github.io/whatsrook-docs/) to get started, find your way through, and deploy your own copy.

If you find WhatsRook useful, please consider **starring and [forking](https://github.com/Thruqe/whatsrook/fork)** the repository! It helps support and motivate further development.

[![Free Hosting](https://img.shields.io/badge/Free-Hosting-26A5E4?style=for-the-badge&logo=render&logoColor=white)](https://wha-console.onrender.com)
[![Join Telegram Channel](https://img.shields.io/badge/Telegram-Channel-26A5E4?style=for-the-badge&logo=telegram&logoColor=white)](https://t.me/whatsrook)

## Features

- Real-time event streaming (messages, groups, stories, channels)
- Bidirectional communication — receive events, dispatch actions
- Build bots, automations, and integrations on top of WhatsApp
- Powered by whatsmeow (no browser automation, no Puppeteer)

## Configuration

WhatsRook can be configured via environment variables (e.g. `.env` file) or CLI flags:

| Variable       | Flag              | Default  | Description                                                               |
| :------------- | :---------------- | :------- | :------------------------------------------------------------------------ |
| `SESSION`      | `-s, --session`   | —        | Phone number used to identify the WhatsApp session (e.g. `2348000000000`) |
| `CLIENT`       | `-c, --client`    | `chrome` | Client platform: `chrome`, `android`, `ios`                               |
| `PAIR`         | `-p, --pair`      | `false`  | Request an 8-character pairing code                                       |
| `QRCODE`       | `-q, --qrcode`    | `false`  | Print QR code to terminal for scanning                                    |
| `DATABASE_URL` | `-db, --database` | `sqlite` | Database connection URL (`postgres://...` or `sqlite`)                    |
| `VERBOSE`      | `-v, --verbose`   | `false`  | Enable verbose logging                                                    |
| `PORT`         | `-P, --port`      | `3000`   | WebSocket/HTTP server port                                                |

## Database & Storage

WhatsRook uses PostgreSQL as its primary database engine, with automatic fallback to embedded SQLite.

Get a free managed PostgreSQL database at [Supabase](https://supabase.com) and set `DATABASE_URL` in your environment or `.env` file. For details, see the [Database & Storage Guide](https://thruqe.github.io/whatsrook-docs/DATABASE).

## Deployment

WhatsRook supports multiple deployment platforms including Pterodactyl, Heroku, Render, and Local Docker.

For step-by-step guides on deploying WhatsRook, see the [Deployment Documentation](https://thruqe.github.io/whatsrook-docs/DEPLOYMENT).

## Contributing

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Disclaimer

See [DISCLAIMER.md](DISCLAIMER.md) for full terms and limitations.

## License

MIT — see [LICENSE](LICENSE) for details.

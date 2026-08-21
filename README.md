# WhatsRook

> [!CAUTION]
> **Educational project only.** Review [DISCLAIMER](DISCLAIMER.md) before use.

A command-line tool and Go library to connect to WhatsApp.

## Resources

- [Deployment Console](https://wha-console.onrender.com)
- [Telegram Channel](https://t.me/whatsrook)

## Documentation

- [Features](docs/features.md)
- [Configuration](docs/CONFIGURATION.md)
- [Database & Storage](docs/database.md)
- [Contributing & Governance](docs/contributing.md)

## Quick Start

### Installation

#### Linux & macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Thruqe/whatsrook/master/scripts/installer/install.sh | bash
```

#### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/Thruqe/whatsrook/master/scripts/installer/install.ps1 | iex
```

#### Build from Source

```bash
task build
```

### Pair and Connect

Authenticate using an 8-character pairing code:
```bash
whatsrook -s 2348000000000 -p
```

Or scan an ASCII QR code:
```bash
whatsrook -s 2348000000000 -q
```

## Acknowledgements

WhatsRook is built upon and inspired by the incredible work of the open-source community:

- **[whatsmeow](https://github.com/tulir/whatsmeow)** — WhatsApp multi-device protocol library in Go.
- **[whatsapp-rust](https://github.com/oxidezap/whatsapp-rust)** — WhatsApp Web protocol and calling architecture in Rust.
- **[hypermeow](https://github.com/polymorfa/hypermeow)** — WhatsApp protocol enhancements and VoIP engine extensions.
- **[whatsapp-rust-bridge](https://github.com/oxidezap/whatsapp-rust-bridge)** — WhatsApp protocol bindings and media bridge.

## License

Distributed under the [MIT License](LICENSE).

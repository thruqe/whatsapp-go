![whatsrook](https://pub-687f7ee583cf4f339cab34d2b3a3871d.r2.dev/logo.svg)

_`whatsrook-externals` is the official suite of standalone external plugins for [WhatsRook](https://github.com/thruqe/whatsapp-go), built in Rust using [`whatsrook-sdk`](../sdk)._

[![CI](https://github.com/thruqe/whatsapp-go/actions/workflows/externals-ci.yml/badge.svg?branch=master)](https://github.com/thruqe/whatsapp-go/actions/workflows/externals-ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## Installation

With WhatsRook's platform-aware installer, run from any WhatsApp chat:

```text
.install <plugin>
```

To install every official plugin in one go:

```text
.install all
```

For prebuilt binaries, custom URLs, and building from source, see the [External Plugins Guide](../../docs/EXTERNAL_PLUGIN.md).

## Plugins

| Command | Plugin | Category | Description |
| :--- | :--- | :--- | :--- |
| `.weather` | `weather` | Utility | Real-time weather forecasts by city or coordinates |
| `.urban` | `urban` | Reference | Urban Dictionary definitions, examples, and votes |
| `.shorturl` | `shorturl` | Utility | URL shortener via TinyURL and is.gd |
| `.calc` | `calc` | Math | High-precision mathematical and scientific expression evaluator |
| `.fact` | `fact` | Fun | Random interesting facts with offline fallbacks |
| `.quotes` | `quotes` | Fun | Inspirational quotes and attributed authors |
| `.joke` | `joke` | Fun | Clean, witty jokes and punchlines |
| `.rizz` | `rizz` | Fun | Modern pickup lines and charismatic phrases |
| `.btc` | `btc` | Finance | Live Bitcoin price, 24h change, and halving countdown |
| `.markets` | `markets` | Finance | Forex, crypto, commodity, and index rates |
| `.news` | `news` | Media | AP News top headlines by country code |
| `.wabeta` | `wabeta` | WhatsApp | WhatsApp beta feature leaks from WABetaInfo |
| `.why` | `why` | AI | AI-powered deep search and reasoning from Why.com |
| `.ss` | `ss` | Utility | Full-page webpage screenshot capture |
| `.tts` | `tts` | Media | Google Text-to-Speech audio notes |
| `.qrcode` | `qrcode` | Utility | High-resolution QR code generator |
| `.fancy` | `fancy` | Styling | Converts text into 20+ decorative Unicode fonts |
| `.font` | `font` | Styling | Renders text in a specific numbered Unicode font |
| `.fonts` | `fonts` | Styling | Directory and visual samples of all font styles |
| `.git` | `git` | Developer | GitHub repo explorer, commits, releases, and ZIP downloads |
| `.mp4url` | `mp4url` | Media | Downloads and streams video from direct MP4 URLs |
| `.cpu` | `cpu` | System | Host CPU model, cores, threads, and load |
| `.memory` | `memory` | System | Host RAM usage and available memory |
| `.captcha` | `captcha` | Utility | Animated dial verification code video captchas |
| `.sticker` | `sticker` | Media | Converts media to a letterboxed 512×512 WebP sticker |
| `.circle` | `circle` | Media | Converts media to a circular-masked WebP sticker |
| `.crop` | `crop` | Media | Converts media to a square-cropped WebP sticker |
| `.take` | `take` | Media | Re-packs sticker metadata — author and pack name |
| `.media` | `media` | Media | Video/audio converter, audio extractor, and trimmer |

Full usage examples and command reference: [External Plugins Guide](../../docs/EXTERNAL_PLUGIN.md)

## Documentation

Full architectural documentation and usage instructions are available in [EXTERNAL_PLUGIN.md](../../docs/EXTERNAL_PLUGIN.md):

- 📖 **Architecture & Protocol** &mdash; Stdin/stdout NDJSON wire protocol, Request schema, Action frames, and ACK loops.
- 📦 **Installation Guide** &mdash; WhatsApp 1-click install (`.install <name>`), custom URLs, and local binaries.
- 🧩 **Plugin Catalog** &mdash; Command reference for all official plugins.
- 🛠️ [**Rust SDK**](../sdk) &mdash; Build custom plugins with `whatsrook-sdk` in Rust.

## Building from Source

```bash
cd externals
cargo build --release --workspace
```

Or from the monorepo root:

```bash
task build:externals
```

Requires Rust stable (1.85+) and `ffmpeg` on `PATH` for media and sticker plugins.

## Contributions

Contributions follow the monorepo [Contributing Guidelines](../../docs/CONTRIBUTING.md) and [Architecture Guidelines](../../docs/AGENTS.md).

## Acknowledgements

whatsrook-externals is part of the [WhatsRook](https://github.com/thruqe/whatsapp-go) ecosystem. All plugins are built with [`whatsrook-sdk`](../sdk).

## Licensing

This project is open source, see the [LICENSE](./LICENSE) file for full details.

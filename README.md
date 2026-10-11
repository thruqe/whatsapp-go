![whatsrook](https://pub-687f7ee583cf4f339cab34d2b3a3871d.r2.dev/logo.svg)

whatsrook is an automation bot that integrates seamlessly with your whatsapp linked devices, executing actions automatically under your direct command.

## Usage

[Session ID](https://thruqe.github.io/whatsapp-go/), this is allows you customize your enivorment into a cryptographic key, you can pass it as a parameter to the cli or in the `.env` file as `SESSION_ID` field.

Configuration is managed through environment variables; see the [sample environment file](./.env.example) for the supported settings.

External plugins can be built independently and installed as executable commands. Read the [external plugin setup documentation](./docs/EXTERNAL_PLUGIN.md) to learn how to create, build, install, and use them.

If you want to install on your system, see the [Installation Guide](./docs/INSTALLATION.md) to quickly install.

## Features

### End-to-End Encrypted Messaging & Rich Media Engine
- **Noise & Signal Protocol Compliance**: Native multi-device implementation with end-to-end encryption, protocol buffer serialization, and secure cryptographic key exchanges.
- **High-Performance Media Pipeline**: Download, convert, and deliver photos, videos, documents, audio voice notes (Opus PTT with native audio waveforms), and animated WebP stickers.
- **Optimized Sticker Studio**: Convert images and videos into WhatsApp stickers with custom EXIF metadata (pack name and author). Automatically caps animated video stickers to 10 seconds and compresses them under WhatsApp's 650 KB limit.
- **View-Once Vault**: Automatically intercept or resend expiring View-Once photos and videos (`.vv`, `.autoview`) directly into chat.

### Autonomous Agentic AI & Multilingual Intelligence
- **Proactive Tool-Using Meta AI**: Equipped with intent-to-tool routing that proactively executes bot commands (downloads, sticker creation, weather, translations, calculations) on the user's behalf.
- **Composed & Grounded Demeanor**: Relaxed, sensible, and natural conversational tone without hyperactivity, giggling, or underworld street slang across all languages.
- **Multilingual Support Across 8 Locales**: Complete localized catalogs and command descriptions in English (`en`), Spanish (`es`), Indonesian (`id`), Portuguese (`pt`), French (`fr`), Arabic (`ar`), Russian (`ru`), and German (`de`) with auto-detection from country calling codes (`.lang`).
- **Contextual Dialogue Memory**: Maintains recent conversational turns, quoted message context, and participant identity for intelligent replies in both DMs and group chats.

### Automated Group & Community Administration
- **Complete Participant Control**: Add, remove (`.kick`, `.kickall`), promote, and demote group members and admins with single commands.
- **Comprehensive Group Moderation**:
  - **Anti-Link**: Automatically detect and purge unauthorized group invite links or external URLs.
  - **Anti-Spam & Anti-Flood**: Mitigate rapid message floods and repetitive spam bursts.
  - **Anti-Word**: Filter and delete messages matching configurable banned word lists.
  - **Anti-Msg**: Automatically delete messages from restricted participants.
  - **CAPTCHA Verification**: Protect groups from automated bots by requiring new members to solve a quick verification challenge.
- **Scheduled Auto-Mute / Auto-Unmute**: Configure daily open and close schedules (`.automute`, `.autounmute`) tied to your local timezone (`.timezone`).
- **Activity Metrics & Warnings**: Track member participation stats (`.gstats`), issue formal warnings (`.warn`, `.unwarn`), and configure enforcement thresholds (`.setwarn`).
- **Custom Welcome & Goodbye Greetings**: Deliver automatic greeting and farewell messages when members join or depart.

### VoIP Calls, Audio Playout & Voicemail
- **Encrypted VoIP Engine**: Built-in SRTP, SRTCP, and Sframe encrypted calling stack with STUN/TURN ICE negotiation.
- **Anti-Call Protection**: Automatically reject incoming unsolicited voice or video calls (`.anticall`) to protect bot uptime and privacy.
- **Automated Voicemail**: Automatically answer unanswered calls with customizable audio voicemail responses (`.voicemail`).
- **Audio Cards & Media Playout**: Stream custom audio cards and external media during calls and alive queries.

### Embedded Scripting Runtimes (Grol & JavaScript)
- **Grol Language Runtime**: Embedded [Grol](https://grol.io/) (Go REPL Open Language) scripting engine (`.grol`) with per-chat persistent sessions, mathematical calculations, AST inspection (`-p`), and canonical code beautification (`-f`).
- **Live Context Evaluation**: Safely evaluate JavaScript and Grol code snippets (`.eval`) with live access to current message, sender, chat metadata, and bot client APIs.

### Modular Plugin & Extension Architecture
- **Dual Plugin Engine**: Fast compiled internal commands paired with decoupled out-of-process standalone executable plugins.
- **Standalone Addon SDK**: Build custom plugins in Go or any language using the lightweight JSON-RPC standard I/O protocol (`pkg/addons/sdk`).
- **29 Official Plugins**: Built-in catalog of plugins including weather forecasts, currency/crypto tickers (`.btc`, `.markets`), calculations (`.calc`), text-to-speech (`.tts`), QR code generation, screenshots (`.ss`), URL shorteners, news, and jokes.
- **Dynamic Plugin Lifecycle**: Install, inspect, and update plugins on the fly directly from GitHub releases without restarting the bot (`.install`, `.plist`, `.uninstall`).

### Deep Customization & Interactive Menu
- **Interactive Visual Menu**: Browse commands grouped by category (`.menu`) with custom thumbnails and detailed per-command help cards (`.help <command>`).
- **Customizable Alive Identity**: Personalize your `.alive` card with custom message templates, dynamic placeholders (`{uptime}`, `{latency}`, `{ram}`, `{goroutines}`, `{fact}`, `{quote}`), custom images, and audio cards.
- **Custom Sticker Command Triggers**: Bind stickers to trigger specific bot commands automatically (`.setcmd`, `.delcmd`).
- **Configurable Command Prefixes**: Customize single or multi-character command prefixes (`.prefix`).

### Privacy, Security & Operational Modes
- **Public & Private Modes**: Instantly toggle between public community mode and owner-only private mode (`.mode`).
- **Sudo & Role Authorization**: Multi-tier permission model distinguishing between regular members, group admins, sudoers, and the bot owner (`.setsudo`, `.delsudo`, `.listsudo`).
- **Granular Privacy Controls**: Manage WhatsApp privacy settings (Last Seen, Profile Photo, and Read Receipts) directly from chat commands (`.privacy`).
- **Ultra-Low Resource Footprint**: Single self-contained Go binary compiled with `-trimpath` and stripped symbols; operates smoothly on minimal RAM (<30 MB) and negligible CPU usage on low-power devices, VPS instances, or Raspberry Pi.

## Contributions

If you want to help make this project better, please take the time to read this contribution [doc](./docs/CONTRIBUTING.md) and [fork](https://github.com/thruqe/whatsapp-go/fork) this repository. Then open a pull request with your changes.

If you are using any AI agent for assistance, please refer to the [AGENTS documentation](./docs/AGENTS.md) Guide.

## Acknowledgements

whatsrook wouldn't have been possible without these open source, community and passion-driven projects: [whatsmeow](https://github.com/tulir/whatsmeow), [whatsapp-rust](https://github.com/oxidezap/whatsapp-rust), and the broader WhatsApp and Go ecosystem.

## Licensing

This project is open source, see the [LICENSE](./LICENSE) file for full details.

## Support this project

If you found this project to be useful in anyway, please consider sending a tip.

<a href="https://etherscan.io/address/0x76a5307e573401f2b7189fb721a33dc483e985a8"><img src="https://cdn-icons-png.flaticon.com/128/15301/15301597.png" width="24" height="24" alt="Donate ETH" /></a> `0x76a5307e573401f2b7189fb721a33dc483e985a8`

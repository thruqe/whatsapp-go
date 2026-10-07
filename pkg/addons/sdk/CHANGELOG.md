# Changelog

All notable changes to `whatsrook-sdk` are documented here.

This project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.1] — 2026-09-28

### Added

- `fmt` module with WhatsApp Markdown formatting functions (`bold`, `italic`, `strikethrough`, `monospace`, `code_block`, `quote`, `bullet_list`, `numbered_list`) and fluent `MessageBuilder`.
- `media` module with pure-Rust RFC 4648 Base64 encoding/decoding (`encode_base64`, `decode_base64`), data URL helpers (`to_data_url`), and disk-to-data-URL utilities (`read_file_as_base64`, `read_file_as_data_url`).
- Fluent constructors and method chaining on `Action`: `Action::reply`, `Action::image`, `Action::audio`, `Action::video`, `Action::document`, `Action::sticker`, `Action::poll`, `Action::loader`, `Action::done`, `.with_caption()`, `.with_mimetype()`, `.as_ptt()`, `.as_gif()`, `.with_selectable()`, and `.send()`.
- Enhanced `Request` methods: `arg(index)`, `arg_as::<T>(index)`, `subcommand()`, `subcommand_args()`, `flag(name)`, `flag_value(name)`, `is_dm()`, `is_live()`, `is_cancel()`, `sender_phone()`, `chat_target()`, `chat_id()`, `sender_id()`, `mentioned_jids()`.
- Extended action helpers: `send_react_to`, `send_multi_poll`, `send_image_with_type`, `send_document_full`, `send_video_full`, `send_audio_full`.

---

## [0.1.0] — 2026-09-22

### Added

- `Request` struct — deserializes the full WhatsRook JSON payload from `stdin`, with ergonomic accessor methods (`query`, `prefix`, `bot_name`, `push_name`, `quoted_text`, `quoted_sender`, `quoted_id`, `is_group`, `is_sudo`, `is_owner`, `is_admin`).
- `QuotedMessage` struct — quoted/replied-to message context (`id`, `sender`, `text`).
- `Action` enum — all supported action frames: `Reply`, `Edit`, `React`, `Delete`, `SendImage`, `SendAudio`, `SendVideo`, `SendDocument`, `SendSticker`, `Poll`, `Loader`, `Done`.
- `Ack` struct — acknowledgement payload returned by WhatsRook after live-reply actions.
- `send_action` — low-level frame writer; serializes an `Action` to `stdout` and flushes immediately.
- `await_ack` — reads and deserializes one acknowledgement line from `stdin`.
- `send_reply_live` — sends a `reply` frame and returns the `msg_id` for subsequent in-place edits.
- `send_edit_live` — sends an `edit` frame to update a previously sent message.
- `send_react` — reacts to the triggering message with an emoji.
- `send_delete` — revokes/deletes a message by ID.
- `send_image` — sends an image from a URL or base64 data.
- `send_audio` — sends audio or a voice note (PTT).
- `send_video` — sends a video or GIF.
- `send_document` — sends a document file.
- `send_sticker` — sends a WebP sticker.
- `send_poll` — sends an interactive poll.
- `send_loader` — shows a typing/processing indicator with optional status text.
- `send_done` — signals session completion.
- `respond` — simple-mode plain-text reply to `stdout`.
- `respond_err` — writes error to `stderr` and `stdout`, then exits with code 1.
- `create_http_client` — builds a preconfigured blocking `reqwest` client with timeouts and a standard browser User-Agent.
- Re-export of `reqwest` and `reqwest::blocking::Client as HttpClient` for convenience.
- Runnable examples: `hello`, `live_ticker`, `media_reply`.

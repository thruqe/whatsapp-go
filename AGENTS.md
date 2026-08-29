# AGENTS Architecture & Guidelines

Welcome to WhatsRook.

## Core Architecture

- **Root Library (`whatsrook`)**: High-level Go abstraction cleanly wrapping `wa-core` (`Client`, session initialization, phone number pairing, QR channel streaming, session wipe helpers, and client lifecycle management).
- **Package Layout**:
  - `whatsrook`: Core library entrypoint and high-level client abstraction.
  - `whatsrook/src/logger`: Universal, high-performance Zap-based logger (`package Logger`) with multi-destination routing, per-level log files, and zero-allocation structured fields.
  - `whatsrook/wa-core`: WhatsApp protocol core library and VoIP call engine (`wacaller`), binary XML node encoding/decoding, socket engine, SRTP/SRTCP media pipeline, STUN NAT traversal, audio/video playout controllers, and database stores (`wa-core/store/sqlstore`).
  - `whatsrook/src`: Core protocol & messaging abstractions over `wa-core`, media transcode engine (JPEG/Opus/FFmpeg), and helper utilities.
  - `whatsrook/cmd`: WhatsApp bot CLI application, plugin commands (`cli/plugins`), dedicated TUI package (`cli/tui`: interactive Bubbletea setup wizard and live agentic dashboard), and consolidated CLI feature utilities (`cli/utils`: media downloaders, font styling, URL validators, prompts, timezones, Meta AI parsers, games).

# IMPORTANT

## Development Management & Must Use!

Utilize the [Taskfile](./Taskfile.yml)

## Relevant Documentation

- [README](./README.md)
- [Security](./SECURITY.md)

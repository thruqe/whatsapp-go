# AGENTS Architecture & Guidelines

Welcome to WhatsRook.

> **CRITICAL ENFORCEMENT NOTICE**  
> Every instruction, guideline, and directive outlined in this document is a **STRICT REQUIREMENT (MUST DO)**. Zero exceptions or unauthorized deviations are permitted.

## Core Architecture

- **Root Library (`whatsrook`)**: High-level Go abstraction cleanly wrapping `wa-core` (`Client`, session initialization, phone number pairing, QR channel streaming, session wipe helpers, and client lifecycle management).
- **Package Layout**:
  - `whatsrook`: Core library entrypoint and high-level client abstraction.
  - `whatsrook/src/logger`: Universal, high-performance Zap-based logger (`package Logger`) with multi-destination routing, per-level log files, and zero-allocation structured fields.
  - `whatsrook/wa-core`: WhatsApp protocol core library and VoIP call engine (`wacaller`), binary XML node encoding/decoding, socket engine, SRTP/SRTCP media pipeline, STUN NAT traversal, audio/video playout controllers, and database stores (`wa-core/store/sqlstore`).
  - `whatsrook/src`: Core protocol & messaging abstractions over `wa-core`, media transcode engine (JPEG/Opus/FFmpeg), and helper utilities.
  - `whatsrook/cmd`: WhatsApp bot CLI application, plugin commands (`cli/plugins`), dedicated TUI package (`cli/tui`: interactive Bubbletea setup wizard and live agentic dashboard), and consolidated CLI feature utilities (`cli/utils`: media downloaders, font styling, URL validators, prompts, timezones, Meta AI parsers, games).

## Mandatory Engineering & Contribution Directives

Before modifying, refactoring, or introducing new code, you **MUST**:

- **Perform Contextual & Structural Analysis**: Inspect existing code patterns, package composition, formatting paradigms, and architectural conventions across the codebase. Mirror the project's established design idioms.
- **Enforce Documentation & Style Adherence**: Read [`README.md`](./README.md) thoroughly. Internalize and strictly adhere to its documentation tone, compositional structure, and technical writing style.
- **Strictly Follow Contribution & Commit Standards**: Read and comply with [`CONTRIBUTING.md`](./CONTRIBUTING.md). All git commit messages, branch workflows, and pull request conventions must strictly follow the rules defined there.
- **Implement Modern Go Standards**: Perform web searches to identify and apply the latest Go syntax, standard library features, and modern idioms. Strictly avoid legacy or deprecated patterns unless backward compatibility is explicitly required.
- **Verify Operational Flow**: Trace dependencies and verify component behavior directly against source implementations before executing changes.

# IMPORTANT

## Development Management & Must Use!

Utilize the [Taskfile](./Taskfile.yml)

## Relevant Documentation

- [README](./README.md)
- [Contributing Guidelines](./CONTRIBUTING.md)
- [Security](./SECURITY.md)

# Security Policy

WhatsRook is a high-performance WhatsApp client library, VoIP engine, and bot platform built in Go. Because WhatsRook manages cryptographic sessions, real-time messaging, VoIP media pipelines, background database persistence, and privileged automation workflows, security and reliability are core requirements.

---

## 1. Supported Versions

Security updates and vulnerability patches are actively maintained for the following versions:

| Version | Supported          | Notes                                     |
| :------ | :----------------- | :---------------------------------------- |
| `18.x`  | :white_check_mark: | Current active release branch             |
| `< 18.0`| :x:                | End of Life (upgrade to latest `18.x`)    |

---

## 2. Core Security Architecture & Threat Model

### A. Session & Cryptographic Key Protection
- **Signal Protocol & E2EE**: WhatsRook implements the Signal Protocol (Double Ratchet, Curve25519 pre-keys, signed identity keys, and sender keys) for end-to-end encryption.
- **Session Persistence**: Session tokens, pre-keys, and client identities are stored via `sqlstore` (PostgreSQL or SQLite). 
  - File-based SQLite databases and session directories (`./auth/<phone_number>/`) must have strict POSIX permissions (`0700` for directories, `0600` for files).
  - Database connection strings (`DATABASE_URL`) containing passwords must be supplied securely via environment variables or secret managers.
  - On device unlinking or session invalidation, use the built-in session wipe helpers to purge sensitive local keys.

### B. VoIP & Real-Time Media Security (`wa-core/wacaller`)
- **SRTP/SRTCP Encryption**: Real-time audio and video call streams are encrypted using SRTP/SRTCP with session keys negotiated over WhatsApp's encrypted signaling channel.
- **NAT Traversal & STUN**: STUN binding requests and ICE candidates are validated to prevent IP spoofing or unauthorized relay manipulation.
- **Buffer Safety**: Multi-party group audio mixers and playout controllers enforce bounded ring buffers to mitigate memory exhaustion or denial-of-service (DoS) from malformed RTP packets.

### C. Web API & WebSocket Hub (`cli/api.go`)
- **Protobuf Binary Framing**: The built-in HTTP/WebSocket control server uses Google Protocol Buffers for structured binary messaging.
- **Origin & Access Controls**: WebSocket upgrades should restrict origins in production environments (`InsecureSkipVerify` must remain `false` outside local development).
- **Transport Security**: Expose the HTTP/WebSocket API behind a TLS-terminating reverse proxy (e.g., Nginx, Caddy, Cloudflare) when deployed remotely.

### D. Privileged Command & Sudo Execution (`cli/plugins/owner.go`)
- **Sudoers Authorization**: Highly privileged actions (e.g., interactive shell execution `sh`/`exec`, bot restart, runtime updates, contact blocking) are strictly restricted to authenticated sudoer numbers and the bot owner.
- **Interactive Session Isolation**: Shell execution processes are isolated per chat context, subject to execution timeouts, and can be terminated immediately via `stop`/`kill`.
- **Command Sanitization**: External process invocations sanitize arguments to prevent shell injection or unescaped parameter expansion.

### E. Media Transcoding & File Processing (`utils/transcoder.go`)
- **FFmpeg & Media Pipelines**: Transcoding operations (Opus audio conversion, WebP sticker generation, JPEG image processing) run with constrained resource limits and execution deadlines.
- **Path Traversal Prevention**: File caching and download handlers validate paths and disallow relative path traversal sequences (`../`).
- **SSRF Mitigation**: Downloader plugins and external media fetchers validate URLs and protocol schemes before issuing outbound HTTP requests.

### F. Plugin Sandboxing & Group Protections
- **Role-Based Access Control (RBAC)**: Commands are categorized with strict permission boundaries (Owner, Sudoers, Group Admins, Bot Admins, Public).
- **Panic Isolation**: Plugin execution errors and panics are recovered within the dispatch pipeline to ensure an individual plugin failure cannot crash the client runtime.
- **Abuse Prevention Filters**: Group managers include configurable anti-link, anti-spam, auto-mute, and flood rate-limiting rules.

---

## 3. Data Privacy & Zero-Leak Logging

- **Zero Plaintext Secrets**: Never hardcode, commit, or log credentials, API keys, private identity keys, Noise protocol keys, or SRTP master keys.
- **Structured Log Safety (`utils/logger.go`)**: 
  - Multi-level logging writes to segmented log files (`debug.log`, `info.log`, `warn.log`, `error.log`).
  - Debug logs must never expose sensitive user payloads, private keys, or credentials.
  - In public logs, sanitize phone numbers, JIDs, and auth tokens.

---

## 4. Memory Safety & Stability

WhatsRook is designed for long-running production deployments:
- **Goroutine & Context Lifecycles**: All network connections, media playout loops, and periodic schedulers must bind to a cancellable `context.Context` to prevent goroutine leaks on reconnects or call terminations.
- **Buffer Recycling**: Memory buffers pooled via `sync.Pool` have capacity thresholds to prevent heap bloat from sporadic large payloads.
- **Resource Cleanup**: Always close database connections, file handles, RTP sockets, and HTTP response bodies upon completion.

---

## 5. Acceptable Use Policy

WhatsRook interacts directly with real users on WhatsApp. Contributions or deployments that violate these principles are strictly forbidden:

- **No Phishing or Social Engineering**: Developing deceptive workflows, credential harvesters, or pretexting bots will result in an immediate ban from the project.
- **No Surveillance or Stalkerware**: Do not build features intended to covertly track, stalk, or record users without explicit consent.
- **No Unsolicited Mass Spamming**: Do not use WhatsRook for automated unsolicited bulk messaging, advertising, or harassment.
- **Compliance**: Users are responsible for adhering to WhatsApp's Terms of Service and applicable privacy regulations (e.g., GDPR, CCPA).

---

## 6. Reporting a Vulnerability

We take the security of WhatsRook seriously. If you discover a security vulnerability, please disclose it responsibly.

### How to Report
- **Preferred Method**: Submit a private advisory via [GitHub Security Advisories](https://github.com/Thruqe/whatsrook/security/advisories/new).
- **Alternative**: Contact the maintainers directly via email or private channel as listed on [Thruqe's GitHub Profile](https://github.com/Thruqe).

> [!IMPORTANT]
> Please **DO NOT** open public GitHub issues, discussions, or pull requests for undisclosed security vulnerabilities.

### What to Include
To help us triage and resolve the issue quickly, please include:
1. Description of the vulnerability and its potential impact.
2. Steps to reproduce the issue (proof-of-concept code, payloads, or CLI commands).
3. Affected components (e.g., `wa-core/wacaller`, `cli/plugins`, `utils/transcoder`, `sqlstore`).
4. Affected versions and runtime environment (OS, Go version, database engine).

### Response Timeline
- **Acknowledgment**: Within 48 hours of receipt.
- **Assessment & Triage**: Within 5 business days.
- **Fix & Disclosure**: Coordinated release and disclosure date agreed upon with the reporter once a fix is verified.

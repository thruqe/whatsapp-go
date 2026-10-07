# Security Policy

## 1. Credentials and Secrets

Never embed API keys, tokens, or any authentication data in plugin binaries or source code. Pass secrets via environment variables or a platform secret manager.

## 2. Memory Safety and Stability

Rust's ownership model enforces memory safety at compile time. Contributions must not introduce `unsafe` blocks without a clear justification and review. All allocations should be bounded; unbounded growth in a long-running plugin is a bug.

## 3. No Social Engineering

whatsrook-sdk is a tool for legitimate WhatsApp automation. Contributions or usage that enable phishing flows, pretexting, or any logic designed to deceive people into giving up private information are not allowed and will be rejected.

## 4. Acceptable Use

Plugins built with this SDK communicate directly with real people over WhatsApp. This capability must be treated responsibly.

Plugins must not be used to:

- Stalk or covertly monitor someone.
- Harass, threaten, or send abusive content.
- Send unsolicited spam or malicious payloads.

## Reporting a Vulnerability

If a security issue is found in this crate, please contact [thruqe@gmail.com](mailto:thruqe@gmail.com) rather than disclosing it publicly.

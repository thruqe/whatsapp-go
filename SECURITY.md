# Security Policy

## 1. Credentials and Secrets

Never commit or hardcode credentials — API keys, session tokens, auth data, or anything sensitive. Don't log them in plaintext either.

- Use environment variables or a proper secret manager for config.
- Store persisted auth data (sessions, tokens) encrypted at rest.

## 2. Memory Safety and Stability

WhatsRook runs as a long-lived messaging client, so contributions need to be memory-safe:

- No memory leaks or unbounded resource growth
- No unchecked pointer dereferences
- Proper cleanup of connections, goroutines, and object lifecycles

Code that could degrade performance or stability over long uptimes won't be merged as-is.

## 3. No Social Engineering

WhatsRook exists to enable legitimate automation, not deception. Contributions or usage that build phishing flows, pretexting, or any logic designed to trick people into giving up private information are not allowed and will be rejected.

## 4. Acceptable Use

WhatsRook talks directly to real people over WhatsApp — treat that responsibly.

Do not use it to:

- Stalk or covertly monitor someone
- Harass, threaten, or send abusive content
- Send unsolicited spam or malicious payloads

## Reporting a Vulnerability

If you find a security issue, please open an issue or contact the maintainers directly rather than disclosing it publicly.

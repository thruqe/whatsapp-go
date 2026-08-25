# Security Policy

## 1. Credentials and Secrets

Never commit or hardcode credentials, such as API keys, session tokens, or authentication data. This kind of information must not be logged in plaintext either.

- Environment variables or a proper secret manager should be used for configuration.
- Persisted authentication data, such as sessions and tokens, must be stored encrypted at rest.

## 2. Memory Safety and Stability

whatsrook runs as a long-lived messaging client, so contributions need to be memory-safe. This means:

- No memory leaks or unbounded resource growth.
- No unchecked pointer dereferences.
- Proper cleanup of connections, goroutines, and object lifecycles.

Code that could degrade performance or stability over long uptimes will not be merged as it is.

## 3. No Social Engineering

whatsrook exists to enable legitimate automation, not deception. Contributions or usage that build phishing flows, pretexting, or any logic designed to trick people into giving up private information are not allowed and will be rejected.

## 4. Acceptable Use

whatsrook communicates directly with real people over WhatsApp, so this capability must be treated responsibly.

It must not be used to:

- Stalk or covertly monitor someone.
- Harass, threaten, or send abusive content.
- Send unsolicited spam or malicious payloads.

## Reporting a Vulnerability

If a security issue is found, please contact this [email](mailto:thruqe@outlook.com) rather than disclosing it publicly.

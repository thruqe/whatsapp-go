# Diagnostics

`diag` contains the opt-in diagnostic recorder for the `caller` VoIP stack.

Library users can attach a `*diag.Recorder` with `caller.WithDiagnostics`.
The recorder writes one JSONL file per stream so keying, relay, RTP, media, and
call-state events remain independently searchable.

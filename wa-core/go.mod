module go.mau.fi/whatsmeow

go 1.27.0

replace whatsrook => ../

retract (
	v0.1.1 // Retraction carrier only; use @main.
	v0.1.0 // Published accidentally; use @main.
	v0.0.0 // Historical initial tag; use @main.
)

require (
	github.com/beeper/argo-go v1.1.2
	github.com/coder/websocket v1.8.15
	github.com/google/uuid v1.6.0
	github.com/hajimehoshi/go-mp3 v0.3.4
	github.com/pion/datachannel v1.6.2
	github.com/pion/dtls/v3 v3.1.5
	github.com/pion/logging v0.2.4
	github.com/pion/opus v0.1.0
	github.com/pion/sctp v1.11.1
	github.com/polymorfa/libsignal-protocol-go v0.2.3-0.20260806162910-a2adef2e8a11
	github.com/rs/zerolog v1.35.1
	go.mau.fi/util v0.9.12-0.20260717235539-f9ffa7eca58d
	golang.org/x/crypto v0.54.0
	golang.org/x/net v0.57.0
	golang.org/x/sync v0.22.0
	google.golang.org/protobuf v1.36.11
	whatsrook v0.0.0-00010101000000-000000000000
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/petermattis/goid v0.0.0-20260713124913-97594f28f5ca // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/transport/v4 v4.0.2 // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/exp v0.0.0-20260709172345-9ea1abe57597 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

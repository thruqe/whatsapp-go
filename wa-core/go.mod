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
	go.mau.fi/util v0.10.0
	go.uber.org/zap v1.28.0
	golang.org/x/crypto v0.55.0
	golang.org/x/net v0.58.0
	golang.org/x/sync v0.22.0
	google.golang.org/protobuf v1.36.12
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.1 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/petermattis/goid v0.0.0-20260820044319-269ab09b5261 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/transport/v4 v4.1.0 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp v0.0.0-20260824195058-e88cd73687aa // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

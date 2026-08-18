package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestInstallCallAckHookMatchesPinnedUpstreamLayout(t *testing.T) {
	client := NewClient(&store.Device{}, waLog.Noop)
	eng := newEngine(client)
	if err := eng.installCallAckHook(); err != nil {
		t.Fatalf("install raw call adapter: %v", err)
	}
}

func TestInstallCallAckHookRejectsMissingClient(t *testing.T) {
	eng := newEngine(nil)
	if err := eng.installCallAckHook(); err == nil {
		t.Fatal("raw call adapter accepted a missing client")
	}
}

func TestGroupFeaturesRejectUnavailableRawAdapter(t *testing.T) {
	client := &Client{callLogger: zerolog.Nop()}
	eng := newEngine(client)
	eng.rawCallHookErr = errors.New("upstream layout changed")
	if _, err := eng.placeGroupCall(
		context.Background(),
		[]string{"1", "2"},
		GroupCallOptions{},
	); err == nil {
		t.Fatal("group call continued without its raw call adapter")
	}
}

func TestInstallCallAckHookRoutesCallAck(t *testing.T) {
	client := NewClient(&store.Device{}, waLog.Noop)
	eng := newEngine(client)
	if err := eng.installCallAckHook(); err != nil {
		t.Fatalf("install raw call adapter: %v", err)
	}

	call := &Call{eng: eng, id: "CALL_123", phase: CallPhaseCalling}
	eng.calls["CALL_123"] = &engineCall{
		call:      call,
		direction: CallDirectionOutgoing,
	}

	ackNode := &waBinary.Node{
		Tag: "ack",
		Attrs: waBinary.Attrs{
			"class": "call",
			"id":    "MSG_123",
		},
		Content: []waBinary.Node{
			{
				Tag: "relay",
				Attrs: waBinary.Attrs{
					"call-id": "CALL_123",
				},
				Content: []waBinary.Node{
					{
						Tag: "te2",
						Attrs: waBinary.Attrs{
							"relay_id": "1",
						},
						Content: []byte{127, 0, 0, 1, 0x1f, 0x90},
					},
				},
			},
		},
	}

	if client.RawNodeHandler == nil {
		t.Fatal("RawNodeHandler was not set")
	}

	_, drop := client.RawNodeHandler(context.Background(), ackNode)
	if drop {
		t.Fatal("RawNodeHandler unexpectedly dropped ack node")
	}

	eng.mu.Lock()
	m := eng.calls["CALL_123"]
	eng.mu.Unlock()

	if m.relay == nil {
		t.Fatal("relay was not populated from call ack")
	}
	if len(m.relay.endpoints) == 0 {
		t.Fatal("relay endpoints were not parsed")
	}
}

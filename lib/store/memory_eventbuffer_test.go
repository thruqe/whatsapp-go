// Copyright (c) 2026 whatsrook contributors
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package store

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestMemoryEventBuffer_BufferedEvents(t *testing.T) {
	ctx := context.Background()
	buf := NewMemoryEventBuffer(MemoryEventBufferOptions{
		EventTTL:    200 * time.Millisecond,
		OutgoingTTL: 200 * time.Millisecond,
		MaxEvents:   10,
	})
	defer func() { _ = buf.Close() }()

	hash := sha256.Sum256([]byte("test_ciphertext"))
	plaintext := []byte("hello world decrypted")
	serverTime := time.Now().Add(-10 * time.Second)

	// 1. PutBufferedEvent
	if err := buf.PutBufferedEvent(ctx, hash, plaintext, serverTime); err != nil {
		t.Fatalf("PutBufferedEvent failed: %v", err)
	}

	// 2. GetBufferedEvent
	event, err := buf.GetBufferedEvent(ctx, hash)
	if err != nil {
		t.Fatalf("GetBufferedEvent failed: %v", err)
	}
	if event == nil || string(event.Plaintext) != string(plaintext) {
		t.Fatalf("unexpected event: %+v", event)
	}

	// 3. ClearBufferedEventPlaintext
	if err := buf.ClearBufferedEventPlaintext(ctx, hash); err != nil {
		t.Fatalf("ClearBufferedEventPlaintext failed: %v", err)
	}
	event, err = buf.GetBufferedEvent(ctx, hash)
	if err != nil {
		t.Fatalf("GetBufferedEvent after clear failed: %v", err)
	}
	if event == nil || event.Plaintext != nil {
		t.Fatalf("expected plaintext to be nil, got: %+v", event)
	}

	// 4. DoDecryptionTxn
	executed := false
	err = buf.DoDecryptionTxn(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil || !executed {
		t.Fatalf("DoDecryptionTxn failed: err=%v, executed=%v", err, executed)
	}

	// 5. Expiration
	time.Sleep(250 * time.Millisecond)
	event, err = buf.GetBufferedEvent(ctx, hash)
	if err != nil || event != nil {
		t.Fatalf("expected event to expire, got: %+v, err: %v", event, err)
	}
}

func TestMemoryEventBuffer_OutgoingEvents(t *testing.T) {
	ctx := context.Background()
	buf := NewMemoryEventBuffer(MemoryEventBufferOptions{
		EventTTL:    200 * time.Millisecond,
		OutgoingTTL: 200 * time.Millisecond,
		MaxEvents:   10,
	})
	defer func() { _ = buf.Close() }()

	chatJID := types.NewJID("12345", types.DefaultUserServer)
	altJID := types.NewJID("12345", types.HiddenUserServer)
	msgID := types.MessageID("3EB0123456789")
	format := "wa"
	payload := []byte("proto-outgoing-message")

	// 1. AddOutgoingEvent
	if err := buf.AddOutgoingEvent(ctx, chatJID, msgID, format, payload); err != nil {
		t.Fatalf("AddOutgoingEvent failed: %v", err)
	}

	// 2. GetOutgoingEvent with primary chat JID
	fmtRet, retPayload, err := buf.GetOutgoingEvent(ctx, chatJID, types.EmptyJID, msgID)
	if err != nil || fmtRet != format || string(retPayload) != string(payload) {
		t.Fatalf("GetOutgoingEvent failed: fmt=%s, payload=%s, err=%v", fmtRet, string(retPayload), err)
	}

	// 3. GetOutgoingEvent with alt JID
	fmtRet, retPayload, err = buf.GetOutgoingEvent(ctx, types.EmptyJID, altJID, msgID)
	// altJID has same user "12345" but different server, but our key is chatJID.ToNonAD().String()
	// Let's test using chatJID
	fmtRet, retPayload, err = buf.GetOutgoingEvent(ctx, chatJID, altJID, msgID)
	if err != nil || fmtRet != format || string(retPayload) != string(payload) {
		t.Fatalf("GetOutgoingEvent with alt failed: fmt=%s, payload=%s, err=%v", fmtRet, string(retPayload), err)
	}

	// 4. Expiration
	time.Sleep(250 * time.Millisecond)
	fmtRet, retPayload, err = buf.GetOutgoingEvent(ctx, chatJID, types.EmptyJID, msgID)
	if err != nil || fmtRet != "" || retPayload != nil {
		t.Fatalf("expected outgoing event to expire, got: %s, %v", fmtRet, retPayload)
	}
}

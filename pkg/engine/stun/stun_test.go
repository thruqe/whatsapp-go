package stun

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type stunKat struct {
	Inputs struct {
		SSRC uint32 `json:"ssrc"`
	} `json:"inputs"`
	Stun struct {
		TX              string `json:"tx"`
		RelayToken      string `json:"relayToken"`
		MiKey           string `json:"miKey"`
		Crc32Abc        uint32 `json:"crc32_abc"`
		AttrToken       string `json:"attr_token"`
		XorEndpoint     string `json:"xorEndpoint"`
		NativeSenderSub string `json:"nativeSenderSub"`
		MinimalMi       string `json:"minimalMi"`
		WithFp          string `json:"withFp"`
		WasmAllocate    string `json:"wasmAllocate"`
		Ping            string `json:"ping"`
	} `json:"stun"`
	StunProto struct {
		VoipSenderSubscriptions   string `json:"voip_sender_subscriptions"`
		ApkSenderSubscriptionsNo  string `json:"apk_sender_subscriptions_nopid"`
		ApkSenderSubscriptionsPid string `json:"apk_sender_subscriptions_pid"`
		ApkStreamDescriptors      string `json:"apk_stream_descriptors"`
	} `json:"stun_proto"`
}

func loadStunKat(t *testing.T) stunKat {
	t.Helper()
	raw, err := os.ReadFile("testdata/kats.json")
	if err != nil {
		t.Fatalf("read kats.json: %v", err)
	}
	var k stunKat
	if err := json.Unmarshal(raw, &k); err != nil {
		t.Fatalf("parse kats.json: %v", err)
	}
	return k
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func tx12(t *testing.T, k stunKat) [12]byte {
	t.Helper()
	var tx [12]byte
	copy(tx[:], mustHex(t, k.Stun.TX))
	return tx
}

// TestCrc32IsIEEE checks the FINGERPRINT CRC-32 against the kat and the IEEE constant.
func TestCrc32IsIEEE(t *testing.T) {
	k := loadStunKat(t)
	if got := stunFingerprint([]byte("abc")); uint32(got) != k.Stun.Crc32Abc {
		t.Errorf("crc32(abc) = %#x, want %#x", got, k.Stun.Crc32Abc)
	}
	if got := stunFingerprint([]byte("abc")); got != 0x352441c2 {
		t.Errorf("crc32(abc) = %#x, want 0x352441c2", got)
	}
}

// TestAttrAndEndpointMatchKAT checks attribute encoding, the XOR endpoint, and the
// native sender subscription.
func TestAttrAndEndpointMatchKAT(t *testing.T) {
	k := loadStunKat(t)
	token := mustHex(t, k.Stun.RelayToken)
	if got := hex.EncodeToString(stunAttr(attrRelayToken, token)); got != k.Stun.AttrToken {
		t.Errorf("attr_token = %s, want %s", got, k.Stun.AttrToken)
	}
	ep, ok := EncodeXorRelayEndpoint("157.240.226.133", 3478)
	if !ok {
		t.Fatal("EncodeXorRelayEndpoint returned ok=false")
	}
	if got := hex.EncodeToString(ep[:]); got != k.Stun.XorEndpoint {
		t.Errorf("xorEndpoint = %s, want %s", got, k.Stun.XorEndpoint)
	}
	sub := CreateNativeSenderSubscription(k.Inputs.SSRC)
	if got := hex.EncodeToString(sub[:]); got != k.Stun.NativeSenderSub {
		t.Errorf("nativeSenderSub = %s, want %s", got, k.Stun.NativeSenderSub)
	}
}

// TestEncodeRequestMIAndFingerprint checks the MI-only and MI+FINGERPRINT encodings.
func TestEncodeRequestMIAndFingerprint(t *testing.T) {
	k := loadStunKat(t)
	tx := tx12(t, k)
	attrs := stunAttr(attrRelayToken, mustHex(t, k.Stun.RelayToken))
	miKey := mustHex(t, k.Stun.MiKey)

	minimal := EncodeStunRequest(MsgAllocateRequest, tx, attrs, miKey, false)
	if got := hex.EncodeToString(minimal); got != k.Stun.MinimalMi {
		t.Errorf("minimalMi = %s, want %s", got, k.Stun.MinimalMi)
	}
	withFp := EncodeStunRequest(MsgAllocateRequest, tx, attrs, miKey, true)
	if got := hex.EncodeToString(withFp); got != k.Stun.WithFp {
		t.Errorf("withFp = %s, want %s", got, k.Stun.WithFp)
	}
}

// TestWasmAllocateAndPing checks the WASM allocate request and the ping.
func TestWasmAllocateAndPing(t *testing.T) {
	k := loadStunKat(t)
	tx := tx12(t, k)
	token := mustHex(t, k.Stun.RelayToken)
	miKey := mustHex(t, k.Stun.MiKey)
	ep, ok := EncodeXorRelayEndpoint("157.240.226.133", 3478)
	if !ok {
		t.Fatal("endpoint ok=false")
	}
	alloc := BuildWasmStunAllocateRequest(tx, token, ep, miKey)
	if got := hex.EncodeToString(alloc); got != k.Stun.WasmAllocate {
		t.Errorf("wasmAllocate = %s, want %s", got, k.Stun.WasmAllocate)
	}
	ping := BuildWhatsappPing(tx)
	if got := hex.EncodeToString(ping[:]); got != k.Stun.Ping {
		t.Errorf("ping = %s, want %s", got, k.Stun.Ping)
	}
}

func TestWasmStreamDescriptorsMatchCapturedTemplate(t *testing.T) {
	ssrcs := [9]uint32{
		1170300490,
		2781599269,
		4281963094,
		2798104311,
		3731645995,
		1364979034,
		2983933125,
		4140589437,
		2522729392,
	}
	if got := hex.EncodeToString(CreateWasmStreamDescriptors(ssrcs)); got != hex.EncodeToString(wasmStreamDescriptorsTemplate) {
		t.Errorf("wasm stream descriptors = %s, want %s", got, hex.EncodeToString(wasmStreamDescriptorsTemplate))
	}
}

func TestWasmGroupAllocateCarriesCapturedVideoSubscriptions(t *testing.T) {
	ssrcs := [9]uint32{
		0x3ea26c0c,
		0x0bf99b28,
		0xf42e4556,
		0x14e8f126,
		0xbb16134f,
		0x98b14f00,
		0xe0e04163,
		0x74ed8516,
		0xdea8a613,
	}
	participantPIDs := []uint32{1, 2}
	transactionID := [12]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	endpointXOR := [6]byte{0x2c, 0x84, 0xbc, 0xe2, 0xb5, 0xc7}
	packet := BuildWasmStunAllocateRequestWithGroupSubscriptionsAndHBHFEC(
		transactionID,
		[]byte{0x01, 0x02, 0x03},
		endpointXOR,
		ssrcs,
		0xb31ded3e,
		[2]uint32{0xc1a17938, 0x1bb20c84},
		participantPIDs,
		[]byte("0123456789abcdef"),
	)
	attrs := ParseStunAttributes(packet)
	wantTypes := []uint16{0x4000, 0x4025, 0x4021, 0x4024, 0x805a, 0x0016, 0x0008}
	if len(attrs) != len(wantTypes) {
		t.Fatalf("group allocate attr count = %d, want %d", len(attrs), len(wantTypes))
	}
	for i, want := range wantTypes {
		if attrs[i].AttrType != want {
			t.Errorf("group allocate attr[%d].type = %#x, want %#x", i, attrs[i].AttrType, want)
		}
	}
	wantSenderSubscriptions := "0a1f0a1d0a0fa6e2a3a701cfa6d8d80b809ec5c5091204080110011204080210010a130a110a0fe38281870e968ab6a70793cca2f50d0a1a0a180a0e8cd889f503a8b6e65fd68ab9a10f12020801120208020a110a0f0a05bedaf7980b1202080112020802"
	if got := hex.EncodeToString(attrs[1].Value); got != wantSenderSubscriptions {
		t.Errorf("group sender subscriptions = %s, want %s", got, wantSenderSubscriptions)
	}
	wantReceiverSubscriptions := "1202080112020802"
	if got := hex.EncodeToString(attrs[2].Value); got != wantReceiverSubscriptions {
		t.Errorf("group receiver subscriptions = %s, want %s", got, wantReceiverSubscriptions)
	}
	wantStreamDescriptors := "0a06188cd889f5030a07100118a8b6e65f0a08100218d68ab9a10f0a08080118a6e2a3a7010a0a0801100118cfa6d8d80b0a0a0801100218809ec5c5090a08080218e38281870e0a0a0802100118968ab6a7070a0a080210021893cca2f50d0a0a0803100318b8f2858d0c0a0a08041003188499c8dd01"
	if got := hex.EncodeToString(attrs[3].Value); got != wantStreamDescriptors {
		t.Errorf("group stream descriptors = %s, want %s", got, wantStreamDescriptors)
	}
	if got := hex.EncodeToString(attrs[4].Value); got != "02" {
		t.Errorf("group participant count = %s, want 02", got)
	}

	oneParticipant := BuildWasmStunAllocateRequestWithGroupSubscriptionsAndHBHFEC(
		transactionID,
		[]byte{0x01, 0x02, 0x03},
		endpointXOR,
		ssrcs,
		0xb31ded3e,
		[2]uint32{0xc1a17938, 0x1bb20c84},
		[]uint32{1},
		[]byte("0123456789abcdef"),
	)
	oneParticipantAttrs := ParseStunAttributes(oneParticipant)
	if got, want := oneParticipantAttrs[3].Value, CreateWasmStreamDescriptors(ssrcs); !bytes.Equal(got, want) {
		t.Errorf("one-participant stream descriptors = %x, want no HBH-FEC suffix %x", got, want)
	}
}

// TestParseRoundTripsAttributes parses the minimal MI request back into attributes.
func TestParseRoundTripsAttributes(t *testing.T) {
	k := loadStunKat(t)
	minimal := mustHex(t, k.Stun.MinimalMi)
	if !IsStunPacket(minimal) {
		t.Fatal("minimal not classified as STUN packet")
	}
	if mt, ok := StunMessageType(minimal); !ok || mt != MsgAllocateRequest {
		t.Errorf("message type = (%#x, %v), want (%#x, true)", mt, ok, MsgAllocateRequest)
	}
	attrs := ParseStunAttributes(minimal)
	if len(attrs) != 2 {
		t.Fatalf("attr count = %d, want 2", len(attrs))
	}
	if attrs[0].AttrType != attrRelayToken {
		t.Errorf("attr[0].type = %#x, want %#x", attrs[0].AttrType, attrRelayToken)
	}
	if hex.EncodeToString(attrs[0].Value) != k.Stun.RelayToken {
		t.Errorf("attr[0].value = %x, want %s", attrs[0].Value, k.Stun.RelayToken)
	}
	if attrs[1].AttrType != attrMessageIntegrity || len(attrs[1].Value) != 20 {
		t.Errorf("attr[1] = (%#x, len %d), want (%#x, 20)", attrs[1].AttrType, len(attrs[1].Value), attrMessageIntegrity)
	}
}

// TestProtobufPayloadsMatchKAT checks the three protobuf subscription/descriptor blobs.
func TestProtobufPayloadsMatchKAT(t *testing.T) {
	k := loadStunKat(t)
	ssrc := k.Inputs.SSRC
	if got := hex.EncodeToString(CreateVoipSenderSubscriptions(ssrc)); got != k.StunProto.VoipSenderSubscriptions {
		t.Errorf("voip_sender_subscriptions = %s, want %s", got, k.StunProto.VoipSenderSubscriptions)
	}
	if got := hex.EncodeToString(CreateApkSenderSubscriptions(ssrc, nil)); got != k.StunProto.ApkSenderSubscriptionsNo {
		t.Errorf("apk_sender_subscriptions_nopid = %s, want %s", got, k.StunProto.ApkSenderSubscriptionsNo)
	}
	pid := uint32(7)
	if got := hex.EncodeToString(CreateApkSenderSubscriptions(ssrc, &pid)); got != k.StunProto.ApkSenderSubscriptionsPid {
		t.Errorf("apk_sender_subscriptions_pid = %s, want %s", got, k.StunProto.ApkSenderSubscriptionsPid)
	}
	if got := hex.EncodeToString(CreateApkStreamDescriptors(ssrc)); got != k.StunProto.ApkStreamDescriptors {
		t.Errorf("apk_stream_descriptors = %s, want %s", got, k.StunProto.ApkStreamDescriptors)
	}
}

// TestAndroidAllocateCarriesThreeAttrs checks the APK allocate carries the four attrs.
func TestAndroidAllocateCarriesThreeAttrs(t *testing.T) {
	k := loadStunKat(t)
	tx := tx12(t, k)
	token := mustHex(t, k.Stun.RelayToken)
	miKey := mustHex(t, k.Stun.MiKey)
	ssrc := k.Inputs.SSRC
	pkt := BuildAndroidStunAllocateRequest(tx, token, ssrc, nil, miKey, false)
	attrs := ParseStunAttributes(pkt)
	if len(attrs) != 4 {
		t.Fatalf("attr count = %d, want 4", len(attrs))
	}
	want := []uint16{attrRelayToken, attrSenderSubscriptionsV2, attrStreamDescriptors, attrMessageIntegrity}
	for i, w := range want {
		if attrs[i].AttrType != w {
			t.Errorf("attr[%d].type = %#x, want %#x", i, attrs[i].AttrType, w)
		}
	}
	if hex.EncodeToString(attrs[2].Value) != hex.EncodeToString(CreateApkStreamDescriptors(ssrc)) {
		t.Errorf("attr[2].value mismatch")
	}
}

// TestPongMatching checks pong classification with and without a transaction id.
func TestPongMatching(t *testing.T) {
	k := loadStunKat(t)
	tx := tx12(t, k)
	pong := BuildWhatsappPing(tx)
	binary.BigEndian.PutUint16(pong[0:2], MsgWhatsappPong)
	if !IsWhatsappPong(pong[:], tx[:]) {
		t.Error("pong with matching tx not recognized")
	}
	if !IsWhatsappPong(pong[:], nil) {
		t.Error("pong with nil tx not recognized")
	}
	var wrong [12]byte
	if IsWhatsappPong(pong[:], wrong[:]) {
		t.Error("pong with wrong tx falsely matched")
	}
}

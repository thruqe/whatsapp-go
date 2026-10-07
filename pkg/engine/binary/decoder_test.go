package binary_test

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	original := binary.Node{
		Tag: "iq",
		Attrs: binary.Attrs{
			"id":   "abc",
			"type": "result",
			"from": types.NewJID("12345", types.DefaultUserServer),
		},
		Content: []binary.Node{
			{Tag: "body", Content: []byte("hi")},
		},
	}
	marshaled, err := binary.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	unpacked, err := binary.Unpack(marshaled)
	if err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}
	decoded, err := binary.Unmarshal(unpacked)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(*decoded, original) {
		t.Errorf("round trip mismatch:\n original = %+v\n decoded  = %+v", original, *decoded)
	}
}

func TestUnmarshalMalformedInputDoesNotPanic(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"node tag is nil", []byte{248, 1, 0}},
		{"empty packed-string tag", []byte{248, 1, 255, 128}},
		{"jidpair user is a list", []byte{248, 2, 252, 1, 97, 250, 248, 0, 252, 1, 115}},
		{"jidpair server is a list", []byte{248, 2, 252, 1, 97, 250, 0, 248, 0}},
		{"adjid user is a list", []byte{248, 2, 252, 1, 97, 247, 1, 1, 248, 0}},
		{"fbjid user is a list", []byte{248, 2, 252, 1, 97, 246, 248, 0, 0, 0, 252, 4, 109, 115, 103, 114}},
		{"interopjid user is a list", []byte{248, 2, 252, 1, 97, 245, 248, 0, 0, 0, 0, 0, 252, 7, 105, 110, 116, 101, 114, 111, 112}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Unmarshal panicked on malformed input: %v", r)
				}
			}()
			if _, err := binary.Unmarshal(tc.input); err == nil {
				t.Error("expected an error for malformed input, got nil")
			}
		})
	}
}

func TestUnpackEmptyInputDoesNotPanic(t *testing.T) {
	for _, input := range [][]byte{nil, {}} {
		t.Run(string(input), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Unpack panicked on empty input: %v", r)
				}
			}()
			if _, err := binary.Unpack(input); err == nil {
				t.Error("expected an error for empty input, got nil")
			}
		})
	}
}

package msg

import (
	"reflect"
	"testing"
)

func TestMsgEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		msg  Msg
	}{
		{
			name: "basic message",
			msg: Msg{
				Magic:     MagicNumber,
				Version:   1,
				Type:      1,
				MsgID:     100,
				Timestamp: 1234567890,
			},
		},
		{
			name: "message with payload",
			msg: Msg{
				Magic:      MagicNumber,
				Version:    1,
				Flags:      0x01,
				Type:       2,
				MsgID:      200,
				PayloadLen: 5,
				Timestamp:  987654321,
				Payload:    []byte{1, 2, 3, 4, 5},
				Checksum:   12345,
			},
		},
		{
			name: "message with max values",
			msg: Msg{
				Magic:     MagicNumber,
				Version:   2,
				Flags:     0xFF,
				Type:      5,
				MsgID:     65535,
				Timestamp: 4294967295,
				Checksum:  65535,
			},
		},
		{
			name: "message with string payload",
			msg: Msg{
				Magic:      MagicNumber,
				Version:    1,
				Flags:      0x12,
				Type:       3,
				MsgID:      999,
				PayloadLen: 13,
				Timestamp:  1617181920,
				Payload:    []byte("Hello, World!"),
				Checksum:   54321,
			},
		},
		{
			name: "complex message",
			msg: Msg{
				Magic:      MagicNumber,
				Version:    1,
				Flags:      0xAA,
				Type:       10,
				MsgID:      256,
				PayloadLen: 10,
				Timestamp:  999999999,
				Payload:    []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
				Checksum:   0xABCD,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Encode
			encoded, err := tt.msg.Encode()
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			t.Logf("Encoded: %d bytes | hex: %x", len(encoded), encoded)

			// Verify minimum size
			expectedMinSize := HeaderSize + CrcSize
			if len(encoded) < expectedMinSize {
				t.Errorf("Encoded size = %d, want at least %d", len(encoded), expectedMinSize)
			}

			// Test Decode
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			// Verify round-trip using DeepEqual
			if !reflect.DeepEqual(decoded, tt.msg) {
				t.Errorf("RoundTrip failed\ngot:  %+v\nwant: %+v", decoded, tt.msg)
			}

			// Log payload details if present
			if len(decoded.Payload) > 0 {
				t.Logf("Payload: %d bytes | hex: %x | string: %q", len(decoded.Payload), decoded.Payload, string(decoded.Payload))
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty data", data: []byte{}},
		{name: "insufficient data - only magic number", data: []byte{0x4E, 0x59}},
		{name: "insufficient data - partial header", data: []byte{0x4E, 0x59, 0x01, 0x00, 0x01}},
		{
			name: "incomplete payload",
			data: func() []byte {
				msg := Msg{
					Magic:      MagicNumber,
					Version:    1,
					Type:       1,
					MsgID:      1,
					PayloadLen: 10,
					Payload:    []byte{1, 2, 3},
				}
				data, _ := msg.Encode()
				return data[:len(data)-2]
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.data)
			if err == nil {
				t.Errorf("Decode() expected error, got nil")
			}
		})
	}
}

func TestNewMsg(t *testing.T) {
	// Test auto-generated MsgID
	msg1 := New(1, []byte("test"))
	msg2 := New(2, []byte("hello"))

	if msg1.Magic != MagicNumber {
		t.Errorf("Expected MagicNumber, got %v", msg1.Magic)
	}
	if msg1.Version != 1 {
		t.Errorf("Expected Version 1, got %v", msg1.Version)
	}
	if msg1.MsgID == msg2.MsgID {
		t.Errorf("MsgIDs should be different: msg1=%v, msg2=%v", msg1.MsgID, msg2.MsgID)
	}
	if msg1.MsgID+1 != msg2.MsgID {
		t.Errorf("MsgIDs should be sequential: msg1=%v, msg2=%v", msg1.MsgID, msg2.MsgID)
	}

	t.Logf("Auto-generated MsgIDs: msg1=%d, msg2=%d", msg1.MsgID, msg2.MsgID)

	// Test manual MsgID
	customMsg := NewWithID(3, 999, []byte("custom"))
	if customMsg.MsgID != 999 {
		t.Errorf("Expected custom MsgID 999, got %v", customMsg.MsgID)
	}

	// Test round-trip with auto-generated ID
	encoded, err := msg1.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(decoded, msg1) {
		t.Errorf("Round-trip failed\ngot:  %+v\nwant: %+v", decoded, msg1)
	}
}

func TestHandshakePayloadRoundTrip(t *testing.T) {
	original := &HandshakePayload{
		Version: 1,
		Sn:      "CLIENT001",
		Token:   "token123456",
	}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded, err := DecodeHandshakePayload(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if decoded.Version != original.Version ||
		decoded.Sn != original.Sn ||
		decoded.Token != original.Token {
		t.Errorf("Round-trip mismatch\ngot:  %+v\nwant: %+v", decoded, original)
	}
}

func TestHandshakeResponseRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		code    uint8
		message string
	}{
		{"success", HandshakeOK, "OK"},
		{"bad sn", HandshakeBadSn, "unknown SN"},
		{"bad token", HandshakeBadToken, "token mismatch"},
		{"empty message", HandshakeFailed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &HandshakeResponsePayload{Code: tt.code, Message: tt.message}
			encoded, err := original.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			decoded, err := DecodeHandshakeResponsePayload(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			if decoded.Code != original.Code || decoded.Message != original.Message {
				t.Errorf("Mismatch\ngot:  %+v\nwant: %+v", decoded, original)
			}
		})
	}
}


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

package msg

import (
	"LEPG/internal/model"
	"reflect"
	"testing"
)

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
}

func TestHandshakePayloadRoundTrip(t *testing.T) {
	original := &HandshakePayload{
		FirmwareVersion: 1,
		Sn:              "CLIENT001",
		Token:           "token123456",
	}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded := &HandshakePayload{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("Round-trip mismatch\ngot:  %+v\nwant: %+v", decoded, original)
	}
}

func TestAckPayloadRoundTrip(t *testing.T) {
	original := &AckPayload{MsgID: 1234, Code: Ok}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded := &AckPayload{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("Round-trip mismatch\ngot:  %+v\nwant: %+v", decoded, original)
	}
}

func TestHeartbeatPayloadRoundTrip(t *testing.T) {
	original := &HeartbeatPayload{}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded := &HeartbeatPayload{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("Round-trip mismatch\ngot:  %+v\nwant: %+v", decoded, original)
	}
}

func TestNotifyPayloadRoundTrip(t *testing.T) {
	// RawData > 255B 以覆盖 2 字节长度前缀路径
	raw := make([]byte, 300)
	for i := range raw {
		raw[i] = byte(i)
	}
	original := &NotifyPayload{
		EventCode:  0x02,
		DeviceHash: "abcdef0123456789",
		Timestamp:  1700000000,
		Severity:   1,
		Message:    "device offline",
		RawData:    raw,
	}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded := &NotifyPayload{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("Round-trip mismatch\ngot:  %+v\nwant: %+v", decoded, original)
	}
}

func TestUploadPayloadRoundTrip(t *testing.T) {
	original := &UploadPayload{
		Readings: []model.Reading{
			{
				ID:         1,
				Device:     "aaaa000000000001",
				DeviceName: "sensor-1",
				Point:      "bbbb000000000001",
				PointName:  "temperature",
				DataType:   model.DataTypeFloat32,
				Value:      "23.5",
				Quality:    model.QualityGood,
				Unit:       "C",
				Timestamp:  1700000000000,
			},
			{
				ID:        2,
				Device:    "aaaa000000000001",
				Point:     "bbbb000000000002",
				DataType:  model.DataTypeBool,
				Value:     "true",
				Quality:   model.QualityBad,
				Timestamp: 1700000001000,
			},
		},
	}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded := &UploadPayload{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("Round-trip mismatch\ngot:  %+v\nwant: %+v", decoded, original)
	}
}

func TestHandshakePayloadTruncation(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"only version", []byte{1}},
		{"missing token length", []byte{1, 0}},
		{"sn length exceeds buffer", []byte{1, 5, 0}},
		{"token length exceeds buffer", []byte{1, 0, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &HandshakePayload{}
			if err := p.Decode(tt.data); err == nil {
				t.Errorf("expected error, decoded: %+v", p)
			}
		})
	}
}

func TestNotifyPayloadTruncation(t *testing.T) {
	full := &NotifyPayload{
		EventCode:  0x01,
		DeviceHash: "hash1234",
		Timestamp:  1,
		Severity:   0,
		Message:    "ok",
		RawData:    []byte{0xDE, 0xAD},
	}
	encoded, err := full.Encode()
	if err != nil {
		t.Fatalf("setup encode failed: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", encoded[:3]},
		{"truncated before timestamp", encoded[:1+1+len(full.DeviceHash)+2]},
		{"truncated before raw data length", encoded[:len(encoded)-4]},
		{"truncated raw data", encoded[:len(encoded)-1]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &NotifyPayload{}
			if err := p.Decode(tt.data); err == nil {
				t.Errorf("expected error, decoded: %+v", p)
			}
		})
	}
}

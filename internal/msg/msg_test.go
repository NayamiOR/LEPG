package msg

import (
	"bytes"
	stderrors "errors"
	"io"
	"net"
	"reflect"
	"testing"

	"LEPG/internal/errors"
	"LEPG/internal/model"
	"LEPG/internal/utils"
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

func TestParseMsgRoundTrip(t *testing.T) {
	factory := NewMsgFactory()

	tests := []struct {
		name    string
		msgType uint8
		packet  Packable
	}{
		{
			name:    "handshake",
			msgType: MsgTypeHandshake,
			packet:  &HandshakePayload{FirmwareVersion: 1, Sn: "CLIENT001", Token: "token123456"},
		},
		{
			name:    "upload",
			msgType: MsgTypeUpload,
			packet: &UploadPayload{Readings: []model.Reading{
				{ID: 1, Device: "aaaa000000000001", Value: "23.5", DataType: model.DataTypeFloat32, Timestamp: 1700000000000},
			}},
		},
		{
			name:    "notify",
			msgType: MsgTypeNotify,
			packet:  &NotifyPayload{EventCode: 0x02, DeviceHash: "hash", Timestamp: 1, Severity: 1, Message: "offline", RawData: []byte{1, 2, 3}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := factory.NewMsg(tt.msgType, tt.packet)
			if err != nil {
				t.Fatalf("NewMsg failed: %v", err)
			}
			parsed, err := ParseMsg(m)
			if err != nil {
				t.Fatalf("ParseMsg failed: %v", err)
			}
			if !reflect.DeepEqual(tt.packet, parsed) {
				t.Errorf("round-trip mismatch\ngot:  %+v\nwant: %+v", parsed, tt.packet)
			}
		})
	}
}

func TestParseMsgAckTypes(t *testing.T) {
	factory := NewMsgFactory()
	ackTypes := []struct {
		name string
		t    uint8
	}{
		{"handshakeAck", MsgTypeHandshakeAck},
		{"uploadAck", MsgTypeUploadAck},
		{"heartbeatAck", MsgTypeHeartbeatAck},
	}
	for _, at := range ackTypes {
		t.Run(at.name, func(t *testing.T) {
			original := &AckPayload{MsgID: 42, Code: Ok}
			m, err := factory.NewMsg(at.t, original)
			if err != nil {
				t.Fatalf("NewMsg failed: %v", err)
			}
			parsed, err := ParseMsg(m)
			if err != nil {
				t.Fatalf("ParseMsg failed: %v", err)
			}
			got, ok := parsed.(*AckPayload)
			if !ok {
				t.Fatalf("expected *AckPayload, got %T", parsed)
			}
			if !reflect.DeepEqual(original, got) {
				t.Errorf("mismatch\ngot:  %+v\nwant: %+v", got, original)
			}
		})
	}
}

func TestParseMsgUnknownType(t *testing.T) {
	m := &Msg{Type: 99, Payload: []byte{}}
	if _, err := ParseMsg(m); err == nil {
		t.Fatal("expected error for unknown packet type, got nil")
	}
}

// decodeFrameFromBytes 把字节经 net.Pipe 喂给 DecodeFrame。
// 所有调用方都会恰好消费完写入字节，writer 的 Write/Close 都能完成，不会泄漏 goroutine。
func decodeFrameFromBytes(t *testing.T, data []byte) (Msg, error) {
	t.Helper()
	a, b := net.Pipe()
	go func() {
		a.Write(data)
		a.Close()
	}()
	got, err := DecodeFrame(b)
	b.Close()
	return got, err
}

func TestMsgFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Msg
	}{
		{
			name: "with payload",
			msg: Msg{
				Magic: MagicNumber, Version: 2, Flags: 0x0F, Type: MsgTypeUpload,
				MsgID: 4660, PayloadLen: 5, Timestamp: 123456,
				Payload: []byte{1, 2, 3, 4, 5},
			},
		},
		{
			name: "nil payload",
			msg: Msg{
				Magic: MagicNumber, Version: 1, Flags: 0, Type: MsgTypeHeartbeat,
				MsgID: 1, PayloadLen: 0, Timestamp: 99,
				Payload: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.msg
			original.Checksum = utils.CalChecksum(original.headerAndPayload())

			encoded, err := original.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			got, err := decodeFrameFromBytes(t, encoded)
			if err != nil {
				t.Fatalf("DecodeFrame failed: %v", err)
			}
			if got.Magic != original.Magic || got.Version != original.Version ||
				got.Flags != original.Flags || got.Type != original.Type ||
				got.MsgID != original.MsgID || got.PayloadLen != original.PayloadLen ||
				got.Timestamp != original.Timestamp || got.Checksum != original.Checksum {
				t.Errorf("header round-trip mismatch\ngot:  %+v\nwant: %+v", got, original)
			}
			if !bytes.Equal(got.Payload, original.Payload) {
				t.Errorf("payload mismatch\ngot:  %v\nwant: %v", got.Payload, original.Payload)
			}
		})
	}
}

func TestDecodeFrameChecksumMismatch(t *testing.T) {
	msg := Msg{
		Magic: MagicNumber, Version: 1, Type: MsgTypeUpload, MsgID: 1,
		Payload: []byte{1, 2, 3, 4, 5},
	}
	msg.Checksum = utils.CalChecksum(msg.headerAndPayload())

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	encoded[HeaderSize] ^= 0xFF // 篡改 payload 首字节，checksum 字段不变

	_, err = decodeFrameFromBytes(t, encoded)
	if !stderrors.Is(err, errors.ErrChecksumMismatch) {
		t.Errorf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestDecodeFrameTruncated(t *testing.T) {
	// 合法 magic 但缺后续字段 → magic 通过、version 读触发 io.EOF
	_, err := decodeFrameFromBytes(t, []byte{0x4E, 0x59})
	if err == nil {
		t.Fatal("expected error on truncated frame, got nil")
	}
	if !stderrors.Is(err, io.EOF) && !stderrors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected io.EOF or io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestDecodeFrameInvalidMagic(t *testing.T) {
	msg := Msg{
		Magic: MagicNumber, Version: 1, Type: MsgTypeUpload, MsgID: 1,
		Payload: []byte{1, 2, 3},
	}
	msg.Checksum = utils.CalChecksum(msg.headerAndPayload())

	encoded, err := msg.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	encoded[0] = 0x00 // 破坏 magic

	_, err = decodeFrameFromBytes(t, encoded)
	if !stderrors.Is(err, errors.ErrInvalidMagic) {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestChecksumCoversHeader(t *testing.T) {
	// 两个 Msg 仅在头部字段 Type 上不同；若 checksum 覆盖头部，二者 checksum 必须不同。
	m1 := &Msg{Magic: MagicNumber, Version: 1, Type: MsgTypeUpload, Payload: []byte("x")}
	m2 := &Msg{Magic: MagicNumber, Version: 1, Type: MsgTypeHeartbeat, Payload: []byte("x")}
	m1.Checksum = utils.CalChecksum(m1.headerAndPayload())
	m2.Checksum = utils.CalChecksum(m2.headerAndPayload())
	if m1.Checksum == m2.Checksum {
		t.Fatal("checksum must differ when a header field differs")
	}
}

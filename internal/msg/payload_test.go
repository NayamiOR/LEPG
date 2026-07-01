package msg

import (
	"LEPG/internal/model"
	"testing"
)

// ── HandshakePayload ─────────────────────────────────────────

func TestHandshakePayload_RoundTrip(t *testing.T) {
	original := &HandshakePayload{
		FirmwareVersion: 1,
		Sn:              "CLIENT001",
		Token:           "secret-token-123",
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded HandshakePayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.FirmwareVersion != original.FirmwareVersion {
		t.Errorf("FirmwareVersion: got %d, want %d", decoded.FirmwareVersion, original.FirmwareVersion)
	}
	if decoded.Sn != original.Sn {
		t.Errorf("Sn: got %q, want %q", decoded.Sn, original.Sn)
	}
	if decoded.Token != original.Token {
		t.Errorf("Token: got %q, want %q", decoded.Token, original.Token)
	}
}

func TestHandshakePayload_SnTooLong(t *testing.T) {
	p := &HandshakePayload{Sn: string(make([]byte, 256))}
	_, err := p.Encode()
	if err == nil {
		t.Error("expected error for Sn > 255 bytes")
	}
}

func TestHandshakePayload_TokenTooLong(t *testing.T) {
	p := &HandshakePayload{Sn: "ok", Token: string(make([]byte, 256))}
	_, err := p.Encode()
	if err == nil {
		t.Error("expected error for Token > 255 bytes")
	}
}

func TestHandshakePayload_Decode_TooShort(t *testing.T) {
	var p HandshakePayload
	err := p.Decode([]byte{1}) // 至少需要 3 bytes (version + snlen + tokenlen)
	if err == nil {
		t.Error("expected error for too-short payload")
	}
}

func TestHandshakePayload_Decode_InvalidSnLen(t *testing.T) {
	// snLen=10, 但实际数据不够
	var p HandshakePayload
	err := p.Decode([]byte{1, 10, 0})
	if err == nil {
		t.Error("expected error for invalid sn length")
	}
}

// ── AckPayload ───────────────────────────────────────────────

func TestAckPayload_RoundTrip(t *testing.T) {
	original := &AckPayload{
		MsgID: 42,
		Code:  Ok,
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(encoded) != 3 {
		t.Errorf("expected 3 bytes, got %d", len(encoded))
	}

	var decoded AckPayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.MsgID != 42 {
		t.Errorf("MsgID: got %d, want 42", decoded.MsgID)
	}
	if decoded.Code != Ok {
		t.Errorf("Code: got %d, want %d", decoded.Code, Ok)
	}
}

func TestAckPayload_Decode_TooShort(t *testing.T) {
	var p AckPayload
	err := p.Decode([]byte{1, 2})
	if err == nil {
		t.Error("expected error for <3 bytes")
	}
}

func TestAckPayload_ReasonCodes(t *testing.T) {
	codes := []struct {
		name string
		code uint8
	}{
		{"Ok", Ok},
		{"Failed", Failed},
		{"BadSn", BadSn},
		{"BadToken", BadToken},
	}
	for _, tc := range codes {
		p := &AckPayload{MsgID: 1, Code: tc.code}
		enc, _ := p.Encode()
		var dec AckPayload
		dec.Decode(enc)
		if dec.Code != tc.code {
			t.Errorf("%s: expected code %d, got %d", tc.name, tc.code, dec.Code)
		}
	}
}

// ── UploadPayload ────────────────────────────────────────────

func TestUploadPayload_RoundTrip(t *testing.T) {
	original := &UploadPayload{
		Readings: []model.Reading{
			{
				Device:     "aaaaaaaaaaaaaaaa",
				DeviceName: "温湿度传感器-01",
				Point:      "bbbbbbbbbbbbbbbb",
				PointName:  "temperature",
				DataType:   model.DataTypeFloat32,
				Value:      "25.3",
				Quality:    model.QualityGood,
				Timestamp:  1719302400000,
			},
			{
				Device:     "cccccccccccccccc",
				DeviceName: "电表-02",
				Point:      "dddddddddddddddd",
				PointName:  "power",
				DataType:   model.DataTypeFloat64,
				Value:      "1500.5",
				Quality:    model.QualityGood,
				Timestamp:  1719302400000,
			},
		},
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded UploadPayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(decoded.Readings) != 2 {
		t.Fatalf("expected 2 readings, got %d", len(decoded.Readings))
	}
	if decoded.Readings[0].DeviceName != "温湿度传感器-01" {
		t.Errorf("DeviceName[0]: got %q", decoded.Readings[0].DeviceName)
	}
	if decoded.Readings[0].Value != "25.3" {
		t.Errorf("Value[0]: got %q", decoded.Readings[0].Value)
	}
	if decoded.Readings[1].DeviceName != "电表-02" {
		t.Errorf("DeviceName[1]: got %q", decoded.Readings[1].DeviceName)
	}
}

func TestUploadPayload_EmptyReadings(t *testing.T) {
	original := &UploadPayload{Readings: []model.Reading{}}
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode empty readings: %v", err)
	}
	var decoded UploadPayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode empty readings: %v", err)
	}
	if len(decoded.Readings) != 0 {
		t.Errorf("expected 0 readings, got %d", len(decoded.Readings))
	}
}

func TestUploadPayload_Decode_GarbageData(t *testing.T) {
	var p UploadPayload
	err := p.Decode([]byte{0xFF, 0xFF, 0xFF})
	if err == nil {
		t.Error("expected error for garbage data")
	}
}

// ── HeartbeatPayload ─────────────────────────────────────────

func TestHeartbeatPayload_RoundTrip(t *testing.T) {
	p := &HeartbeatPayload{}
	encoded, err := p.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(encoded) != 0 {
		t.Errorf("heartbeat payload should be empty, got %d bytes", len(encoded))
	}

	var decoded HeartbeatPayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
}

func TestHeartbeatPayload_Decode_NonEmpty(t *testing.T) {
	var p HeartbeatPayload
	err := p.Decode([]byte{0x00})
	if err == nil {
		t.Error("expected error for non-empty heartbeat payload")
	}
}

// ── NotifyPayload ────────────────────────────────────────────

func TestNotifyPayload_RoundTrip(t *testing.T) {
	original := &NotifyPayload{
		EventCode:  0x01, // 上线
		DeviceHash: "abcd1234abcd1234",
		Timestamp:  1719302400,
		Severity:   1, // 警告
		Message:    "设备离线超过 5 分钟",
		RawData:    []byte{0xAA, 0xBB, 0xCC},
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded NotifyPayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.EventCode != 0x01 {
		t.Errorf("EventCode: got %d, want 1", decoded.EventCode)
	}
	if decoded.DeviceHash != "abcd1234abcd1234" {
		t.Errorf("DeviceHash: got %q", decoded.DeviceHash)
	}
	if decoded.Timestamp != 1719302400 {
		t.Errorf("Timestamp: got %d", decoded.Timestamp)
	}
	if decoded.Severity != 1 {
		t.Errorf("Severity: got %d", decoded.Severity)
	}
	if decoded.Message != "设备离线超过 5 分钟" {
		t.Errorf("Message: got %q", decoded.Message)
	}
	if len(decoded.RawData) != 3 {
		t.Fatalf("RawData len: got %d, want 3", len(decoded.RawData))
	}
}

func TestNotifyPayload_EmptyFields(t *testing.T) {
	// 所有变长字段为空的 Notify
	original := &NotifyPayload{
		EventCode:  0x02,
		DeviceHash: "",
		Timestamp:  100,
		Severity:   0,
		Message:    "",
		RawData:    nil,
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded NotifyPayload
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.DeviceHash != "" {
		t.Errorf("expected empty DeviceHash, got %q", decoded.DeviceHash)
	}
	if len(decoded.RawData) != 0 {
		t.Errorf("expected empty RawData, got %d bytes", len(decoded.RawData))
	}
}

func TestNotifyPayload_Decode_TooShort(t *testing.T) {
	var p NotifyPayload
	err := p.Decode([]byte{0x01, 0x00}) // 只有 2 bytes，需要至少 10
	if err == nil {
		t.Error("expected error for too-short notify payload")
	}
}

func TestNotifyPayload_Decode_TruncatedBeforeTimestamp(t *testing.T) {
	var p NotifyPayload
	// EventCode(1) + DeviceHashLen(1) + DeviceHash(0) + 不够 timestamp 的 4 bytes
	err := p.Decode([]byte{0x01, 0x00, 0x00, 0x00})
	if err == nil {
		t.Error("expected error for truncated before timestamp")
	}
}

func TestNotifyPayload_DeviceHashTooLong(t *testing.T) {
	p := &NotifyPayload{DeviceHash: string(make([]byte, 256))}
	_, err := p.Encode()
	if err == nil {
		t.Error("expected error for DeviceHash > 255 bytes")
	}
}

func TestNotifyPayload_MessageTooLong(t *testing.T) {
	p := &NotifyPayload{Message: string(make([]byte, 256))}
	_, err := p.Encode()
	if err == nil {
		t.Error("expected error for Message > 255 bytes")
	}
}

// ── Factory + Registry Integration ───────────────────────────

func TestFactory_NewMsg_Handshake(t *testing.T) {
	f := NewMsgFactory()
	payload := &HandshakePayload{FirmwareVersion: 1, Sn: "CLIENT001", Token: "abc"}
	m, err := f.NewMsg(MsgTypeHandshake, payload)
	if err != nil {
		t.Fatalf("NewMsg: %v", err)
	}
	if m.Type != MsgTypeHandshake {
		t.Errorf("expected type Handshake, got %d", m.Type)
	}
	if m.Magic != MagicNumber {
		t.Errorf("expected magic 0x4E59, got 0x%04X", m.Magic)
	}

	// Parse back via registry
	parsed, err := ParseMsg(m)
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	hs, ok := parsed.(*HandshakePayload)
	if !ok {
		t.Fatalf("expected HandshakePayload, got %T", parsed)
	}
	if hs.Sn != "CLIENT001" {
		t.Errorf("SN: got %q", hs.Sn)
	}
}

func TestFactory_NewMsg_Upload(t *testing.T) {
	f := NewMsgFactory()
	payload := &UploadPayload{
		Readings: []model.Reading{
			{DeviceName: "sensor", PointName: "temp", Value: "25", Timestamp: 1000},
		},
	}
	m, err := f.NewMsg(MsgTypeUpload, payload)
	if err != nil {
		t.Fatalf("NewMsg: %v", err)
	}

	parsed, err := ParseMsg(m)
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	up, ok := parsed.(*UploadPayload)
	if !ok {
		t.Fatalf("expected UploadPayload, got %T", parsed)
	}
	if len(up.Readings) != 1 {
		t.Errorf("expected 1 reading, got %d", len(up.Readings))
	}
}

func TestFactory_NewMsg_Ack(t *testing.T) {
	f := NewMsgFactory()
	for _, ackType := range []uint8{MsgTypeHandshakeAck, MsgTypeUploadAck, MsgTypeHeartbeatAck} {
		payload := &AckPayload{MsgID: 7, Code: Ok}
		m, err := f.NewMsg(ackType, payload)
		if err != nil {
			t.Fatalf("NewMsg type=%d: %v", ackType, err)
		}

		parsed, err := ParseMsg(m)
		if err != nil {
			t.Fatalf("ParseMsg type=%d: %v", ackType, err)
		}
		ack, ok := parsed.(*AckPayload)
		if !ok {
			t.Fatalf("type=%d: expected AckPayload, got %T", ackType, parsed)
		}
		if ack.MsgID != 7 {
			t.Errorf("type=%d: MsgID got %d", ackType, ack.MsgID)
		}
	}
}

func TestFactory_NewMsg_Heartbeat(t *testing.T) {
	f := NewMsgFactory()
	m, err := f.NewMsg(MsgTypeHeartbeat, &HeartbeatPayload{})
	if err != nil {
		t.Fatalf("NewMsg: %v", err)
	}

	parsed, err := ParseMsg(m)
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	if _, ok := parsed.(*HeartbeatPayload); !ok {
		t.Fatalf("expected HeartbeatPayload, got %T", parsed)
	}
}

func TestFactory_NewMsg_Notify(t *testing.T) {
	f := NewMsgFactory()
	payload := &NotifyPayload{
		EventCode:  0x01,
		DeviceHash: "aaaa",
		Timestamp:  1234567890,
		Severity:   1,
		Message:    "test",
	}
	m, err := f.NewMsg(MsgTypeNotify, payload)
	if err != nil {
		t.Fatalf("NewMsg: %v", err)
	}

	parsed, err := ParseMsg(m)
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	np, ok := parsed.(*NotifyPayload)
	if !ok {
		t.Fatalf("expected NotifyPayload, got %T", parsed)
	}
	if np.Message != "test" {
		t.Errorf("Message: got %q", np.Message)
	}
}

func TestParseMsg_UnknownType(t *testing.T) {
	m := &Msg{Type: 255, Payload: []byte{}}
	_, err := ParseMsg(m)
	if err == nil {
		t.Error("expected error for unknown message type")
	}
}

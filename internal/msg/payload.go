package msg

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
)

// --- HandshakePayload ---
// Wire: [FirmwareVersion:1B][SnLen:1B][Sn:nB][TokenLen:1B][Token:nB]

func (p *HandshakePayload) Encode() ([]byte, error) {
	if len(p.Sn) > 255 {
		return nil, fmt.Errorf("handshake sn too long: %d bytes", len(p.Sn))
	}
	if len(p.Token) > 255 {
		return nil, fmt.Errorf("handshake token too long: %d bytes", len(p.Token))
	}
	buf := make([]byte, 0, 1+1+len(p.Sn)+1+len(p.Token))
	buf = append(buf, p.FirmwareVersion)
	buf = append(buf, byte(len(p.Sn)))
	buf = append(buf, []byte(p.Sn)...)
	buf = append(buf, byte(len(p.Token)))
	buf = append(buf, []byte(p.Token)...)
	return buf, nil
}

func (p *HandshakePayload) Decode(data []byte) error {
	if len(data) < 3 {
		return fmt.Errorf("handshake payload too short: %d bytes", len(data))
	}
	offset := 0
	p.FirmwareVersion = data[offset]
	offset++

	snLen := int(data[offset])
	offset++
	if offset+snLen > len(data) {
		return fmt.Errorf("invalid sn length: %d", snLen)
	}
	p.Sn = string(data[offset : offset+snLen])
	offset += snLen

	if offset >= len(data) {
		return fmt.Errorf("missing token length")
	}
	tokenLen := int(data[offset])
	offset++
	if offset+tokenLen > len(data) {
		return fmt.Errorf("invalid token length: %d", tokenLen)
	}
	p.Token = string(data[offset : offset+tokenLen])
	return nil
}

// --- AckPayload ---
// Wire: [MsgID:2B BE][Code:1B]

func (p *AckPayload) Encode() ([]byte, error) {
	buf := make([]byte, 3)
	binary.BigEndian.PutUint16(buf[0:2], p.MsgID)
	buf[2] = p.Code
	return buf, nil
}

func (p *AckPayload) Decode(data []byte) error {
	if len(data) < 3 {
		return fmt.Errorf("ack payload too short: %d bytes", len(data))
	}
	p.MsgID = binary.BigEndian.Uint16(data[0:2])
	p.Code = data[2]
	return nil
}

// --- UploadPayload ---
// 序列化沿用 client/server 现有的 gob 格式，仅做接口包装。

func (p *UploadPayload) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p.Readings); err != nil {
		return nil, fmt.Errorf("upload payload encode: %w", err)
	}
	return buf.Bytes(), nil
}

func (p *UploadPayload) Decode(data []byte) error {
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p.Readings); err != nil {
		return fmt.Errorf("upload payload decode: %w", err)
	}
	return nil
}

// --- HeartbeatPayload ---

func (p *HeartbeatPayload) Encode() ([]byte, error) {
	return nil, nil
}

func (p *HeartbeatPayload) Decode(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("heartbeat payload must be empty: %d bytes", len(data))
	}
	return nil
}

// --- NotifyPayload ---
// Wire:
//
//	[EventCode:1B]
//	[DeviceHashLen:1B][DeviceHash:nB]
//	[Timestamp:4B BE]
//	[Severity:1B]
//	[MessageLen:1B][Message:nB]
//	[RawDataLen:2B BE][RawData:nB]

func (p *NotifyPayload) Encode() ([]byte, error) {
	if len(p.DeviceHash) > 255 {
		return nil, fmt.Errorf("notify device hash too long: %d bytes", len(p.DeviceHash))
	}
	if len(p.Message) > 255 {
		return nil, fmt.Errorf("notify message too long: %d bytes", len(p.Message))
	}
	buf := make([]byte, 0, 1+1+len(p.DeviceHash)+4+1+1+len(p.Message)+2+len(p.RawData))
	buf = append(buf, p.EventCode)
	buf = append(buf, byte(len(p.DeviceHash)))
	buf = append(buf, []byte(p.DeviceHash)...)

	var ts [4]byte
	binary.BigEndian.PutUint32(ts[:], p.Timestamp)
	buf = append(buf, ts[:]...)

	buf = append(buf, p.Severity)
	buf = append(buf, byte(len(p.Message)))
	buf = append(buf, []byte(p.Message)...)

	var rdLen [2]byte
	binary.BigEndian.PutUint16(rdLen[:], uint16(len(p.RawData)))
	buf = append(buf, rdLen[:]...)
	buf = append(buf, p.RawData...)
	return buf, nil
}

func (p *NotifyPayload) Decode(data []byte) error {
	// 所有变长字段为空时的最小长度：1+1+4+1+1+2 = 10
	if len(data) < 10 {
		return fmt.Errorf("notify payload too short: %d bytes", len(data))
	}
	off := 0

	p.EventCode = data[off]
	off++

	dhLen := int(data[off])
	off++
	if off+dhLen > len(data) {
		return fmt.Errorf("invalid device hash length: %d", dhLen)
	}
	p.DeviceHash = string(data[off : off+dhLen])
	off += dhLen

	// 定长块：Timestamp(4) + Severity(1) + MessageLen(1) = 6
	if off+6 > len(data) {
		return fmt.Errorf("notify payload truncated before timestamp")
	}
	p.Timestamp = binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	p.Severity = data[off]
	off++
	msgLen := int(data[off])
	off++

	if off+msgLen > len(data) {
		return fmt.Errorf("invalid message length: %d", msgLen)
	}
	p.Message = string(data[off : off+msgLen])
	off += msgLen

	if off+2 > len(data) {
		return fmt.Errorf("notify payload truncated before raw data length")
	}
	rdLen := int(binary.BigEndian.Uint16(data[off : off+2]))
	off += 2
	if off+rdLen > len(data) {
		return fmt.Errorf("invalid raw data length: %d", rdLen)
	}
	p.RawData = append([]byte(nil), data[off:off+rdLen]...)
	return nil
}

package msg

import (
	"encoding/binary"
	"fmt"

	"LEPG/internal/model"
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
// Wire（大端序）: [Count:4B] + Count × Reading
// Reading: [ID:8B][DevLen:2B][Device][NameLen:2B][DeviceName][PointLen:2B][Point]
//          [PointNameLen:2B][PointName][DataTypeLen:2B][DataType][ValLen:2B][Value]
//          [Quality:1B][UnitLen:2B][Unit][Timestamp:8B]
// 2026-08-19 起由 gob 改为手写 TLV：gob 在独立流协议下每次都要重传类型定义并重编译
// （compileDec ~13µs/次协议税），手写格式与项目其他 payload（Handshake/Notify）风格统一。

func appendTLVString(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(s)))
	return append(b, s...)
}

func readTLVString(data []byte, off *int) (string, error) {
	if *off+2 > len(data) {
		return "", fmt.Errorf("truncated string length")
	}
	n := int(binary.BigEndian.Uint16(data[*off : *off+2]))
	*off += 2
	if *off+n > len(data) {
		return "", fmt.Errorf("truncated string")
	}
	s := string(data[*off : *off+n])
	*off += n
	return s, nil
}

// minReadingLen 是最小单条 Reading 的 wire 长度（所有字符串为空时）：8+2+2+2+2+2+2+1+2+8 = 31
const minReadingLen = 31

func (p *UploadPayload) Encode() ([]byte, error) {
	total := 4
	for i := range p.Readings {
		r := &p.Readings[i]
		for _, s := range []string{r.Device, r.DeviceName, r.Point, r.PointName, string(r.DataType), r.Value, r.Unit} {
			if len(s) > 65535 {
				return nil, fmt.Errorf("upload payload encode: string field too long: %d bytes", len(s))
			}
			total += 2 + len(s)
		}
		total += 8 + 1 + 8 // ID + Quality + Timestamp
	}
	buf := make([]byte, 0, total)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(p.Readings)))
	for i := range p.Readings {
		r := &p.Readings[i]
		buf = binary.BigEndian.AppendUint64(buf, uint64(r.ID))
		buf = appendTLVString(buf, r.Device)
		buf = appendTLVString(buf, r.DeviceName)
		buf = appendTLVString(buf, r.Point)
		buf = appendTLVString(buf, r.PointName)
		buf = appendTLVString(buf, string(r.DataType))
		buf = appendTLVString(buf, r.Value)
		buf = append(buf, byte(r.Quality))
		buf = appendTLVString(buf, r.Unit)
		buf = binary.BigEndian.AppendUint64(buf, uint64(r.Timestamp))
	}
	return buf, nil
}

func (p *UploadPayload) Decode(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("upload payload decode: payload too short: %d bytes", len(data))
	}
	count := int(binary.BigEndian.Uint32(data[:4]))
	// 防护：count 不可能超过 len(data)/最小单条长度，防恶意大 count 触发大分配
	if count > len(data)/minReadingLen+1 {
		return fmt.Errorf("upload payload decode: invalid reading count: %d", count)
	}
	off := 4
	p.Readings = make([]model.Reading, 0, count)
	for i := 0; i < count; i++ {
		var r model.Reading
		if off+8 > len(data) {
			return fmt.Errorf("upload payload decode: truncated id")
		}
		r.ID = int64(binary.BigEndian.Uint64(data[off : off+8]))
		off += 8
		var err error
		if r.Device, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: device: %w", err)
		}
		if r.DeviceName, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: device name: %w", err)
		}
		if r.Point, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: point: %w", err)
		}
		if r.PointName, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: point name: %w", err)
		}
		var dt string
		if dt, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: data type: %w", err)
		}
		r.DataType = model.DataType(dt)
		if r.Value, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: value: %w", err)
		}
		if off+1 > len(data) {
			return fmt.Errorf("upload payload decode: truncated quality")
		}
		r.Quality = model.Quality(data[off])
		off++
		if r.Unit, err = readTLVString(data, &off); err != nil {
			return fmt.Errorf("upload payload decode: unit: %w", err)
		}
		if off+8 > len(data) {
			return fmt.Errorf("upload payload decode: truncated timestamp")
		}
		r.Timestamp = int64(binary.BigEndian.Uint64(data[off : off+8]))
		off += 8
		p.Readings = append(p.Readings, r)
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

// --- 构造器：供 registry 使用，从 Msg 解出具体 Packable ---

func decodeHandshake(m *Msg) (Packable, error) {
	p := &HandshakePayload{}
	if err := p.Decode(m.Payload); err != nil {
		return nil, err
	}
	return p, nil
}

func decodeUpload(m *Msg) (Packable, error) {
	p := &UploadPayload{}
	if err := p.Decode(m.Payload); err != nil {
		return nil, err
	}
	return p, nil
}

func decodeHeartbeat(m *Msg) (Packable, error) {
	p := &HeartbeatPayload{}
	if err := p.Decode(m.Payload); err != nil {
		return nil, err
	}
	return p, nil
}

func decodeNotify(m *Msg) (Packable, error) {
	p := &NotifyPayload{}
	if err := p.Decode(m.Payload); err != nil {
		return nil, err
	}
	return p, nil
}

func decodeAck(m *Msg) (Packable, error) {
	p := &AckPayload{}
	if err := p.Decode(m.Payload); err != nil {
		return nil, err
	}
	return p, nil
}

func init() {
	RegisterPacketType(MsgTypeHandshake, decodeHandshake)
	RegisterPacketType(MsgTypeUpload, decodeUpload)
	RegisterPacketType(MsgTypeHeartbeat, decodeHeartbeat)
	RegisterPacketType(MsgTypeNotify, decodeNotify)
	// AckPayload 是通用 ACK，挂在三个 ack type 下
	RegisterPacketType(MsgTypeHandshakeAck, decodeAck)
	RegisterPacketType(MsgTypeUploadAck, decodeAck)
	RegisterPacketType(MsgTypeHeartbeatAck, decodeAck)
}

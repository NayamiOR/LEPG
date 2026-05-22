package msg

import (
	"LEPG/internal/errors"
	"LEPG/internal/utils"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
)

const (
	MagicNumber    uint16 = 0x4E59 // NY
	MagicSize      int    = 2
	VersionSize    int    = 1
	FlagsSize      int    = 1
	TypeSize       int    = 1
	MsgIDSize      int    = 2
	PayloadLenSize int    = 2
	TimestampSize  int    = 4
	ChecksumSize   int    = 2
	HeaderSize            = MagicSize + VersionSize + FlagsSize + TypeSize + MsgIDSize + PayloadLenSize + TimestampSize
	CrcSize        int    = 2
)

const (
	version uint8 = 1
)

var globalIDGen = &atomicIdGenerator{}

// 消息类型常量
const (
	MsgTypeHandshake uint8 = 0x01 // 握手消息
	MsgTypeAuth      uint8 = 0x02 // 认证消息
	MsgTypeHeartbeat uint8 = 0x03 // 心跳消息
	MsgTypeError     uint8 = 0x04 // 错误消息
	MsgTypeUpload    uint8 = 0x05 // 上传数据消息
)

type Msg struct {
	Magic      uint16
	Version    uint8
	Flags      uint8
	Type       uint8
	MsgID      uint16
	PayloadLen uint16
	Timestamp  utils.Timestamp
	Payload    []byte
	Checksum   uint16
}

type idGenerator interface {
	Next() uint16
}

type atomicIdGenerator struct {
	current atomic.Uint32
}

func (g *atomicIdGenerator) Next() uint16 {
	return uint16(g.current.Add(1) % 65536)
}

type MsgFactory struct {
	idGen idGenerator
}

// NewMsgFactory 创建新的消息工厂实例
func NewMsgFactory() *MsgFactory {
	return &MsgFactory{
		idGen: &atomicIdGenerator{},
	}
}

func (f *MsgFactory) NewMsg(flags uint8, t uint8, payload []byte) *Msg {
	return &Msg{
		Magic:      MagicNumber,
		Version:    version,
		Flags:      flags,
		Type:       t,
		MsgID:      f.idGen.Next(),
		PayloadLen: uint16(len(payload)),
		Timestamp:  utils.NewTimestamp(),
		Payload:    payload,
		Checksum:   utils.CalChecksum(payload),
	}
}

type Packable interface {
	Encode() ([]byte, error)
	Type() uint8
}

func (f *MsgFactory) NewFromPacket(packet Packable) *Msg {
	// TODO
	return nil
}

func (m *Msg) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write fixed-size fields
	binary.Write(buf, binary.BigEndian, m.Magic)
	binary.Write(buf, binary.BigEndian, m.Version)
	binary.Write(buf, binary.BigEndian, m.Flags)
	binary.Write(buf, binary.BigEndian, m.Type)
	binary.Write(buf, binary.BigEndian, m.MsgID)
	binary.Write(buf, binary.BigEndian, m.PayloadLen)
	binary.Write(buf, binary.BigEndian, m.Timestamp)

	// Write payload if present
	if m.Payload != nil {
		buf.Write(m.Payload)
	}

	// Write checksum
	binary.Write(buf, binary.BigEndian, m.Checksum)

	return buf.Bytes(), nil
}

func Decode(data []byte) (Msg, error) {
	var m Msg
	reader := bytes.NewReader(data)

	// Read fixed-size fields
	err := binary.Read(reader, binary.BigEndian, &m.Magic)
	if err != nil {
		return m, err
	}
	err = binary.Read(reader, binary.BigEndian, &m.Version)
	if err != nil {
		return m, err
	}
	err = binary.Read(reader, binary.BigEndian, &m.Flags)
	if err != nil {
		return m, err
	}
	err = binary.Read(reader, binary.BigEndian, &m.Type)
	if err != nil {
		return m, err
	}
	err = binary.Read(reader, binary.BigEndian, &m.MsgID)
	if err != nil {
		return m, err
	}
	err = binary.Read(reader, binary.BigEndian, &m.PayloadLen)
	if err != nil {
		return m, err
	}
	err = binary.Read(reader, binary.BigEndian, &m.Timestamp)
	if err != nil {
		return m, err
	}

	// Read payload if present
	if m.PayloadLen > 0 {
		m.Payload = make([]byte, m.PayloadLen)
		_, err = reader.Read(m.Payload)
		if err != nil {
			return m, err
		}
	} else {
		m.Payload = nil
	}

	// Read checksum
	err = binary.Read(reader, binary.BigEndian, &m.Checksum)
	if err != nil {
		return m, err
	}

	return m, nil
}

func DecodeFrame(conn net.Conn) (Msg, error) {
	var m Msg

	// Read head
	magicBuf := make([]byte, MagicSize)
	_, err := io.ReadFull(conn, magicBuf)
	if err != nil {
		return m, err
	}
	versionBuf := make([]byte, VersionSize)
	_, err = io.ReadFull(conn, versionBuf)
	if err != nil {
		return m, err
	}
	flagsBuf := make([]byte, FlagsSize)
	_, err = io.ReadFull(conn, flagsBuf)
	if err != nil {
		return m, err
	}
	typeBuf := make([]byte, TypeSize)
	_, err = io.ReadFull(conn, typeBuf)
	if err != nil {
		return m, err
	}
	msgIDBuf := make([]byte, MsgIDSize)
	_, err = io.ReadFull(conn, msgIDBuf)
	if err != nil {
		return m, err
	}
	payloadLenBuf := make([]byte, PayloadLenSize)
	_, err = io.ReadFull(conn, payloadLenBuf)
	if err != nil {
		return m, err
	}
	timestampBuf := make([]byte, TimestampSize)
	_, err = io.ReadFull(conn, timestampBuf)
	if err != nil {
		return m, err
	}

	payloadLen := binary.BigEndian.Uint16(payloadLenBuf)

	m.Magic = binary.BigEndian.Uint16(magicBuf)
	m.Version = versionBuf[0]
	m.Flags = flagsBuf[0]
	m.Type = typeBuf[0]
	m.MsgID = binary.BigEndian.Uint16(msgIDBuf)
	m.PayloadLen = payloadLen
	m.Timestamp = utils.Timestamp(binary.BigEndian.Uint32(timestampBuf))

	// Read payload and checksum
	payloadBuf := make([]byte, payloadLen)
	_, err = io.ReadFull(conn, payloadBuf)
	if err != nil {
		return m, err
	}
	checksumBuf := make([]byte, ChecksumSize)
	_, err = io.ReadFull(conn, checksumBuf)
	if err != nil {
		return m, err
	}

	m.Payload = payloadBuf
	m.Checksum = binary.BigEndian.Uint16(checksumBuf)

	// Calculate checksum and verify
	checksum := utils.CalChecksum(m.Payload)
	if checksum != m.Checksum {
		return m, errors.ErrChecksumMismatch
	}

	return m, nil
}

// New creates a new message with auto-generated MsgID
func New(msgType uint8, payload []byte) Msg {
	return Msg{
		Magic:      MagicNumber,
		Version:    version,
		Type:       msgType,
		MsgID:      globalIDGen.Next(),
		PayloadLen: uint16(len(payload)),
		Timestamp:  utils.NewTimestamp(),
		Payload:    payload,
		Checksum:   utils.CalChecksum(payload),
	}
}

// NewWithID creates a new message with custom MsgID
func NewWithID(flags uint8, msgID uint16, payload []byte) Msg {
	return Msg{
		Magic:      MagicNumber,
		Version:    version,
		Flags:      flags,
		MsgID:      msgID,
		PayloadLen: uint16(len(payload)),
		Timestamp:  utils.NewTimestamp(),
		Payload:    payload,
		Checksum:   utils.CalChecksum(payload),
	}
}

package msg

import (
	"LEPG/internal/errors"
	"LEPG/internal/utils"
	"bytes"
	"encoding/binary"
	stderrors "errors"
	"io"
	"net"
	"sync"
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
	MsgTypeUpload    uint8 = 0x02 // 上传数据消息
	MsgTypeHeartbeat uint8 = 0x03 // 心跳消息
	MsgTypeError     uint8 = 0x04 // 错误消息
)

// Flags 常量
const (
	FlagRequest  uint8 = 0x00
	FlagResponse uint8 = 0x01
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
	// 保留向后兼容的包装：编码 packet 并构造 Msg
	payload, err := packet.Encode()
	if err != nil {
		return nil
	}
	return &Msg{
		Magic:      MagicNumber,
		Version:    version,
		Flags:      0,
		Type:       packet.Type(),
		MsgID:      f.idGen.Next(),
		PayloadLen: uint16(len(payload)),
		Timestamp:  utils.NewTimestamp(),
		Payload:    payload,
		Checksum:   utils.CalChecksum(payload),
	}
}

// 注册制工厂：将消息类型映射到构造器，用于从 Msg 构造具体的 Packable
var (
	packetRegistry   = make(map[uint8]func(*Msg) (Packable, error))
	packetRegistryMu sync.RWMutex
)

// RegisterPacketType 注册一个消息类型的构造器
func RegisterPacketType(t uint8, ctor func(*Msg) (Packable, error)) {
	packetRegistryMu.Lock()
	defer packetRegistryMu.Unlock()
	packetRegistry[t] = ctor
}

// UnregisterPacketType 注销消息类型的构造器
func UnregisterPacketType(t uint8) {
	packetRegistryMu.Lock()
	defer packetRegistryMu.Unlock()
	delete(packetRegistry, t)
}

// ParseMsg 使用已注册的构造器将通用 Msg 转换为具体的 Packable
func ParseMsg(m *Msg) (Packable, error) {
	packetRegistryMu.RLock()
	ctor, ok := packetRegistry[m.Type]
	packetRegistryMu.RUnlock()
	if !ok {
		return nil, stderrors.New("unknown packet type")
	}
	return ctor(m)
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

// --- 示例 Packable 实现与注册 ---

type HandshakePacket struct {
	Handshake *HandshakePayload
	Response  *HandshakeResponsePayload
}

func (p *HandshakePacket) Encode() ([]byte, error) {
	if p.Handshake != nil {
		return p.Handshake.Encode()
	}
	if p.Response != nil {
		return p.Response.Encode()
	}
	return nil, nil
}

func (p *HandshakePacket) Type() uint8 { return MsgTypeHandshake }

func NewHandshakeRequestPacket(version uint8, sn, token string) *HandshakePacket {
	return &HandshakePacket{
		Handshake: &HandshakePayload{Version: version, Sn: sn, Token: token},
	}
}

func NewHandshakeResponsePacket(code uint8, message string) *HandshakePacket {
	return &HandshakePacket{
		Response: &HandshakeResponsePayload{Code: code, Message: message},
	}
}

type UploadPacket struct {
	Payload []byte
}

func (p *UploadPacket) Encode() ([]byte, error) { return p.Payload, nil }
func (p *UploadPacket) Type() uint8             { return MsgTypeUpload }
func NewUploadPacket(payload []byte) *UploadPacket {
	return &UploadPacket{Payload: payload}
}

type HeartbeatPacket struct {
	Payload []byte
}

func (p *HeartbeatPacket) Encode() ([]byte, error) { return p.Payload, nil }
func (p *HeartbeatPacket) Type() uint8             { return MsgTypeHeartbeat }
func NewHeartbeatPacket(payload []byte) *HeartbeatPacket {
	return &HeartbeatPacket{Payload: payload}
}

type ErrorPacket struct {
	Payload []byte
}

func (p *ErrorPacket) Encode() ([]byte, error) { return p.Payload, nil }
func (p *ErrorPacket) Type() uint8             { return MsgTypeError }
func NewErrorPacket(payload []byte) *ErrorPacket {
	return &ErrorPacket{Payload: payload}
}

func init() {
	RegisterPacketType(MsgTypeHandshake, func(m *Msg) (Packable, error) {
		if m == nil {
			return nil, nil
		}
		if m.Flags&FlagResponse != 0 {
			resp, err := DecodeHandshakeResponsePayload(m.Payload)
			if err != nil {
				return nil, err
			}
			return &HandshakePacket{Response: resp}, nil
		}
		hs, err := DecodeHandshakePayload(m.Payload)
		if err != nil {
			return nil, err
		}
		return &HandshakePacket{Handshake: hs}, nil
	})
	RegisterPacketType(MsgTypeUpload, func(m *Msg) (Packable, error) {
		if m == nil {
			return nil, nil
		}
		return &UploadPacket{Payload: m.Payload}, nil
	})
	RegisterPacketType(MsgTypeHeartbeat, func(m *Msg) (Packable, error) {
		if m == nil {
			return nil, nil
		}
		return &HeartbeatPacket{Payload: m.Payload}, nil
	})
	RegisterPacketType(MsgTypeError, func(m *Msg) (Packable, error) {
		if m == nil {
			return nil, nil
		}
		return &ErrorPacket{Payload: m.Payload}, nil
	})
}

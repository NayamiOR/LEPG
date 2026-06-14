package msg

import (
	"LEPG/internal/errors"
	"LEPG/internal/model"
	"LEPG/internal/utils"
	"bytes"
	"encoding/binary"
	stderrors "errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// 包级常量和类型定义
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

// 消息类型常量
const (
	MsgTypeHandshake    uint8 = iota + 1 // 握手消息
	MsgTypeHandshakeAck                  // 握手ACK
	MsgTypeUpload                        // 上传数据消息
	MsgTypeUploadAck                     // 上传ACK
	MsgTypeHeartbeat                     // 心跳消息
	MsgTypeHeartbeatAck                  // 心跳ACK
	MsgTypeNotify                        // 消息通知
)

// Flags 常量（暂时不用）
const ()

// Reason Code
const (
	Ok uint8 = iota + 1
	Failed
	BadSn
	BadToken
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

type Packable interface {
	Encode() ([]byte, error)
	Decode(data []byte) error
}

// ID generator interface and implementation
var globalIDGen = &atomicIdGenerator{}

type idGenerator interface {
	Next() uint16
}

type atomicIdGenerator struct {
	current atomic.Uint32
}

func (g *atomicIdGenerator) Next() uint16 {
	return uint16(g.current.Add(1) % 65536)
}

// 工厂

type MsgFactory struct {
	idGen idGenerator
}

func NewMsgFactory() *MsgFactory {
	return &MsgFactory{
		idGen: &atomicIdGenerator{},
	}
}

func (f *MsgFactory) NewMsg(t uint8, packet Packable) (*Msg, error) {
	defaultFlags := uint8(0)
	payload, err := packet.Encode()
	if err != nil {
		return nil, err
	}

	msg := &Msg{
		Magic:      MagicNumber,
		Version:    version,
		Flags:      defaultFlags,
		Type:       t,
		MsgID:      f.idGen.Next(),
		PayloadLen: uint16(len(payload)),
		Timestamp:  utils.NewTimestamp(),
		Payload:    payload,
	}
	msg.Checksum = utils.CalChecksum(msg.headerAndPayload())

	return msg, nil
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

// headerAndPayload 序列化固定头部字段与 payload，即 checksum 的覆盖范围。
func (m *Msg) headerAndPayload() []byte {
	buf := new(bytes.Buffer)

	// Fixed-size header fields
	binary.Write(buf, binary.BigEndian, m.Magic)
	binary.Write(buf, binary.BigEndian, m.Version)
	binary.Write(buf, binary.BigEndian, m.Flags)
	binary.Write(buf, binary.BigEndian, m.Type)
	binary.Write(buf, binary.BigEndian, m.MsgID)
	binary.Write(buf, binary.BigEndian, m.PayloadLen)
	binary.Write(buf, binary.BigEndian, m.Timestamp)

	// Payload if present
	if m.Payload != nil {
		buf.Write(m.Payload)
	}

	return buf.Bytes()
}

func (m *Msg) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Header + payload
	buf.Write(m.headerAndPayload())

	// Checksum
	binary.Write(buf, binary.BigEndian, m.Checksum)

	return buf.Bytes(), nil
}

func DecodeFrame(conn net.Conn) (Msg, error) {
	var m Msg

	// Read head
	magicBuf := make([]byte, MagicSize)
	_, err := io.ReadFull(conn, magicBuf)
	if err != nil {
		return m, err
	}
	if binary.BigEndian.Uint16(magicBuf) != MagicNumber {
		return m, errors.ErrInvalidMagic
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

	// Calculate checksum over header + payload and verify
	checksum := utils.CalChecksum(m.headerAndPayload())
	if checksum != m.Checksum {
		return m, errors.ErrChecksumMismatch
	}

	return m, nil
}

// New creates a new message with auto-generated MsgID
func New(msgType uint8, payload []byte) Msg {
	m := Msg{
		Magic:      MagicNumber,
		Version:    version,
		Type:       msgType,
		MsgID:      globalIDGen.Next(),
		PayloadLen: uint16(len(payload)),
		Timestamp:  utils.NewTimestamp(),
		Payload:    payload,
	}
	m.Checksum = utils.CalChecksum(m.headerAndPayload())
	return m
}

type AckPayload struct {
	MsgID uint16
	Code  uint8
}

type HandshakePayload struct {
	FirmwareVersion uint8
	Sn              string
	Token           string
}

type UploadPayload struct {
	Readings []model.Reading
}

type HeartbeatPayload struct {
}

type NotifyPayload struct {
	EventCode  uint8  // 事件类型（如 0x01=上线, 0x02=离线, 0x10=传感器故障）
	DeviceHash string // 关联设备 Hash
	Timestamp  uint32 // 事件发生的 Unix 秒级时间
	Severity   uint8  // 严重等级（0=信息, 1=警告, 2=严重, 3=致命）
	Message    string // 人类可读的描述
	RawData    []byte // 可选的附加结构化数据（如故障详情 JSON/二进制）
}

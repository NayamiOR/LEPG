package msg

import (
	"LEPG/internal/errors"
	"LEPG/internal/model"
	"LEPG/internal/utils"
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
	msg.Checksum = 0

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

func (m *Msg) Encode() ([]byte, error) {
	totalLen := HeaderSize + len(m.Payload) + CrcSize
	buf := make([]byte, totalLen)

	binary.BigEndian.PutUint16(buf[0:2], m.Magic)
	buf[2] = m.Version
	buf[3] = m.Flags
	buf[4] = m.Type
	binary.BigEndian.PutUint16(buf[5:7], m.MsgID)
	binary.BigEndian.PutUint16(buf[7:9], uint16(len(m.Payload)))
	binary.BigEndian.PutUint32(buf[9:13], uint32(m.Timestamp))

	copy(buf[HeaderSize:HeaderSize+len(m.Payload)], m.Payload)

	checksum := utils.CalChecksum(buf[:HeaderSize+len(m.Payload)])
	binary.BigEndian.PutUint16(buf[HeaderSize+len(m.Payload):], checksum)
	m.Checksum = checksum

	return buf, nil
}

func DecodeFrame(conn net.Conn) (Msg, error) {
	var m Msg

	// Read entire 13-byte header in one call.
	headerBuf := make([]byte, HeaderSize)
	if _, err := io.ReadFull(conn, headerBuf); err != nil {
		return m, err
	}

	// Parse header fields from slices — zero allocation.
	m.Magic = binary.BigEndian.Uint16(headerBuf[0:2])
	if m.Magic != MagicNumber {
		return m, errors.ErrInvalidMagic
	}
	m.Version = headerBuf[2]
	m.Flags = headerBuf[3]
	m.Type = headerBuf[4]
	m.MsgID = binary.BigEndian.Uint16(headerBuf[5:7])
	m.PayloadLen = binary.BigEndian.Uint16(headerBuf[7:9])
	m.Timestamp = utils.Timestamp(binary.BigEndian.Uint32(headerBuf[9:13]))

	// Read payload + CRC in one call.
	restLen := int(m.PayloadLen) + ChecksumSize
	restBuf := make([]byte, restLen)
	if _, err := io.ReadFull(conn, restBuf); err != nil {
		return m, err
	}

	m.Payload = restBuf[:m.PayloadLen]
	m.Checksum = binary.BigEndian.Uint16(restBuf[m.PayloadLen:])

	// CRC over [header | payload] — concatenate once instead of
	// re-serialising through headerAndPayload + binary.Write chain.
	full := make([]byte, 0, HeaderSize+int(m.PayloadLen))
	full = append(full, headerBuf...)
	full = append(full, m.Payload...)
	if checksum := utils.CalChecksum(full); checksum != m.Checksum {
		return m, errors.ErrChecksumMismatch
	}

	return m, nil
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

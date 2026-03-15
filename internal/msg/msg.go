package msg

import (
	"LEPG/internal/utils"
	"bytes"
	"encoding/binary"
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

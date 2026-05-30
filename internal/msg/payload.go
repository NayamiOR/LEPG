package msg

import "fmt"

// --- HandshakePayload (client -> server) ---
// Wire format: [Version:1B][SnLen:1B][Sn:nB][TokenLen:1B][Token:nB]

type HandshakePayload struct {
	Version uint8
	Sn      string
	Token   string
}

func (p *HandshakePayload) Encode() ([]byte, error) {
	buf := make([]byte, 0, 1+1+len(p.Sn)+1+len(p.Token))
	buf = append(buf, p.Version)
	buf = append(buf, byte(len(p.Sn)))
	buf = append(buf, []byte(p.Sn)...)
	buf = append(buf, byte(len(p.Token)))
	buf = append(buf, []byte(p.Token)...)
	return buf, nil
}

func DecodeHandshakePayload(data []byte) (*HandshakePayload, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("handshake payload too short: %d bytes", len(data))
	}
	offset := 0
	version := data[offset]
	offset++

	snLen := int(data[offset])
	offset++
	if offset+snLen > len(data) {
		return nil, fmt.Errorf("invalid sn length: %d", snLen)
	}
	sn := string(data[offset : offset+snLen])
	offset += snLen

	if offset >= len(data) {
		return nil, fmt.Errorf("missing token length")
	}
	tokenLen := int(data[offset])
	offset++
	if offset+tokenLen > len(data) {
		return nil, fmt.Errorf("invalid token length: %d", tokenLen)
	}
	token := string(data[offset : offset+tokenLen])

	return &HandshakePayload{
		Version: version,
		Sn:      sn,
		Token:   token,
	}, nil
}

// --- HandshakeResponsePayload (server -> client) ---
// Wire format: [Code:1B][MsgLen:1B][Msg:nB]

type HandshakeResponsePayload struct {
	Code    uint8
	Message string
}

const (
	HandshakeOK       uint8 = 0x00
	HandshakeFailed   uint8 = 0x01
	HandshakeBadSn    uint8 = 0x02
	HandshakeBadToken uint8 = 0x03
)

func (p *HandshakeResponsePayload) Encode() ([]byte, error) {
	buf := make([]byte, 0, 1+1+len(p.Message))
	buf = append(buf, p.Code)
	buf = append(buf, byte(len(p.Message)))
	buf = append(buf, []byte(p.Message)...)
	return buf, nil
}

func DecodeHandshakeResponsePayload(data []byte) (*HandshakeResponsePayload, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("response payload too short")
	}
	offset := 0
	code := data[offset]
	offset++

	if offset >= len(data) {
		return &HandshakeResponsePayload{Code: code, Message: ""}, nil
	}
	msgLen := int(data[offset])
	offset++

	if offset+msgLen > len(data) {
		return nil, fmt.Errorf("invalid message length: %d", msgLen)
	}
	message := string(data[offset : offset+msgLen])

	return &HandshakeResponsePayload{
		Code:    code,
		Message: message,
	}, nil
}

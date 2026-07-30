package utils

// CRC16-CCITT polynomial (0x1021)
const (
	crc16Poly = 0x1021
	crc16Init = 0xFFFF
)

var crc16Table [256]uint16

func init() {
	for i := range crc16Table {
		crc := uint16(i) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ crc16Poly
			} else {
				crc <<= 1
			}
		}
		crc16Table[i] = crc
	}
}

// CalChecksum calculates CRC16-CCITT checksum.
func CalChecksum(payload []byte) uint16 {
	crc := uint16(crc16Init)
	for _, b := range payload {
		crc = (crc << 8) ^ crc16Table[byte(crc>>8)^b]
	}
	return crc
}

package utils

// CRC16-CCITT polynomial (0x1021)
const (
	crc16Poly = 0x1021
	crc16Init = 0xFFFF
)

// CalChecksum calculates CRC16-CCITT checksum
// Uses the standard CRC-16-CCITT polynomial (0x1021)
// Initial value: 0xFFFF, Final XOR: 0x0000
func CalChecksum(payload []byte) uint16 {
	crc := uint16(crc16Init)

	for _, b := range payload {
		crc ^= uint16(b) << 8

		for range 8 {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ uint16(crc16Poly)
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

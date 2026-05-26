package model

// AccessType defines the access permissions for a data point
type AccessType string

const (
	AccessReadOnly  AccessType = "ro" // Read-only
	AccessReadWrite AccessType = "rw" // Read-write
	AccessWriteOnly AccessType = "wo" // Write-only
)

// DataType defines the Modbus data type
type DataType string

const (
	DataTypeBool    DataType = "bool"    // Boolean (coil/discrete input)
	DataTypeInt16   DataType = "int16"   // 16-bit signed integer
	DataTypeUint16  DataType = "uint16"  // 16-bit unsigned integer
	DataTypeInt32   DataType = "int32"   // 32-bit signed integer (2 registers)
	DataTypeUint32  DataType = "uint32"  // 32-bit unsigned integer (2 registers)
	DataTypeFloat32 DataType = "float32" // 32-bit float (2 registers, IEEE 754)
)

// ByteOrder defines the byte order for multi-register data types
type ByteOrder string

const (
	ByteOrderBigEndian       ByteOrder = "abcd" // Big-Endian (ABCD) - standard Modbus byte order
	ByteOrderLittleEndian    ByteOrder = "dcba" // Little-Endian (DCBA) - reversed byte order
	ByteOrderMidLittleEndian ByteOrder = "badc" // Mid-Little Endian (BADC) - byte big-endian, word little-endian
	ByteOrderMidBigEndian    ByteOrder = "cdab" // Mid-Big Endian (CDAB) - byte little-endian, word big-endian
)

// ConnectionType defines the Modbus connection type
type ConnectionType string

const (
	ConnectionTypeRTU ConnectionType = "rtu" // Modbus RTU (serial)
	ConnectionTypeTCP ConnectionType = "tcp" // Modbus TCP
)

// ByteOrderConversion converts bytes from one byte order to another
// For Modbus multi-register values (int32, uint32, float32)
func ByteOrderConversion(data []byte, order ByteOrder) []byte {
	if len(data) != 4 {
		return data // Only handle 4-byte values (2 registers)
	}

	switch order {
	case ByteOrderBigEndian: // ABCD - standard Modbus (no conversion)
		return data
	case ByteOrderLittleEndian: // DCBA - reverse all bytes
		return []byte{data[3], data[2], data[1], data[0]}
	case ByteOrderMidLittleEndian: // BADC - swap bytes within each word
		return []byte{data[1], data[0], data[3], data[2]}
	case ByteOrderMidBigEndian: // CDAB - swap words
		return []byte{data[2], data[3], data[0], data[1]}
	default:
		return data
	}
}

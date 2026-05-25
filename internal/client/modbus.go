package client

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/goburrow/modbus"
)

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

func TcpDevicePolling(dvc *DeviceConfig) error {
	slog.Info("Modbus TCP polling started", "device", dvc.Name)
	link := fmt.Sprintf("%s:%d", dvc.TCP.Host, dvc.TCP.Port)
	handler := modbus.NewTCPClientHandler(link)
	handler.SlaveId = dvc.SlaveID
	handler.Timeout = dvc.Timeout * time.Millisecond

	// Create Modbus client
	client := modbus.NewClient(handler)

	// Connect to TCP server
	err := handler.Connect()
	if err != nil {
		return err
	}
	defer handler.Close()

	slog.Info("Modbus TCP connected",
		"address", link,
		"slave_id", handler.SlaveId)

	pollInterval := dvc.PollInterval

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		for _, point := range dvc.Points {
			// slog.Info("Polling point", "point", point.Name, "function_code", point.FunctionCode)
			var results []byte
			var err error
			switch point.FunctionCode {
			case 1: // Read Coils
				results, err = client.ReadCoils(point.Address, point.Quantity)
			case 2: // Read Discrete Inputs
				results, err = client.ReadDiscreteInputs(point.Address, point.Quantity)
			case 3: // Read Holding Registers
				results, err = client.ReadHoldingRegisters(point.Address, point.Quantity)
			case 4: // Read Input Registers
				results, err = client.ReadInputRegisters(point.Address, point.Quantity)
			default:
				slog.Error("Unsupported function code", "point", point.Name, "function_code", point.FunctionCode)
				continue
			}
			if err != nil {
				slog.Error("Failed to read holding registers", "error", err)
				continue
			}

			var floatVal float64
			var boolVal bool
			originalResults := make([]byte, len(results))
			copy(originalResults, results) // 保存原始结果以供调试

			// TODO: 检查纠正解析逻辑
			switch point.DataType {
			case DataTypeBool:
				boolVal = results[0] != 0
			case DataTypeInt16:
				floatVal = float64(int16(results[0])<<8 | int16(results[1]))
			case DataTypeUint16:
				floatVal = float64(uint16(results[0])<<8 | uint16(results[1]))
			case DataTypeInt32:
				floatVal = float64(int32(results[0])<<24 | int32(results[1])<<16 | int32(results[2])<<8 | int32(results[3]))
			case DataTypeUint32:
				floatVal = float64(uint32(results[0])<<24 | uint32(results[1])<<16 | uint32(results[2])<<8 | uint32(results[3]))
			case DataTypeFloat32:
				// Convert 4 bytes to IEEE 754 float32
				if len(results) < 4 {
					slog.Error("Insufficient data for float32", "point", point.Name, "length", len(results))
					continue
				}
				// Debug: log raw data
				slog.Debug("Float32 raw data", "point", point.Name, "results", results, "len", len(results))

				// Apply byte order conversion
				converted := ByteOrderConversion(results[:4], point.ByteOrder)
				slog.Debug("Float32 converted", "point", point.Name, "converted", converted, "byte_order", point.ByteOrder)

				// Convert bytes to uint32 then to float32 using IEEE 754
				bits := binary.BigEndian.Uint32(converted)
				floatVal = float64(math.Float32frombits(bits))
				slog.Debug("Float32 final value", "point", point.Name, "bits", bits, "value", floatVal)
			}

			// Apply scale and offset for numeric types only
			if point.DataType != DataTypeBool {
				floatVal = floatVal*point.Scale + point.Offset
			}

			// Log based on data type
			if point.DataType == DataTypeBool {
				slog.Info("Modbus TCP point polling success",
					"point", point.Name,
					"type", point.DataType,
					"value", boolVal)
			} else {
				slog.Info("Modbus TCP point polling success",
					"point", point.Name,
					"type", point.DataType,
					"unit", point.Unit,
					"origin", originalResults,
					"value", floatVal)
			}
		}
	}
	slog.Info("For died")

	return nil
}

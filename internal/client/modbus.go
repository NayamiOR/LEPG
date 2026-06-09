package client

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"time"

	"LEPG/internal/model"

	"github.com/goburrow/modbus"
)

func TcpDevicePolling(channel chan model.Reading, dvc *DeviceConfig) error {
	slog.Info("Modbus TCP polling started", "device", dvc.Name)
	link := fmt.Sprintf("%s:%d", dvc.TCP.Host, dvc.TCP.Port)
	handler := modbus.NewTCPClientHandler(link)
	handler.SlaveId = dvc.SlaveID
	handler.Timeout = dvc.Timeout

	deviceHash, err := dvc.Hash()

	if err != nil {
		return err
	}

	// Create Modbus client
	client := modbus.NewClient(handler)

	// Connect to TCP server
	err = handler.Connect()
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
			case model.DataTypeBool:
				boolVal = results[0] != 0
			case model.DataTypeInt16:
				floatVal = float64(int16(results[0])<<8 | int16(results[1]))
			case model.DataTypeUint16:
				floatVal = float64(uint16(results[0])<<8 | uint16(results[1]))
			case model.DataTypeInt32:
				floatVal = float64(int32(results[0])<<24 | int32(results[1])<<16 | int32(results[2])<<8 | int32(results[3]))
			case model.DataTypeUint32:
				floatVal = float64(uint32(results[0])<<24 | uint32(results[1])<<16 | uint32(results[2])<<8 | uint32(results[3]))
			case model.DataTypeFloat32:
				// Convert 4 bytes to IEEE 754 float32
				if len(results) < 4 {
					slog.Error("Insufficient data for float32", "point", point.Name, "length", len(results))
					continue
				}
				// Debug: log raw data
				slog.Debug("Float32 raw data", "point", point.Name, "results", results, "len", len(results))

				// Apply byte order conversion
				converted := model.ByteOrderConversion(results[:4], point.ByteOrder)
				slog.Debug("Float32 converted", "point", point.Name, "converted", converted, "byte_order", point.ByteOrder)

				// Convert bytes to uint32 then to float32 using IEEE 754
				bits := binary.BigEndian.Uint32(converted)
				floatVal = float64(math.Float32frombits(bits))
				slog.Debug("Float32 final value", "point", point.Name, "bits", bits, "value", floatVal)
			}

			// Apply scale and offset for numeric types only
			if point.DataType != model.DataTypeBool {
				floatVal = floatVal*point.Scale + point.Offset
			}

			// Log based on data type
			slog.Info("Captured!")
			// if point.DataType == model.DataTypeBool {
			// 	slog.Info("Modbus TCP point polling success",
			// 		"point", point.Name,
			// 		"type", point.DataType,
			// 		"value", boolVal)
			// } else {
			// 	slog.Info("Modbus TCP point polling success",
			// 		"point", point.Name,
			// 		"type", point.DataType,
			// 		"unit", point.Unit,
			// 		"origin", originalResults,
			// 		"value", floatVal)
			// }

			reading := model.Reading{
				Device:     deviceHash,
				DeviceName: dvc.Name,
				Point:      model.HashPoint(dvc.Name, point.Name),
				PointName:  point.Name,
				DataType:   point.DataType,
				Value:      model.SerializeValue(point.DataType, floatVal, boolVal),
				Quality:    model.QualityGood,
				Unit:       point.Unit,
				Timestamp:  time.Now().UnixMilli(),
			}

			channel <- reading
		}
	}
	slog.Info("For died")

	return nil
}

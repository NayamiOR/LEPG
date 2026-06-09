package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"time"

	"LEPG/internal/model"

	"github.com/goburrow/modbus"
)

// connectableHandler extends modbus.ClientHandler with Connect/Close for lifecycle management.
// Both TCPClientHandler and RTUClientHandler satisfy this interface.
type connectableHandler interface {
	modbus.ClientHandler
	Connect() error
	Close() error
}

// createHandler creates a Modbus client handler based on device connection type.
func createHandler(dvc *DeviceConfig) (connectableHandler, error) {
	switch dvc.Type {
	case model.ConnectionTypeTCP:
		link := fmt.Sprintf("%s:%d", dvc.TCP.Host, dvc.TCP.Port)
		handler := modbus.NewTCPClientHandler(link)
		handler.SlaveId = dvc.SlaveID
		handler.Timeout = dvc.Timeout
		return handler, nil
	case model.ConnectionTypeRTU:
		handler := modbus.NewRTUClientHandler(dvc.RTU.Port)
		handler.SlaveId = dvc.SlaveID
		handler.Timeout = dvc.Timeout
		handler.BaudRate = dvc.RTU.BaudRate
		handler.DataBits = dvc.RTU.DataBits
		handler.StopBits = dvc.RTU.StopBits
		handler.Parity = dvc.RTU.Parity
		return handler, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %s", dvc.Type)
	}
}

func ModbusDevicePolling(ctx context.Context, channel chan model.Reading, dvc *DeviceConfig) error {
	slog.Info("Modbus polling started", "device", dvc.Name, "type", dvc.Type)

	handler, err := createHandler(dvc)
	if err != nil {
		return err
	}

	deviceHash, err := dvc.Hash()
	if err != nil {
		return err
	}

	// Create Modbus client
	client := modbus.NewClient(handler)

	// Connect
	err = handler.Connect()
	if err != nil {
		return err
	}
	defer handler.Close()

	slog.Info("Modbus connected",
		"type", dvc.Type,
		"slave_id", dvc.SlaveID)

	pollInterval := dvc.PollInterval

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("Modbus TCP polling stopped", "device", dvc.Name)
			return nil
		case <-ticker.C:
		}

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

			var value any
			originalResults := make([]byte, len(results))
			copy(originalResults, results) // 保存原始结果以供调试

			// TODO: 检查纠正解析逻辑
			switch point.DataType {
			case model.DataTypeBool:
				value = results[0] != 0
			case model.DataTypeInt16:
				value = float64(int16(results[0])<<8 | int16(results[1]))
			case model.DataTypeUint16:
				value = float64(uint16(results[0])<<8 | uint16(results[1]))
			case model.DataTypeInt32:
				value = float64(int32(results[0])<<24 | int32(results[1])<<16 | int32(results[2])<<8 | int32(results[3]))
			case model.DataTypeUint32:
				value = float64(uint32(results[0])<<24 | uint32(results[1])<<16 | uint32(results[2])<<8 | uint32(results[3]))
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
				value = float64(math.Float32frombits(bits))
				slog.Debug("Float32 final value", "point", point.Name, "bits", bits, "value", value)
			}

			// Apply scale and offset for numeric types only
			if point.DataType != model.DataTypeBool {
				value = float64(value.(float64))*point.Scale + point.Offset
			}

			// Log based on data type
			slog.Info("Modbus TCP reading",
				"point", point.Name,
				"type", point.DataType,
				"unit", point.Unit,
				"value", value)

			reading := model.Reading{
				Device:     deviceHash,
				DeviceName: dvc.Name,
				Point:      model.HashPoint(dvc.Name, point.Name),
				PointName:  point.Name,
				DataType:   point.DataType,
				Value:      model.SerializeValue(point.DataType, value),
				Quality:    model.QualityGood,
				Unit:       point.Unit,
				Timestamp:  time.Now().UnixMilli(),
			}

			channel <- reading
		}
	}
}

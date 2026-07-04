package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"

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

// parsePointValue converts raw Modbus response bytes into a typed value (bool or float64).
// Returns nil if the data can't be parsed (e.g. insufficient bytes).
func parsePointValue(point *ModbusPointConfig, results []byte) any {
	switch point.DataType {
	case model.DataTypeBool:
		return results[0] != 0
	case model.DataTypeInt16:
		v := float64(int16(results[0])<<8 | int16(results[1]))
		return v*point.Scale + point.Offset
	case model.DataTypeUint16:
		v := float64(uint16(results[0])<<8 | uint16(results[1]))
		return v*point.Scale + point.Offset
	case model.DataTypeInt32:
		v := float64(int32(results[0])<<24 | int32(results[1])<<16 | int32(results[2])<<8 | int32(results[3]))
		return v*point.Scale + point.Offset
	case model.DataTypeUint32:
		v := float64(uint32(results[0])<<24 | uint32(results[1])<<16 | uint32(results[2])<<8 | uint32(results[3]))
		return v*point.Scale + point.Offset
	case model.DataTypeFloat32:
		if len(results) < 4 {
			return nil
		}
		converted := model.ByteOrderConversion(results[:4], point.ByteOrder)
		bits := binary.BigEndian.Uint32(converted)
		v := float64(math.Float32frombits(bits))
		return v*point.Scale + point.Offset
	default:
		return nil
	}
}

func ModbusDevicePolling(ctx context.Context, channel chan model.Reading, dvc *DeviceConfig) error {
	rt, err := NewModbusRuntime(dvc)
	if err != nil {
		return err
	}
	registerModbusRuntime(dvc.Name, rt)
	defer unregisterModbusRuntime(dvc.Name)

	if err := rt.Start(ctx, channel); err != nil {
		return err
	}
	<-ctx.Done()
	rt.Stop()
	slog.Info("Modbus polling stopped", "device", dvc.Name)
	return nil
}

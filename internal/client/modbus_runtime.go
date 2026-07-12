package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"LEPG/internal/model"

	"github.com/goburrow/modbus"
)

const (
	stateOffline  int32 = 0
	stateOnline   int32 = 1
	stateDegraded int32 = 2
)

// ModbusRuntime wraps a Modbus device connection with poll+write goroutines.
type ModbusRuntime struct {
	cfg     *DeviceConfig
	client  modbus.Client
	handler connectableHandler
	state   atomic.Int32
	writeCh chan writeCmd
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type writeCmd struct {
	pointName string
	value     float64
	resultCh  chan error
}

// NewModbusRuntime creates a ModbusRuntime without connecting.
func NewModbusRuntime(cfg *DeviceConfig) (*ModbusRuntime, error) {
	handler, err := createHandler(cfg)
	if err != nil {
		return nil, err
	}
	return &ModbusRuntime{
		cfg:     cfg,
		client:  modbus.NewClient(handler),
		handler: handler,
		writeCh: make(chan writeCmd, 16),
		stopCh:  make(chan struct{}),
	}, nil
}

// Start connects to the device and launches poll and write goroutines.
func (rt *ModbusRuntime) Start(ctx context.Context, readingCh chan<- model.Reading) error {
	if err := rt.handler.Connect(); err != nil {
		return err
	}
	rt.state.Store(stateOnline)

	slog.Info("Modbus connected",
		"device", rt.cfg.Name,
		"type", rt.cfg.Type,
		"slave_id", rt.cfg.SlaveID)

	rt.wg.Add(2)
	go rt.pollLoop(ctx, readingCh)
	go rt.writeLoop(ctx)
	return nil
}

// Stop signals both goroutines to exit and closes the connection.
func (rt *ModbusRuntime) Stop() error {
	close(rt.stopCh)
	rt.wg.Wait()
	return rt.handler.Close()
}

// Write sends a write command to the device. It blocks until the write completes.
// Returns an error if the device is offline or the write fails.
func (rt *ModbusRuntime) Write(pointName string, value float64) error {
	if rt.state.Load() != stateOnline {
		return fmt.Errorf("device %s is not online (state=%d)", rt.cfg.Name, rt.state.Load())
	}

	point := rt.findPoint(pointName)
	if point == nil {
		return fmt.Errorf("point %s not found on device %s", pointName, rt.cfg.Name)
	}
	if point.Access == model.AccessReadOnly {
		return fmt.Errorf("point %s is read-only", pointName)
	}

	cmd := writeCmd{
		pointName: pointName,
		value:     value,
		resultCh:  make(chan error, 1),
	}

	select {
	case rt.writeCh <- cmd:
	default:
		return fmt.Errorf("write channel full for device %s", rt.cfg.Name)
	}

	return <-cmd.resultCh
}

func (rt *ModbusRuntime) findPoint(name string) *ModbusPointConfig {
	for _, p := range rt.cfg.Points {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// pollLoop periodically reads all readable points from the device.
func (rt *ModbusRuntime) pollLoop(ctx context.Context, readingCh chan<- model.Reading) {
	defer rt.wg.Done()

	ticker := time.NewTicker(rt.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rt.stopCh:
			return
		case <-ticker.C:
		}

		rt.pollOnce(readingCh)
	}
}

func (rt *ModbusRuntime) pollOnce(readingCh chan<- model.Reading) {
	deviceHash, err := rt.cfg.Hash()
	if err != nil {
		slog.Error("Failed to hash device", "device", rt.cfg.Name, "error", err)
		return
	}

	hadError := false
	for _, point := range rt.cfg.Points {
		if point.Access == model.AccessWriteOnly {
			continue
		}
		if isWriteFunctionCode(point.FunctionCode) {
			continue
		}

		var results []byte
		switch point.FunctionCode {
		case 1:
			results, err = rt.client.ReadCoils(point.Address, point.Quantity)
		case 2:
			results, err = rt.client.ReadDiscreteInputs(point.Address, point.Quantity)
		case 3:
			results, err = rt.client.ReadHoldingRegisters(point.Address, point.Quantity)
		case 4:
			results, err = rt.client.ReadInputRegisters(point.Address, point.Quantity)
		default:
			continue
		}
		if err != nil {
			slog.Warn("Failed to read point", "device", rt.cfg.Name, "point", point.Name, "error", err)
			hadError = true
			continue
		}

		value := parsePointValue(point, results)
		if value == nil {
			continue
		}

		logReading("modbus", rt.cfg.Name, point.Name, point.DataType, value, point.Unit, time.Now().UnixMilli())

		reading := model.Reading{
			Device:     deviceHash,
			DeviceName: rt.cfg.Name,
			Point:      model.HashPoint(rt.cfg.Name, point.Name),
			PointName:  point.Name,
			DataType:   point.DataType,
			Value:      model.SerializeValue(point.DataType, value),
			Quality:    model.QualityGood,
			Unit:       point.Unit,
			Timestamp:  time.Now().UnixMilli(),
		}

		readingCh <- reading
	}

	if hadError {
		rt.state.Store(stateDegraded)
	} else {
		rt.state.Store(stateOnline)
	}
}

// writeLoop consumes write commands from the channel and executes them.
func (rt *ModbusRuntime) writeLoop(ctx context.Context) {
	defer rt.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rt.stopCh:
			return
		case cmd := <-rt.writeCh:
			err := rt.executeWrite(&cmd)
			if err != nil {
				rt.state.Store(stateDegraded)
				slog.Warn("Write failed", "device", rt.cfg.Name, "point", cmd.pointName, "error", err)
			}
			cmd.resultCh <- err
		}
	}
}

func (rt *ModbusRuntime) executeWrite(cmd *writeCmd) error {
	point := rt.findPoint(cmd.pointName)
	if point == nil {
		return fmt.Errorf("point %s not found", cmd.pointName)
	}

	// 检查 function_code 与 data_type 兼容性
	switch point.FunctionCode {
	case 5:
		if point.DataType != model.DataTypeBool {
			return fmt.Errorf("FC5 requires bool data type, got %s", point.DataType)
		}
	case 6:
		if point.DataType == model.DataTypeBool {
			return fmt.Errorf("FC6 requires numeric data type, got bool (use FC5 for coils)")
		}
		if point.DataType == model.DataTypeInt32 || point.DataType == model.DataTypeUint32 || point.DataType == model.DataTypeFloat32 {
			return fmt.Errorf("FC6 requires int16/uint16 data type, got %s (use FC16 for multi-register)", point.DataType)
		}
	case 16:
		if point.DataType == model.DataTypeBool || point.DataType == model.DataTypeInt16 || point.DataType == model.DataTypeUint16 {
			return fmt.Errorf("FC16 requires int32/uint32/float32 data type, got %s (use FC6 for single register)", point.DataType)
		}
	default:
		return fmt.Errorf("unsupported write function code: %d", point.FunctionCode)
	}

	// Reverse scale/offset: raw = (value - offset) / scale
	raw := (cmd.value - point.Offset) / point.Scale

	switch point.FunctionCode {
	case 5:
		return rt.writeCoil(point.Address, raw)
	case 6:
		return rt.writeSingleReg(point.Address, raw, point.DataType)
	case 16:
		return rt.writeMultiReg(point.Address, raw, point.DataType, point.ByteOrder)
	default:
		return fmt.Errorf("unsupported write function code: %d", point.FunctionCode)
	}
}

func (rt *ModbusRuntime) writeCoil(address uint16, value float64) error {
	var coilValue uint16
	if value != 0 {
		coilValue = 0xFF00
	}
	_, err := rt.client.WriteSingleCoil(address, coilValue)
	return err
}

func (rt *ModbusRuntime) writeSingleReg(address uint16, value float64, dt model.DataType) error {
	var regValue uint16
	switch dt {
	case model.DataTypeInt16:
		regValue = uint16(int16(value))
	case model.DataTypeUint16:
		regValue = uint16(value)
	default:
		return fmt.Errorf("FC6 requires int16/uint16 data type, got %s", dt)
	}
	_, err := rt.client.WriteSingleRegister(address, regValue)
	return err
}

func (rt *ModbusRuntime) writeMultiReg(address uint16, value float64, dt model.DataType, byteOrder model.ByteOrder) error {
	var buf [4]byte
	switch dt {
	case model.DataTypeInt32:
		binary.BigEndian.PutUint32(buf[:], uint32(int32(value)))
	case model.DataTypeUint32:
		binary.BigEndian.PutUint32(buf[:], uint32(value))
	case model.DataTypeFloat32:
		binary.BigEndian.PutUint32(buf[:], math.Float32bits(float32(value)))
	default:
		return fmt.Errorf("FC16 requires int32/uint32/float32 data type, got %s", dt)
	}

	deviceBytes := model.ByteOrderConversion(buf[:], byteOrder)
	_, err := rt.client.WriteMultipleRegisters(address, 2, deviceBytes)
	return err
}

func isWriteFunctionCode(fc int) bool {
	return fc == 5 || fc == 6 || fc == 16
}

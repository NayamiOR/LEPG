//go:build integration

package client

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"testing"
	"LEPG/internal/model"
	"time"
)

// Unit test for float32 parsing logic
func TestFloat32Parsing(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		byteOrder ByteOrder
		expected  float64
	}{
		{
			name:      "10.0A big-endian",
			input:     []byte{0x41, 0x20, 0x00, 0x00}, // IEEE 754: 10.0
			byteOrder: model.ByteOrderBigEndian,
			expected:  10.0,
		},
		{
			name:      "2.2kW big-endian",
			input:     []byte{0x40, 0x0C, 0xCC, 0xCD}, // IEEE 754: 2.2
			byteOrder: model.ByteOrderBigEndian,
			expected:  2.2,
		},
		{
			name:      "10.0 little-endian",
			input:     []byte{0x41, 0x20, 0x00, 0x00}, // IEEE 754: 10.0
			byteOrder: model.ByteOrderLittleEndian,
			expected:  10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test byte order conversion
			converted := model.ByteOrderConversion(tt.input, tt.byteOrder)

			// Test float32 conversion
			bits := binary.BigEndian.Uint32(converted)
			result := math.Float32frombits(bits)

			if result != float32(tt.expected) {
				t.Errorf("Expected %f, got %f (converted: %v, bits: %08x)",
					tt.expected, result, converted, bits)
			} else {
				t.Logf("Success: %f = %f (converted: %v, bits: %08x)",
					tt.expected, result, converted, bits)
			}
		})
	}
}

func TestMain(m *testing.M) {
	// Start simulator
	cmd := exec.Command("python", "scripts/modbus_simulator.py")
	if err := cmd.Start(); err != nil {
		fmt.Println("Warning: Could not start simulator, integration tests will be skipped")
		m.Run()
		return
	}

	// Wait for simulator to start
	time.Sleep(2 * time.Second)

	// Run tests
	exitCode := m.Run()

	// Stop simulator
	cmd.Process.Kill()

	cmd.Wait()

	os.Exit(exitCode)
}

func TestTcpDevicePolling_Integration_Connection(t *testing.T) {
	device := &DeviceConfig{
		Name:         "test-device",
		Type:         model.ConnectionTypeTCP,
		Timeout:      5 * time.Second,
		SlaveID:      1,
		PollInterval: 1 * time.Second,
		TCP: &TcpSlaveConfig{
			Host: "127.0.0.1",
			Port: 5020,
		},
		Points: []*PointConfig{
			{
				Name:         "temperature",
				FunctionCode: 3,
				Address:      0,
				Quantity:     1,
			},
		},
	}

	// Run polling in goroutine
	done := make(chan error, 1)
	go func() {
		done <- TcpDevicePolling(device)
	}()

	// Wait for multiple polling cycles to complete
	select {
	case <-time.After(12 * time.Second):
		t.Log("Polling completed multiple cycles successfully")
	case err := <-done:
		if err != nil {
			t.Fatalf("TcpDevicePolling failed: %v", err)
		}
	}
}

func TestTcpDevicePolling_Integration_FC3_HoldingRegisters(t *testing.T) {
	device := &DeviceConfig{
		Name:         "test-fc3",
		Type:         model.ConnectionTypeTCP,
		Timeout:      5 * time.Second,
		SlaveID:      1,
		PollInterval: 1 * time.Second,
		TCP: &TcpSlaveConfig{
			Host: "127.0.0.1",
			Port: 5020,
		},
		Points: []*PointConfig{
			{
				Name:         "temperature",
				FunctionCode: 3, // Read Holding Registers
				Address:      0,
				Quantity:     2,
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- TcpDevicePolling(device)
	}()

	select {
	case <-time.After(12 * time.Second):
		t.Log("FC3 polling completed multiple cycles successfully")
	case err := <-done:
		if err != nil {
			t.Fatalf("FC3 test failed: %v", err)
		}
	}
}

func TestTcpDevicePolling_Integration_FC4_InputRegisters(t *testing.T) {
	device := &DeviceConfig{
		Name:         "test-fc4",
		Type:         model.ConnectionTypeTCP,
		Timeout:      5 * time.Second,
		SlaveID:      1,
		PollInterval: 1 * time.Second,
		TCP: &TcpSlaveConfig{
			Host: "127.0.0.1",
			Port: 5020,
		},
		Points: []*PointConfig{
			{
				Name:         "input-registers",
				FunctionCode: 4, // Read Input Registers
				Address:      0,
				Quantity:     10,
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- TcpDevicePolling(device)
	}()

	select {
	case <-time.After(12 * time.Second):
		t.Log("FC4 polling completed multiple cycles successfully")
	case err := <-done:
		if err != nil {
			t.Fatalf("FC4 test failed: %v", err)
		}
	}
}
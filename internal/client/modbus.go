package client

import (
	"fmt"
	"log/slog"
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

// ConnectionType defines the Modbus connection type
type ConnectionType string

const (
	ConnectionTypeRTU ConnectionType = "rtu" // Modbus RTU (serial)
	ConnectionTypeTCP ConnectionType = "tcp" // Modbus TCP
)

// ModbusRTUExample demonstrates Modbus RTU connection and polling
// This example uses hardcoded parameters for testing
func ModbusRTUExample() error {
	// Hardcoded RTU parameters for testing
	// Windows: "COM3", Linux: "/dev/ttyUSB0"
	handler := modbus.NewRTUClientHandler("/dev/ttyUSB0")
	handler.BaudRate = 9600
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	// handler.SlaveId = 0
	handler.Timeout = 5 * time.Second

	// Create Modbus client
	client := modbus.NewClient(handler)

	// Connect to serial port
	err := handler.Connect()
	if err != nil {
		return err
	}
	defer handler.Close()

	slog.Info("Modbus RTU connected",
		"port", "/dev/ttyUSB0",
		"baud_rate", 9600,
		"slave_id", handler.SlaveId)

	// Polling loop
	pollInterval := 5 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Read holding registers (function code 3)
		// Reading 10 registers starting from address 0
		// results, err := client.ReadHoldingRegisters(0, 10)
		results, err := client.ReadInputRegisters(0, 10)
		if err != nil {
			slog.Error("Failed to read holding registers", "error", err)
			continue
		}

		// Log results
		slog.Info("Modbus RTU polling success",
			"address", 0,
			"quantity", 10,
			"values", results)
	}
	return nil
}

// ModbusTCPExample demonstrates Modbus TCP connection and polling
// This example uses hardcoded parameters for testing
func ModbusTCPExample() error {
	// Hardcoded TCP parameters for testing
	// Note: Using port 5020 to match built-in simulator (scripts/modbus_simulator.py)
	// Change to 502 for standard Modbus devices
	handler := modbus.NewTCPClientHandler("127.0.0.1:5020")
	handler.SlaveId = 1 // 匹配模拟器的 slave_id
	handler.Timeout = 5 * time.Second

	// Create Modbus client
	client := modbus.NewClient(handler)

	// Connect to TCP server
	err := handler.Connect()
	if err != nil {
		return err
	}
	defer handler.Close()

	slog.Info("Modbus TCP connected",
		"address", "127.0.0.1:5020",
		"slave_id", handler.SlaveId)

	// Polling loop
	pollInterval := 1 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Read holding registers (function code 3)
		// Reading 2 registers starting from address 0
		// Simulator: HR[0]=250(temp), HR[1]=650(humidity)
		results, err := client.ReadHoldingRegisters(0, 2)
		if err != nil {
			slog.Error("Failed to read holding registers", "error", err)
			continue
		}

		// Log results
		slog.Info("Modbus TCP polling success",
			"address", 0,
			"quantity", 2,
			"values", results)
	}

	return nil
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
			slog.Info("Polling point", "point", point.Name, "function_code", point.FunctionCode)
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

			slog.Info("Modbus TCP point polling success",
				"point", point.Name,
				"values", results)
		}
	}
	slog.Info("For died")

	return nil
}

// ModbusPollingExample starts the Modbus polling loop
// By default uses TCP for easier testing (can be switched to RTU)
func ModbusPollingExample() error {
	// For testing, we use TCP by default
	// To test RTU, comment out TCP and uncomment RTU below

	// TCP example (default - easier to test with simulators)
	return ModbusTCPExample()

	// RTU example (requires serial hardware or virtual port)
	// return ModbusRTUExample()
}

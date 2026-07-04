package client

import (
	"LEPG/internal/model"
	"testing"
	"time"
)

// --- parsePointValue unit tests ---

func TestParsePointValue_Bool(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "coil",
		DataType: model.DataTypeBool,
	}
	result := parsePointValue(point, []byte{0x00})
	if result.(bool) != false {
		t.Error("expected false")
	}
	result = parsePointValue(point, []byte{0x01})
	if result.(bool) != true {
		t.Error("expected true")
	}
	result = parsePointValue(point, []byte{0xFF})
	if result.(bool) != true {
		t.Error("expected true for non-zero")
	}
}

func TestParsePointValue_Int16(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "temp",
		DataType: model.DataTypeInt16,
		Scale:    0.1,
		Offset:   0,
	}
	// 250 → 25.0
	result := parsePointValue(point, []byte{0x00, 0xFA})
	if result.(float64) != 25.0 {
		t.Errorf("expected 25.0, got %v", result)
	}
	// -10 → -1.0
	result = parsePointValue(point, []byte{0xFF, 0xF6})
	if result.(float64) != -1.0 {
		t.Errorf("expected -1.0, got %v", result)
	}
}

func TestParsePointValue_Uint16(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "humidity",
		DataType: model.DataTypeUint16,
		Scale:    0.1,
		Offset:   0,
	}
	result := parsePointValue(point, []byte{0x02, 0x8A}) // 650 → 65.0
	if result.(float64) != 65.0 {
		t.Errorf("expected 65.0, got %v", result)
	}
}

func TestParsePointValue_Int32(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "counter",
		DataType: model.DataTypeInt32,
		Scale:    1.0,
		Offset:   0,
	}
	result := parsePointValue(point, []byte{0x00, 0x00, 0x03, 0xE8}) // 1000
	if result.(float64) != 1000.0 {
		t.Errorf("expected 1000.0, got %v", result)
	}
}

func TestParsePointValue_Uint32(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "big_counter",
		DataType: model.DataTypeUint32,
		Scale:    1.0,
		Offset:   0,
	}
	result := parsePointValue(point, []byte{0x00, 0x00, 0x13, 0x88}) // 5000
	if result.(float64) != 5000.0 {
		t.Errorf("expected 5000.0, got %v", result)
	}
}

func TestParsePointValue_Float32(t *testing.T) {
	point := &ModbusPointConfig{
		Name:      "current",
		DataType:  model.DataTypeFloat32,
		ByteOrder: model.ByteOrderBigEndian,
		Scale:     1.0,
		Offset:    0,
	}
	// IEEE 754 big-endian: 10.0 = 0x41200000
	result := parsePointValue(point, []byte{0x41, 0x20, 0x00, 0x00})
	if result.(float64) != 10.0 {
		t.Errorf("expected 10.0, got %v", result)
	}
}

func TestParsePointValue_Float32_LittleEndian(t *testing.T) {
	point := &ModbusPointConfig{
		Name:      "power",
		DataType:  model.DataTypeFloat32,
		ByteOrder: model.ByteOrderLittleEndian,
		Scale:     1.0,
		Offset:    0,
	}
	// 2.2 = 0x400CCCCD big-endian → little endian swap
	result := parsePointValue(point, []byte{0xCD, 0xCC, 0x0C, 0x40})
	val := result.(float64)
	if val < 2.19 || val > 2.21 {
		t.Errorf("expected ~2.2, got %v", val)
	}
}

func TestParsePointValue_WithScaleOffset(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "voltage",
		DataType: model.DataTypeUint16,
		Scale:    0.1,
		Offset:   10.0,
	}
	result := parsePointValue(point, []byte{0x08, 0x98}) // 2200*0.1+10 = 230.0
	if result.(float64) != 230.0 {
		t.Errorf("expected 230.0, got %v", result)
	}
}

func TestParsePointValue_InsufficientFloat32(t *testing.T) {
	point := &ModbusPointConfig{
		Name:     "bad_float",
		DataType: model.DataTypeFloat32,
	}
	result := parsePointValue(point, []byte{0x41}) // only 1 byte
	if result != nil {
		t.Error("expected nil for insufficient float32 data")
	}
}

// --- Write error conditions ---

func TestWrite_RejectsOffline(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name: "test-device",
			Points: []*ModbusPointConfig{
				{
					Name:         "setpoint",
					FunctionCode: 6,
					DataType:     model.DataTypeUint16,
					Access:       model.AccessReadWrite,
					Address:      10,
				},
			},
		},
	}
	// rt.state defaults to 0 (offline)
	err := rt.Write("setpoint", 100.0)
	if err == nil {
		t.Error("expected error for offline device")
	}
}

func TestWrite_RejectsReadOnly(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name: "test-device",
			Points: []*ModbusPointConfig{
				{
					Name:         "temperature",
					FunctionCode: 3,
					DataType:     model.DataTypeInt16,
					Access:       model.AccessReadOnly,
					Address:      0,
				},
			},
		},
	}
	rt.state.Store(stateOnline)

	err := rt.Write("temperature", 25.0)
	if err == nil {
		t.Error("expected error for read-only point")
	}
}

func TestWrite_PointNotFound(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name:   "test-device",
			Points: []*ModbusPointConfig{},
		},
	}
	rt.state.Store(stateOnline)

	err := rt.Write("nonexistent", 100.0)
	if err == nil {
		t.Error("expected error for missing point")
	}
}

func TestWrite_FC5RequiresBool(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name: "test-device",
			Points: []*ModbusPointConfig{
				{
					Name:         "coil",
					FunctionCode: 5,
					DataType:     model.DataTypeUint16, // wrong: FC5 needs bool
					Access:       model.AccessReadWrite,
					Address:      0,
				},
			},
		},
	}
	rt.state.Store(stateOnline)

	err := rt.Write("coil", 1.0)
	if err == nil {
		t.Error("expected error for FC5 with non-bool data type")
	}
}

func TestWrite_FC6RejectsBool(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name: "test-device",
			Points: []*ModbusPointConfig{
				{
					Name:         "reg",
					FunctionCode: 6,
					DataType:     model.DataTypeBool, // wrong: FC6 needs numeric
					Access:       model.AccessReadWrite,
					Address:      10,
				},
			},
		},
	}
	rt.state.Store(stateOnline)

	err := rt.Write("reg", 1.0)
	if err == nil {
		t.Error("expected error for FC6 with bool data type")
	}
}

func TestWrite_FC6RejectsMultiRegisterType(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name: "test-device",
			Points: []*ModbusPointConfig{
				{
					Name:         "reg",
					FunctionCode: 6,
					DataType:     model.DataTypeFloat32, // wrong: FC6 needs single reg type
					Access:       model.AccessReadWrite,
					Address:      10,
				},
			},
		},
	}
	rt.state.Store(stateOnline)

	err := rt.Write("reg", 10.0)
	if err == nil {
		t.Error("expected error for FC6 with float32 data type")
	}
}

func TestWrite_FC16RejectsSingleRegisterType(t *testing.T) {
	rt := &ModbusRuntime{
		cfg: &DeviceConfig{
			Name: "test-device",
			Points: []*ModbusPointConfig{
				{
					Name:         "reg",
					FunctionCode: 16,
					DataType:     model.DataTypeUint16, // wrong: FC16 needs multi-reg type
					Access:       model.AccessReadWrite,
					Address:      10,
				},
			},
		},
	}
	rt.state.Store(stateOnline)

	err := rt.Write("reg", 100.0)
	if err == nil {
		t.Error("expected error for FC16 with uint16 data type")
	}
}

// --- Registry tests ---

func TestModbusRuntimeRegistry(t *testing.T) {
	cfg := &DeviceConfig{
		Name:         "registry-test",
		Type:         model.ConnectionTypeTCP,
		Timeout:      5 * time.Second,
		SlaveID:      1,
		PollInterval: 10 * time.Second,
		TCP: &TcpSlaveConfig{
			Host: "127.0.0.1",
			Port: 9999,
		},
		Points: []*ModbusPointConfig{
			{
				Name:         "test",
				FunctionCode: 3,
				Address:      0,
				Quantity:     1,
				DataType:     model.DataTypeInt16,
				Access:       model.AccessReadOnly,
			},
		},
	}

	rt, err := NewModbusRuntime(cfg)
	if err != nil {
		t.Fatalf("NewModbusRuntime failed: %v", err)
	}

	registerModbusRuntime("registry-test", rt)
	defer unregisterModbusRuntime("registry-test")

	got, ok := GetModbusRuntime("registry-test")
	if !ok {
		t.Fatal("GetModbusRuntime returned not found")
	}
	if got != rt {
		t.Error("GetModbusRuntime returned wrong runtime")
	}

	// Non-existent should return false
	_, ok = GetModbusRuntime("nonexistent")
	if ok {
		t.Error("GetModbusRuntime should return false for nonexistent")
	}
}

// --- NewModbusRuntime error case ---

func TestNewModbusRuntime_InvalidType(t *testing.T) {
	cfg := &DeviceConfig{
		Name: "bad-device",
		Type: "invalid",
	}
	_, err := NewModbusRuntime(cfg)
	if err == nil {
		t.Error("expected error for invalid connection type")
	}
}

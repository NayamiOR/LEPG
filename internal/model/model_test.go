package model

import (
	"testing"
)

// ── HashDevice ───────────────────────────────────────────────

func TestHashDevice_Deterministic(t *testing.T) {
	h1 := HashDevice("CLIENT001")
	h2 := HashDevice("CLIENT001")
	if h1 != h2 {
		t.Errorf("same input should produce same hash: %q vs %q", h1, h2)
	}
}

func TestHashDevice_DifferentInputs(t *testing.T) {
	h1 := HashDevice("CLIENT001")
	h2 := HashDevice("CLIENT002")
	if h1 == h2 {
		t.Errorf("different inputs should produce different hashes")
	}
}

func TestHashDevice_Length(t *testing.T) {
	h := HashDevice("test")
	if len(h) != 16 {
		t.Errorf("hash should be 16 hex chars, got %d: %q", len(h), h)
	}
}

// ── HashPoint ────────────────────────────────────────────────

func TestHashPoint_Deterministic(t *testing.T) {
	h1 := HashPoint("sensor-01", "temperature")
	h2 := HashPoint("sensor-01", "temperature")
	if h1 != h2 {
		t.Errorf("same input: %q vs %q", h1, h2)
	}
}

func TestHashPoint_DifferentDeviceName(t *testing.T) {
	h1 := HashPoint("sensor-A", "temp")
	h2 := HashPoint("sensor-B", "temp")
	if h1 == h2 {
		t.Error("different device names should produce different hashes")
	}
}

func TestHashPoint_DifferentPointName(t *testing.T) {
	h1 := HashPoint("sensor-01", "temperature")
	h2 := HashPoint("sensor-01", "humidity")
	if h1 == h2 {
		t.Error("different point names should produce different hashes")
	}
}

func TestHashPoint_Length(t *testing.T) {
	h := HashPoint("dev", "pt")
	if len(h) != 16 {
		t.Errorf("hash should be 16 hex chars, got %d: %q", len(h), h)
	}
}

// ── SerializeValue ───────────────────────────────────────────

func TestSerializeValue_Bool(t *testing.T) {
	if s := SerializeValue(DataTypeBool, true); s != "true" {
		t.Errorf("expected 'true', got %q", s)
	}
	if s := SerializeValue(DataTypeBool, false); s != "false" {
		t.Errorf("expected 'false', got %q", s)
	}
}

func TestSerializeValue_BoolWrongType(t *testing.T) {
	// 传入非 bool → 返回空字符串
	if s := SerializeValue(DataTypeBool, "not-bool"); s != "" {
		t.Errorf("expected '', got %q", s)
	}
}

func TestSerializeValue_Float(t *testing.T) {
	tests := []struct {
		dt DataType
		v  float64
	}{
		{DataTypeInt16, 42},
		{DataTypeUint16, 65535},
		{DataTypeInt32, -100},
		{DataTypeUint32, 4294967295},
		{DataTypeFloat32, 3.14},
		{DataTypeFloat64, 2.718281828},
	}
	for _, tc := range tests {
		s := SerializeValue(tc.dt, tc.v)
		if s == "" {
			t.Errorf("%s: expected non-empty string for %v", tc.dt, tc.v)
		}
	}
}

func TestSerializeValue_FloatWrongType(t *testing.T) {
	if s := SerializeValue(DataTypeFloat32, "not-float"); s != "" {
		t.Errorf("expected '', got %q", s)
	}
}

func TestSerializeValue_JSON(t *testing.T) {
	s := SerializeValue(DataTypeJSON, `{"key":"value"}`)
	if s != `{"key":"value"}` {
		t.Errorf("expected JSON string, got %q", s)
	}
}

func TestSerializeValue_JSONWrongType(t *testing.T) {
	if s := SerializeValue(DataTypeJSON, 123); s != "" {
		t.Errorf("expected '', got %q", s)
	}
}

func TestSerializeValue_UnknownType(t *testing.T) {
	if s := SerializeValue("unknown", "hello"); s != "" {
		t.Errorf("expected '' for unknown type, got %q", s)
	}
}

// ── ParseValue ───────────────────────────────────────────────

func TestParseValue_Float(t *testing.T) {
	f, isBool, err := ParseValue(DataTypeFloat32, "25.3")
	if err != nil {
		t.Fatal(err)
	}
	if isBool {
		t.Error("should not be bool")
	}
	if f != 25.3 {
		t.Errorf("expected 25.3, got %f", f)
	}
}

func TestParseValue_Int(t *testing.T) {
	f, isBool, err := ParseValue(DataTypeInt16, "42")
	if err != nil {
		t.Fatal(err)
	}
	if f != 42 {
		t.Errorf("expected 42, got %f", f)
	}
	if isBool {
		t.Error("should not be bool")
	}
}

func TestParseValue_BoolTrue(t *testing.T) {
	_, isBool, err := ParseValue(DataTypeBool, "true")
	if err != nil {
		t.Fatal(err)
	}
	if !isBool {
		t.Error("expected bool true")
	}
}

func TestParseValue_BoolFalse(t *testing.T) {
	_, isBool, err := ParseValue(DataTypeBool, "false")
	if err != nil {
		t.Fatal(err)
	}
	// ParseValue returns the parsed bool VALUE, not a type flag.
	// "false" → isBool=false (the value is false)
	if isBool {
		t.Error("expected isBool=false for value 'false'")
	}
}

func TestParseValue_InvalidFloat(t *testing.T) {
	_, _, err := ParseValue(DataTypeFloat32, "not-a-number")
	if err == nil {
		t.Error("expected error for invalid float")
	}
}

func TestParseValue_InvalidBool(t *testing.T) {
	_, _, err := ParseValue(DataTypeBool, "not-bool")
	if err == nil {
		t.Error("expected error for invalid bool")
	}
}

// ── ByteOrderConversion ──────────────────────────────────────

func TestByteOrderConversion_BigEndian_NoChange(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := ByteOrderConversion(input, ByteOrderBigEndian)
	for i := range input {
		if result[i] != input[i] {
			t.Errorf("ABCD should not change: result[%d]=0x%02X", i, result[i])
		}
	}
}

func TestByteOrderConversion_LittleEndian(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := ByteOrderConversion(input, ByteOrderLittleEndian)
	expected := []byte{0x04, 0x03, 0x02, 0x01}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("DCBA[%d]: got 0x%02X, want 0x%02X", i, result[i], expected[i])
		}
	}
}

func TestByteOrderConversion_MidLittleEndian(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := ByteOrderConversion(input, ByteOrderMidLittleEndian)
	expected := []byte{0x02, 0x01, 0x04, 0x03}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("BADC[%d]: got 0x%02X, want 0x%02X", i, result[i], expected[i])
		}
	}
}

func TestByteOrderConversion_MidBigEndian(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := ByteOrderConversion(input, ByteOrderMidBigEndian)
	expected := []byte{0x03, 0x04, 0x01, 0x02}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("CDAB[%d]: got 0x%02X, want 0x%02X", i, result[i], expected[i])
		}
	}
}

func TestByteOrderConversion_ShortData(t *testing.T) {
	// <4 bytes → 原样返回
	input := []byte{0xAA, 0xBB}
	result := ByteOrderConversion(input, ByteOrderLittleEndian)
	if len(result) != 2 || result[0] != 0xAA || result[1] != 0xBB {
		t.Errorf("short data should be unchanged: got %v", result)
	}
}

func TestByteOrderConversion_UnknownOrder(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03, 0x04}
	result := ByteOrderConversion(input, "xyz")
	for i := range input {
		if result[i] != input[i] {
			t.Errorf("unknown order should not change: result[%d]=0x%02X", i, result[i])
		}
	}
}

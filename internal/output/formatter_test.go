package output

import (
	"LEPG/internal/model"
	"encoding/json"
	"testing"
)

// ── ThingsBoardFormatter Tests ───────────────────────────────

func TestFormatter_Format_SingleReading(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{
			PointName: "temperature",
			DataType:  model.DataTypeFloat32,
			Value:     "25.3",
			Timestamp: 1719302400000,
		},
	}

	payload, err := f.Format("CLIENT001-温湿度传感器-01", readings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 JSON 结构
	var result map[string][]tbTelemetryEntry
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	entries, ok := result["CLIENT001-温湿度传感器-01"]
	if !ok {
		t.Fatalf("missing device key in payload: %s", string(payload))
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].TS != 1719302400000 {
		t.Errorf("expected ts 1719302400000, got %d", entries[0].TS)
	}

	temperature, ok := entries[0].Values["temperature"].(float64)
	if !ok {
		t.Fatalf("expected float64 temperature, got %T", entries[0].Values["temperature"])
	}
	if temperature != 25.3 {
		t.Errorf("expected temperature 25.3, got %f", temperature)
	}
}

func TestFormatter_Format_MultipleReadings(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "temperature", DataType: model.DataTypeFloat32, Value: "25.3", Timestamp: 1000},
		{PointName: "humidity", DataType: model.DataTypeFloat32, Value: "68", Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	if len(result["device-01"]) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result["device-01"]))
	}
}

func TestFormatter_Format_EmptyReadings(t *testing.T) {
	f := NewThingsBoardFormatter()
	_, err := f.Format("device-01", []model.Reading{})
	if err == nil {
		t.Error("expected error for empty readings")
	}
}

func TestFormatter_Format_BoolValue(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "switch", DataType: model.DataTypeBool, Value: "true", Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	val := result["device-01"][0].Values["switch"]
	if b, ok := val.(bool); !ok || !b {
		t.Errorf("expected bool true, got %T %v", val, val)
	}
}

func TestFormatter_Format_IntValue(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "count", DataType: model.DataTypeInt16, Value: "42", Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	val := result["device-01"][0].Values["count"]
	// int16 → int64 in JSON
	if v, ok := val.(float64); !ok || v != 42 {
		t.Errorf("expected float64 42, got %T %v", val, val)
	}
}

func TestFormatter_Format_JSONValue(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "metadata", DataType: model.DataTypeJSON, Value: `{"version":"1.0","tags":["a","b"]}`, Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	val := result["device-01"][0].Values["metadata"]
	if m, ok := val.(map[string]interface{}); !ok {
		t.Errorf("expected map for JSON value, got %T", val)
	} else if m["version"] != "1.0" {
		t.Errorf("expected version 1.0, got %v", m["version"])
	}
}

func TestFormatter_Format_InvalidJSONFallback(t *testing.T) {
	f := NewThingsBoardFormatter()
	// 不是合法 JSON → fallback 为原始 string
	readings := []model.Reading{
		{PointName: "data", DataType: model.DataTypeJSON, Value: `not-json`, Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	val := result["device-01"][0].Values["data"]
	if s, ok := val.(string); !ok || s != "not-json" {
		t.Errorf("expected string 'not-json' as fallback, got %T %v", val, val)
	}
}

func TestFormatter_Format_InvalidValue(t *testing.T) {
	f := NewThingsBoardFormatter()
	// "abc" 不是合法的 float
	readings := []model.Reading{
		{PointName: "bad", DataType: model.DataTypeFloat32, Value: "abc", Timestamp: 1000},
	}

	_, err := f.Format("device-01", readings)
	if err == nil {
		t.Error("expected error for invalid float value")
	}
}

func TestFormatter_Format_UnknownDataType(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "raw", DataType: "custom_type", Value: "hello", Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	val := result["device-01"][0].Values["raw"]
	if s, ok := val.(string); !ok || s != "hello" {
		t.Errorf("expected string fallback, got %T %v", val, val)
	}
}

func TestFormatter_Format_EmptyPointName_UsesPoint(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "", Point: "deadbeefdeadbeef", DataType: model.DataTypeFloat32, Value: "99", Timestamp: 1000},
	}

	payload, err := f.Format("device-01", readings)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	entry := result["device-01"][0]
	if _, ok := entry.Values["deadbeefdeadbeef"]; !ok {
		t.Errorf("expected hash fallback key, got values: %v", entry.Values)
	}
}

func TestFormatter_Format_PreservesTimestampPerReading(t *testing.T) {
	f := NewThingsBoardFormatter()
	readings := []model.Reading{
		{PointName: "t1", DataType: model.DataTypeFloat32, Value: "10", Timestamp: 1000},
		{PointName: "t2", DataType: model.DataTypeFloat32, Value: "20", Timestamp: 2000},
	}

	payload, _ := f.Format("d", readings)

	var result map[string][]tbTelemetryEntry
	json.Unmarshal(payload, &result)

	if result["d"][0].TS != 1000 {
		t.Errorf("first entry ts expected 1000, got %d", result["d"][0].TS)
	}
	if result["d"][1].TS != 2000 {
		t.Errorf("second entry ts expected 2000, got %d", result["d"][1].TS)
	}
}

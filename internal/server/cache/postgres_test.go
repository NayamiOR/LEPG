package cache

import (
	"LEPG/internal/model"
	"context"
	"os"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("LEPG_TEST_PG")
	if dsn == "" {
		t.Skip("LEPG_TEST_PG not set, skipping PostgreSQL integration test")
	}
	store, err := NewPostgresStore(context.Background(), dsn)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		store.db.ExecContext(ctx, "DELETE FROM readings")
		store.db.ExecContext(ctx, "DELETE FROM devices")
		store.Close()
	})
	return store
}

func testReading(deviceName, pointName, value string) *model.Reading {
	return &model.Reading{
		Device:     model.HashDevice(deviceName),
		DeviceName: deviceName,
		Point:      model.HashPoint(deviceName, pointName),
		PointName:  pointName,
		DataType:   model.DataTypeFloat32,
		Value:      value,
		Quality:    model.QualityGood,
		Unit:       "V",
		Timestamp:  time.Now().UnixMilli(),
	}
}

func TestSaveReadings_CreatesDevice(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	readings := []*model.Reading{
		testReading("temp-sensor-1", "temperature", "25.5"),
	}
	err := store.SaveReadings(ctx, "GW001", readings)
	if err != nil {
		t.Fatalf("SaveReadings: %v", err)
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d := devices[0]
	if d.Sn != "GW001" {
		t.Errorf("Sn = %s, want GW001", d.Sn)
	}
	if d.DeviceHash != model.HashDevice("temp-sensor-1") {
		t.Errorf("DeviceHash = %s, want %s", d.DeviceHash, model.HashDevice("temp-sensor-1"))
	}
	if d.DeviceName != "temp-sensor-1" {
		t.Errorf("DeviceName = %s, want temp-sensor-1", d.DeviceName)
	}
	if d.FirstSeen != d.LastSeen {
		t.Errorf("FirstSeen (%d) != LastSeen (%d) on first insert", d.FirstSeen, d.LastSeen)
	}
	if d.Status != "unknown" {
		t.Errorf("Status = %s, want unknown", d.Status)
	}
}

func TestSaveReadings_UpdatesLastSeenOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	sensor1 := testReading("temp-sensor-1", "temperature", "25.5")
	err := store.SaveReadings(ctx, "GW001", []*model.Reading{sensor1})
	if err != nil {
		t.Fatalf("first SaveReadings: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	sensor1.Timestamp = time.Now().UnixMilli()
	err = store.SaveReadings(ctx, "GW001", []*model.Reading{sensor1})
	if err != nil {
		t.Fatalf("second SaveReadings: %v", err)
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d := devices[0]
	if d.FirstSeen >= d.LastSeen {
		t.Errorf("FirstSeen (%d) should be < LastSeen (%d) after second upload", d.FirstSeen, d.LastSeen)
	}
}

func TestSaveReadings_DedupWithinBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// 3 readings from same device, different points
	readings := []*model.Reading{
		testReading("temp-sensor-1", "temperature", "25.5"),
		testReading("temp-sensor-1", "humidity", "60.0"),
		testReading("temp-sensor-1", "pressure", "1013.2"),
	}
	err := store.SaveReadings(ctx, "GW001", readings)
	if err != nil {
		t.Fatalf("SaveReadings: %v", err)
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device from same-device batch, got %d", len(devices))
	}
}

func TestSaveReadings_MultipleDevices(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	readings := []*model.Reading{
		testReading("temp-sensor-1", "temperature", "25.5"),
		testReading("flow-meter-2", "flow_rate", "3.2"),
		testReading("pressure-3", "pressure", "1013.2"),
	}
	err := store.SaveReadings(ctx, "GW001", readings)
	if err != nil {
		t.Fatalf("SaveReadings: %v", err)
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
}

func TestSaveReadings_DevicesIsolatedByGateway(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.SaveReadings(ctx, "GW001", []*model.Reading{
		testReading("sensor-A", "temp", "20.0"),
	})
	if err != nil {
		t.Fatalf("SaveReadings GW001: %v", err)
	}
	err = store.SaveReadings(ctx, "GW002", []*model.Reading{
		testReading("sensor-A", "temp", "21.0"),
	})
	if err != nil {
		t.Fatalf("SaveReadings GW002: %v", err)
	}

	d1, _ := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	d2, _ := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW002"})
	all, _ := store.QueryDevices(ctx, DeviceQueryFilter{})

	if len(d1) != 1 || len(d2) != 1 {
		t.Fatalf("GW001: %d devices, GW002: %d devices", len(d1), len(d2))
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 total devices across all gateways, got %d", len(all))
	}
}

func TestSaveReadings_EmptyBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.SaveReadings(ctx, "GW001", nil)
	if err != nil {
		t.Fatalf("SaveReadings nil: %v", err)
	}
	err = store.SaveReadings(ctx, "GW001", []*model.Reading{})
	if err != nil {
		t.Fatalf("SaveReadings empty: %v", err)
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices from empty batch, got %d", len(devices))
	}
}

func TestQueryDevices_DefaultLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		name := "sensor-" + string(rune('0'+i%10))
		err := store.SaveReadings(ctx, "GW001", []*model.Reading{
			testReading(name, "temp", "20.0"),
		})
		if err != nil {
			t.Fatalf("SaveReadings: %v", err)
		}
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 10 {
		t.Fatalf("expected 10 devices, got %d", len(devices))
	}
}

func TestQueryDevices_CustomLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		name := "sensor-" + string(rune('0'+i))
		err := store.SaveReadings(ctx, "GW001", []*model.Reading{
			testReading(name, "temp", "20.0"),
		})
		if err != nil {
			t.Fatalf("SaveReadings: %v", err)
		}
	}

	devices, err := store.QueryDevices(ctx, DeviceQueryFilter{Sn: "GW001", Limit: 3})
	if err != nil {
		t.Fatalf("QueryDevices: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices with custom limit, got %d", len(devices))
	}
}

func TestSaveReadings_ReadingsStillSaved(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	readings := []*model.Reading{
		testReading("sensor-1", "temp", "25.5"),
		testReading("sensor-1", "humidity", "60.0"),
	}
	err := store.SaveReadings(ctx, "GW001", readings)
	if err != nil {
		t.Fatalf("SaveReadings: %v", err)
	}

	stored, err := store.QueryReadings(ctx, QueryFilter{Sn: "GW001"})
	if err != nil {
		t.Fatalf("QueryReadings: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 readings, got %d", len(stored))
	}
}

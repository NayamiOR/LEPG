package server

import (
	"LEPG/internal/model"
	"LEPG/internal/msg"
	"LEPG/internal/output"
	"LEPG/internal/server/cache"
	"context"
	"sync"
	"testing"
)

// ── Mock Store ───────────────────────────────────────────────

type mockStore struct {
	mu            sync.Mutex
	savedReadings map[string][]*model.Reading // sn → readings
	saveErr       error
}

func newMockStore() *mockStore {
	return &mockStore{savedReadings: make(map[string][]*model.Reading)}
}

func (m *mockStore) SaveReadings(ctx context.Context, sn string, readings []*model.Reading) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	m.savedReadings[sn] = append(m.savedReadings[sn], readings...)
	return nil
}

func (m *mockStore) QueryReadings(ctx context.Context, filter cache.QueryFilter) ([]*cache.StoredReading, error) {
	return nil, nil
}

func (m *mockStore) QueryDevices(ctx context.Context, filter cache.DeviceQueryFilter) ([]*cache.StoredDevice, error) {
	return nil, nil
}

func (m *mockStore) Close() error { return nil }

// ── Mock Publisher ───────────────────────────────────────────

type mockPublisher struct {
	mu           sync.Mutex
	publishCalls []publishCall
}

type publishCall struct {
	SN      string
	Payload []byte
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{}
}

func (m *mockPublisher) PublishDeviceReadings(sn string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishCalls = append(m.publishCalls, publishCall{SN: sn, Payload: payload})
	return nil
}

// ── Mock Sinker ──────────────────────────────────────────────

type mockSinker struct {
	name       string
	mu         sync.Mutex
	sendCalls  []sinkCall
	sendErr    error
	closed     bool
}

type sinkCall struct {
	DeviceKey string
	Readings  []model.Reading
}

func newMockSinker(name string) *mockSinker {
	return &mockSinker{name: name}
}

func (m *mockSinker) Name() string { return m.name }

func (m *mockSinker) Send(deviceKey string, readings []model.Reading) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	// deep copy readings to avoid data race
	copied := make([]model.Reading, len(readings))
	copy(copied, readings)
	m.sendCalls = append(m.sendCalls, sinkCall{DeviceKey: deviceKey, Readings: copied})
	return nil
}

func (m *mockSinker) HealthCheck() error { return nil }
func (m *mockSinker) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockSinker) getSendCalls() []sinkCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sinkCall, len(m.sendCalls))
	copy(result, m.sendCalls)
	return result
}

// ── Helpers ──────────────────────────────────────────────────

func makeUploadMsg(readings []model.Reading) *msg.Msg {
	factory := msg.NewMsgFactory()
	payload := &msg.UploadPayload{Readings: readings}
	m, _ := factory.NewMsg(msg.MsgTypeUpload, payload)
	return m
}

// ── Tests ────────────────────────────────────────────────────

// TestHandleUpload_SavesAndRoutes verifies the full upload pipeline:
//
//	MsgTypeUpload → ParseMsg → SaveReadings → groupByDeviceName → OutputRouter.Send
func TestHandleUpload_SavesAndRoutes(t *testing.T) {
	store := newMockStore()
	pub := newMockPublisher()
	sink := newMockSinker("test-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})

	readings := []model.Reading{
		{
			Device:     "aaaaaaaaaaaaaaaa",
			DeviceName: "温湿度传感器-01",
			Point:      "bbbbbbbbbbbbbbbb",
			PointName:  "temperature",
			DataType:   model.DataTypeFloat32,
			Value:      "25.3",
			Quality:    model.QualityGood,
			Timestamp:  1719302400000,
		},
		{
			Device:     "aaaaaaaaaaaaaaaa",
			DeviceName: "温湿度传感器-01",
			Point:      "cccccccccccccccc",
			PointName:  "humidity",
			DataType:   model.DataTypeFloat32,
			Value:      "68",
			Quality:    model.QualityGood,
			Timestamp:  1719302400000,
		},
	}

	uploadMsg := makeUploadMsg(readings)
	const testSN = "CLIENT001"

	handleUpload(store, pub, router, uploadMsg, testSN, "127.0.0.1:12345")
	router.Shutdown() // 等待 fan-out goroutines 完成

	// 验证 Store 收到数据
	store.mu.Lock()
	saved := store.savedReadings[testSN]
	store.mu.Unlock()

	if len(saved) != 2 {
		t.Fatalf("expected 2 saved readings, got %d", len(saved))
	}
	if saved[0].DeviceName != "温湿度传感器-01" {
		t.Errorf("expected DeviceName 温湿度传感器-01, got %s", saved[0].DeviceName)
	}

	// 验证 Sinker 收到 fan-out 调用
	calls := sink.getSendCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 sinker call (one device group), got %d", len(calls))
	}
	expectedKey := "CLIENT001-温湿度传感器-01"
	if calls[0].DeviceKey != expectedKey {
		t.Errorf("expected deviceKey %q, got %q", expectedKey, calls[0].DeviceKey)
	}
	if len(calls[0].Readings) != 2 {
		t.Errorf("expected 2 readings in sink call, got %d", len(calls[0].Readings))
	}
}

// TestHandleUpload_MultipleDevices verifies fan-out groups readings by DeviceName
// and calls router.Send once per device group.
func TestHandleUpload_MultipleDevices(t *testing.T) {
	store := newMockStore()
	sink := newMockSinker("multi-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})

	readings := []model.Reading{
		{DeviceName: "温湿度传感器-01", PointName: "temperature", DataType: model.DataTypeFloat32, Value: "25", Timestamp: 1000},
		{DeviceName: "温湿度传感器-01", PointName: "humidity", DataType: model.DataTypeFloat32, Value: "60", Timestamp: 1000},
		{DeviceName: "电表-02", PointName: "power", DataType: model.DataTypeFloat32, Value: "1500", Timestamp: 1000},
	}

	uploadMsg := makeUploadMsg(readings)

	handleUpload(store, newMockPublisher(), router, uploadMsg, "CLIENT001", "127.0.0.1:12345")
	router.Shutdown() // 等待 fan-out goroutines 完成

	calls := sink.getSendCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 sinker calls (two device groups), got %d", len(calls))
	}

	// 收集发送的 deviceKey
	keys := make(map[string]int)
	for _, c := range calls {
		keys[c.DeviceKey] = len(c.Readings)
	}

	if keys["CLIENT001-温湿度传感器-01"] != 2 {
		t.Errorf("expected 2 readings for 温湿度传感器-01, got %d", keys["CLIENT001-温湿度传感器-01"])
	}
	if keys["CLIENT001-电表-02"] != 1 {
		t.Errorf("expected 1 reading for 电表-02, got %d", keys["CLIENT001-电表-02"])
	}
}

// TestHandleUpload_EmptyReadings verifies that an empty upload payload does not
// crash and does not call router.Send.
func TestHandleUpload_EmptyReadings(t *testing.T) {
	store := newMockStore()
	sink := newMockSinker("empty-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})

	uploadMsg := makeUploadMsg([]model.Reading{})

	// 空 readings 不应 panic
	handleUpload(store, newMockPublisher(), router, uploadMsg, "CLIENT001", "127.0.0.1:12345")

	calls := sink.getSendCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 sinker calls for empty readings, got %d", len(calls))
	}
}

// TestHandleUpload_StoreError verifies that when SaveReadings fails,
// router.Send is NOT called (data should not be pushed if it wasn't persisted).
func TestHandleUpload_StoreError(t *testing.T) {
	store := newMockStore()
	store.saveErr = context.DeadlineExceeded // 模拟存储失败
	sink := newMockSinker("err-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})

	readings := []model.Reading{
		{DeviceName: "sensor-01", PointName: "temp", DataType: model.DataTypeFloat32, Value: "25", Timestamp: 1000},
	}
	uploadMsg := makeUploadMsg(readings)

	handleUpload(store, newMockPublisher(), router, uploadMsg, "CLIENT001", "127.0.0.1:12345")

	calls := sink.getSendCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 sinker calls when store fails, got %d", len(calls))
	}
}

// TestHandleUpload_InvalidPayload verifies that corrupt payloads are logged
// but do not panic.
func TestHandleUpload_InvalidPayload(t *testing.T) {
	store := newMockStore()
	sink := newMockSinker("bad-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})

	// 构造一个类型标记为 Upload 但 payload 是垃圾的消息
	badMsg := &msg.Msg{
		Type:    msg.MsgTypeUpload,
		Payload: []byte{0xFF, 0xFF, 0xFF}, // gob 无法解析
	}

	// 不应 panic
	handleUpload(store, newMockPublisher(), router, badMsg, "CLIENT001", "127.0.0.1:12345")

	calls := sink.getSendCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 sinker calls for invalid payload, got %d", len(calls))
	}
}

// TestHandleUpload_NilRouter verifies that a nil router does not panic
// (defensive check for when output module is disabled).
func TestHandleUpload_NilRouter(t *testing.T) {
	store := newMockStore()
	readings := []model.Reading{
		{DeviceName: "sensor-01", PointName: "temp", DataType: model.DataTypeFloat32, Value: "25", Timestamp: 1000},
	}
	uploadMsg := makeUploadMsg(readings)

	// router = nil 不应 panic
	handleUpload(store, newMockPublisher(), nil, uploadMsg, "CLIENT001", "127.0.0.1:12345")

	store.mu.Lock()
	saved := store.savedReadings["CLIENT001"]
	store.mu.Unlock()

	if len(saved) != 1 {
		t.Fatalf("expected 1 saved reading even with nil router, got %d", len(saved))
	}
}

// TestGroupReadingsByDeviceName tests the grouping helper directly.
func TestGroupReadingsByDeviceName(t *testing.T) {
	readings := []*model.Reading{
		{DeviceName: "sensor-A", PointName: "temp", Value: "25"},
		{DeviceName: "sensor-A", PointName: "humidity", Value: "60"},
		{DeviceName: "sensor-B", PointName: "temp", Value: "30"},
		{DeviceName: "", PointName: "orphan", Value: "99"}, // 无名设备 → __unknown__
	}

	grouped := groupReadingsByDeviceName(readings)

	if len(grouped) != 3 {
		t.Fatalf("expected 3 groups (sensor-A, sensor-B, __unknown__), got %d", len(grouped))
	}
	if len(grouped["sensor-A"]) != 2 {
		t.Errorf("expected 2 readings in sensor-A, got %d", len(grouped["sensor-A"]))
	}
	if len(grouped["sensor-B"]) != 1 {
		t.Errorf("expected 1 reading in sensor-B, got %d", len(grouped["sensor-B"]))
	}
	if len(grouped["__unknown__"]) != 1 {
		t.Errorf("expected 1 reading in __unknown__, got %d", len(grouped["__unknown__"]))
	}
}

// TestOutputRouterShutdown_ClosesAllSinks verifies that Shutdown waits for
// in-flight sends and then closes all sinks.
func TestOutputRouterShutdown_ClosesAllSinks(t *testing.T) {
	s1 := newMockSinker("sink-1")
	s2 := newMockSinker("sink-2")
	router := output.NewOutputRouter([]output.Sinker{s1, s2})

	readings := []model.Reading{
		{DeviceName: "sensor-01", PointName: "temp", DataType: model.DataTypeFloat32, Value: "25", Timestamp: 1000},
	}

	router.Send("CLIENT001-sensor-01", readings)
	router.Shutdown()

	if !s1.closed {
		t.Error("sink-1 should be closed after Shutdown")
	}
	if !s2.closed {
		t.Error("sink-2 should be closed after Shutdown")
	}

	// 确认 fan-out 生效
	if len(s1.getSendCalls()) != 1 {
		t.Errorf("sink-1 expected 1 send call, got %d", len(s1.getSendCalls()))
	}
	if len(s2.getSendCalls()) != 1 {
		t.Errorf("sink-2 expected 1 send call, got %d", len(s2.getSendCalls()))
	}
}

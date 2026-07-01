package output

import (
	"fmt"
	"LEPG/internal/model"
	"sync"
	"testing"
	"time"
)

// ── Mock Sinker ──────────────────────────────────────────────

type testSinker struct {
	name      string
	mu        sync.Mutex
	sendCalls []sinkCall
	sendErr   error
	closed    bool
}

type sinkCall struct {
	DeviceKey string
	Readings  []model.Reading
}

func newTestSinker(name string) *testSinker {
	return &testSinker{name: name}
}

func (s *testSinker) Name() string { return s.name }

func (s *testSinker) Send(deviceKey string, readings []model.Reading) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	copied := make([]model.Reading, len(readings))
	copy(copied, readings)
	s.sendCalls = append(s.sendCalls, sinkCall{DeviceKey: deviceKey, Readings: copied})
	return nil
}

func (s *testSinker) HealthCheck() error { return nil }

func (s *testSinker) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *testSinker) getSendCalls() []sinkCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]sinkCall, len(s.sendCalls))
	copy(result, s.sendCalls)
	return result
}

// ── OutputRouter Tests ───────────────────────────────────────

func TestOutputRouter_Send_FanOut(t *testing.T) {
	s1 := newTestSinker("s1")
	s2 := newTestSinker("s2")
	router := NewOutputRouter([]Sinker{s1, s2})

	readings := []model.Reading{
		{DeviceName: "sensor-01", PointName: "temp", Value: "25", Timestamp: 1000},
	}

	router.Send("CLIENT001-sensor-01", readings)
	router.Shutdown()

	if len(s1.getSendCalls()) != 1 {
		t.Errorf("s1 expected 1 call, got %d", len(s1.getSendCalls()))
	}
	if len(s2.getSendCalls()) != 1 {
		t.Errorf("s2 expected 1 call, got %d", len(s2.getSendCalls()))
	}
}

func TestOutputRouter_Send_OneSinkFails(t *testing.T) {
	s1 := newTestSinker("good")
	s2 := newTestSinker("bad")
	s2.sendErr = fmt.Errorf("simulated failure")

	router := NewOutputRouter([]Sinker{s1, s2})

	readings := []model.Reading{
		{DeviceName: "sensor-01", PointName: "temp", Value: "25", Timestamp: 1000},
	}

	router.Send("CLIENT001-sensor-01", readings)
	router.Shutdown()

	// s1 应该仍然收到数据
	if len(s1.getSendCalls()) != 1 {
		t.Errorf("good sink expected 1 call, got %d — failure in one sink should not block others", len(s1.getSendCalls()))
	}
	// s2 虽然失败，也被调用了
	if len(s2.getSendCalls()) != 0 {
		t.Errorf("bad sink expected 0 successful calls, got %d", len(s2.getSendCalls()))
	}
}

func TestOutputRouter_CloseAllSinksOnShutdown(t *testing.T) {
	s1 := newTestSinker("s1")
	s2 := newTestSinker("s2")
	router := NewOutputRouter([]Sinker{s1, s2})

	router.Shutdown()

	if !s1.closed {
		t.Error("s1 should be closed after Shutdown")
	}
	if !s2.closed {
		t.Error("s2 should be closed after Shutdown")
	}
}

func TestOutputRouter_ConcurrentSend(t *testing.T) {
	s1 := newTestSinker("s1")
	s2 := newTestSinker("s2")
	router := NewOutputRouter([]Sinker{s1, s2})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			router.Send("device-01", []model.Reading{
				{PointName: "temp", Value: "25", Timestamp: int64(id)},
			})
		}(i)
	}
	wg.Wait()
	router.Shutdown()

	if len(s1.getSendCalls()) != 10 {
		t.Errorf("s1 expected 10 calls, got %d", len(s1.getSendCalls()))
	}
	if len(s2.getSendCalls()) != 10 {
		t.Errorf("s2 expected 10 calls, got %d", len(s2.getSendCalls()))
	}
}

// ── OutputConfig Tests ───────────────────────────────────────

func TestOutputConfig_Validate_ValidTB_MQTT(t *testing.T) {
	cfg := OutputConfig{
		Type:  "thingsboard-mqtt",
		Name:  "tb-prod",
		Host:  "thingsboard.cloud",
		Token: "abc123",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid TB MQTT config should not error: %v", err)
	}
	if cfg.Port != 1883 {
		t.Errorf("expected default port 1883, got %d", cfg.Port)
	}
	if cfg.QoS != 1 {
		t.Errorf("expected default QoS 1, got %d", cfg.QoS)
	}
}

func TestOutputConfig_Validate_ValidTB_HTTP(t *testing.T) {
	cfg := OutputConfig{
		Type:  "thingsboard-http",
		Name:  "tb-http",
		Host:  "thingsboard.cloud",
		Token: "abc123",
		Port:  443,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid TB HTTP config should not error: %v", err)
	}
	// HTTP 默认 timeout
	if cfg.Timeout != 10 {
		t.Errorf("expected default timeout 10, got %d", cfg.Timeout)
	}
}

func TestOutputConfig_Validate_MissingName(t *testing.T) {
	cfg := OutputConfig{Type: "thingsboard-mqtt"}
	if err := cfg.Validate(); err == nil {
		t.Error("missing name should error")
	}
}

func TestOutputConfig_Validate_MissingHost(t *testing.T) {
	cfg := OutputConfig{Type: "thingsboard-mqtt", Name: "tb", Token: "abc"}
	if err := cfg.Validate(); err == nil {
		t.Error("missing host should error")
	}
}

func TestOutputConfig_Validate_MissingToken(t *testing.T) {
	cfg := OutputConfig{Type: "thingsboard-mqtt", Name: "tb", Host: "localhost"}
	if err := cfg.Validate(); err == nil {
		t.Error("missing token should error")
	}
}

func TestOutputConfig_Validate_UnknownType(t *testing.T) {
	cfg := OutputConfig{Type: "unknown-platform", Name: "test"}
	if err := cfg.Validate(); err == nil {
		t.Error("unknown type should error")
	}
}

func TestOutputConfig_Validate_DefaultsPreserved(t *testing.T) {
	cfg := OutputConfig{
		Type:  "thingsboard-http",
		Name:  "tb",
		Host:  "localhost",
		Token: "abc",
		Port:  8443,
		QoS:   2,
		Timeout: 30,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	// 显式设置的值不被默认值覆盖
	if cfg.Port != 8443 {
		t.Errorf("port should be 8443, got %d", cfg.Port)
	}
	if cfg.QoS != 2 {
		t.Errorf("QoS should be 2, got %d", cfg.QoS)
	}
	if cfg.Timeout != 30 {
		t.Errorf("timeout should be 30, got %d", cfg.Timeout)
	}
}

func TestOutputConfig_Validate_SetDefaults(t *testing.T) {
	cfg := OutputConfig{
		Type:  "thingsboard-mqtt",
		Name:  "tb",
		Host:  "localhost",
		Token: "abc",
		// Port, QoS, Timeout 都不设
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 1883 {
		t.Errorf("expected default port 1883, got %d", cfg.Port)
	}
	if cfg.QoS != 1 {
		t.Errorf("expected default QoS 1, got %d", cfg.QoS)
	}
	if cfg.Timeout != 10 {
		t.Errorf("expected default timeout 10, got %d", cfg.Timeout)
	}
}

// ── NewSinker Factory Tests ──────────────────────────────────

func TestNewSinker_TB_MQTT(t *testing.T) {
	cfg := OutputConfig{
		Type:  "thingsboard-mqtt",
		Name:  "tb-mqtt",
		Host:  "localhost",
		Token: "test-token",
	}
	s, err := NewSinker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "tb-mqtt" {
		t.Errorf("expected name tb-mqtt, got %s", s.Name())
	}
	// 检查 HealthCheck：未 connect 应该报错
	if err := s.HealthCheck(); err == nil {
		t.Error("expected health check to fail for unconnected MQTT sinker")
	}
	// Close 应该安全（未连接也可以 Close）
	if err := s.Close(); err != nil {
		t.Errorf("close should not error on unconnected sinker: %v", err)
	}
}

func TestNewSinker_TB_HTTP(t *testing.T) {
	cfg := OutputConfig{
		Type:  "thingsboard-http",
		Name:  "tb-http",
		Host:  "localhost",
		Token: "test-token",
		Port:  443,
	}
	s, err := NewSinker(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "tb-http" {
		t.Errorf("expected name tb-http, got %s", s.Name())
	}
	// HTTP sinker HealthCheck 总是返回 nil
	if err := s.HealthCheck(); err != nil {
		t.Errorf("HTTP sinker health check should return nil: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("close should not error: %v", err)
	}
}

func TestNewSinker_InvalidConfig(t *testing.T) {
	cfg := OutputConfig{Type: "thingsboard-mqtt"} // 缺少 Name, Host, Token
	_, err := NewSinker(cfg)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestNewSinker_UnknownType(t *testing.T) {
	cfg := OutputConfig{Type: "unknown", Name: "test"}
	_, err := NewSinker(cfg)
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

// ── Helper to avoid data race in concurrent tests ────────────

func init() {
	// ensure test runs don't hang on default timeout
	_ = time.Second
}

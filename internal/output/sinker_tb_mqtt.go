package output

import (
	"LEPG/internal/model"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	// tbGatewayTelemetryTopic is the fixed ThingsBoard Gateway
	// telemetry topic. LEPG always uses the Gateway API, so
	// there is no need to expose this to user configuration.
	tbGatewayTelemetryTopic = "v1/gateway/telemetry"

	// defaultMQTTKeepAlive is the paho keep-alive interval.
	defaultMQTTKeepAlive = 30 * time.Second

	// defaultMQTTConnectTimeout is the paho connect timeout.
	defaultMQTTConnectTimeout = 10 * time.Second

	// defaultMQTTMaxReconnectInterval caps exponential back-off
	// during automatic reconnection.
	defaultMQTTMaxReconnectInterval = 30 * time.Second
)

// ── ThingsBoard MQTT Sinker ─────────────────────────────────

// tbMqttSinker publishes ThingsBoard Gateway telemetry via MQTT.
// It maintains a single long-lived paho connection that is
// automatically reconnected on failure.
type tbMqttSinker struct {
	cfg       OutputConfig
	formatter Formatter
	client    mqtt.Client
	mu        sync.Mutex
}

// NewThingsBoardMqttSinker creates a ThingsBoard MQTT sinker.
// The paho client is NOT connected until the first Send call
// (lazy connect), which keeps startup fast and avoids blocking
// on unreachable brokers during initialisation.
func NewThingsBoardMqttSinker(cfg OutputConfig) (*tbMqttSinker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &tbMqttSinker{
		cfg:       cfg,
		formatter: NewThingsBoardFormatter(),
	}, nil
}

func (s *tbMqttSinker) Name() string { return s.cfg.Name }

func (s *tbMqttSinker) Send(deviceKey string, readings []model.Reading) error {
	// Ensure the connection is alive (lazy connect on first call,
	// paho auto-reconnect handles later failures).
	s.mu.Lock()
	if s.client == nil || !s.client.IsConnected() {
		if err := s.connect(); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("tb mqtt connect: %w", err)
		}
	}
	s.mu.Unlock()

	payload, err := s.formatter.Format(deviceKey, readings)
	if err != nil {
		return fmt.Errorf("tb format: %w", err)
	}

	token := s.client.Publish(tbGatewayTelemetryTopic, byte(s.cfg.QoS), false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("tb mqtt publish: %w", token.Error())
	}

	slog.Debug("tb mqtt published",
		"sink", s.cfg.Name,
		"device_key", deviceKey,
		"count", len(readings),
	)
	return nil
}

func (s *tbMqttSinker) HealthCheck() error {
	if s.client == nil || !s.client.IsConnected() {
		return fmt.Errorf("tb mqtt sinker %q: not connected", s.cfg.Name)
	}
	return nil
}

func (s *tbMqttSinker) Close() error {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
	}
	return nil
}

// connect establishes the paho MQTT connection. Caller must hold s.mu.
func (s *tbMqttSinker) connect() error {
	broker := fmt.Sprintf("tcp://%s:%d", s.cfg.Host, s.cfg.Port)

	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(s.cfg.Name). // unique per sink; TB doesn't enforce a format
		SetUsername(s.cfg.Token). // TB auth: username = access token
		SetKeepAlive(defaultMQTTKeepAlive).
		SetConnectTimeout(defaultMQTTConnectTimeout).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(defaultMQTTMaxReconnectInterval).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			slog.Warn("tb mqtt connection lost",
				"sink", s.cfg.Name,
				"broker", broker,
				"error", err,
			)
		})

	s.client = mqtt.NewClient(opts)
	token := s.client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("connect to %s: %w", broker, token.Error())
	}

	slog.Info("tb mqtt connected",
		"sink", s.cfg.Name,
		"broker", broker,
	)
	return nil
}

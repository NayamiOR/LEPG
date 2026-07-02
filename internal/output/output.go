// Package output provides northbound data output for LEPG server.
//
// Two output modes:
//   - Pull (BrokerPublisher): external consumers subscribe to LEPG's
//     embedded MQTT broker (comqtt). Defined in internal/server/publisher.go.
//   - Push (Sinker/OutputRouter): LEPG actively pushes data to external
//     IoT platforms (ThingsBoard, EdgeX, etc.) via MQTT or HTTP.
//
// Architecture:
//
//	HandleConnection
//	    │
//	    ├── group by DeviceName
//	    ├── construct deviceKey = SN + "-" + DeviceName
//	    │
//	    └── OutputRouter.Send(deviceKey, readings)
//	          │
//	          ├── Sinker A.Send(deviceKey, readings)
//	          │       ├── Formatter.Format(deviceKey, readings) → []byte
//	          │       └── transport.Deliver(payload)
//	          │
//	          └── Sinker B.Send(...)
package output

import (
	"LEPG/internal/model"
	"fmt"
	"log/slog"
	"sync"
)

// ── Interfaces ──────────────────────────────────────────────

// Formatter serializes device readings into a platform-specific payload.
type Formatter interface {
	Format(deviceKey string, readings []model.Reading) ([]byte, error)
}

// Sinker represents a single push output target (MQTT broker or HTTP endpoint).
// Each Sinker holds its own Formatter, which is created internally at
// construction time and not exposed to callers.
type Sinker interface {
	Name() string
	Send(deviceKey string, readings []model.Reading) error
	HealthCheck() error
	Close() error
}

// ── OutputRouter ────────────────────────────────────────────

// OutputRouter fans out readings to all registered Sinkers.
// Each Send call runs in an independent goroutine; a single sink
// failure does not affect others.
type OutputRouter struct {
	sinks []Sinker
	wg    sync.WaitGroup
}

// NewOutputRouter creates a router that fans out to the given sinks.
func NewOutputRouter(sinks []Sinker) *OutputRouter {
	return &OutputRouter{sinks: sinks}
}

// Send fans out readings to all sinks concurrently. Fire-and-forget:
// individual sink errors are logged but never returned.
func (r *OutputRouter) Send(deviceKey string, readings []model.Reading) {
	for _, sink := range r.sinks {
		r.wg.Add(1)
		go func(s Sinker) {
			defer r.wg.Done()
			if err := s.Send(deviceKey, readings); err != nil {
				slog.Error("sink send failed",
					"sink", s.Name(),
					"device_key", deviceKey,
					"count", len(readings),
					"error", err)
			}
		}(sink)
	}
}

// Shutdown waits for in-flight sends to complete, then closes all sinks.
func (r *OutputRouter) Shutdown() {
	r.wg.Wait()
	for _, sink := range r.sinks {
		if err := sink.Close(); err != nil {
			slog.Warn("sink close failed", "name", sink.Name(), "error", err)
		}
	}
}

// ── OutputConfig ────────────────────────────────────────────

// SinkType enumerates supported platform+transport combinations.
type SinkType string

const (
	SinkThingsBoardMQTT SinkType = "thingsboard-mqtt"
	SinkThingsBoardHTTP SinkType = "thingsboard-http"
	// Future:
	// SinkEdgeXMQTT        SinkType = "edgex-mqtt"
	// SinkEdgeXHTTP        SinkType = "edgex-http"
	// SinkAliyunMQTT       SinkType = "aliyun-mqtt"
	// SinkGenericMQTT      SinkType = "generic-mqtt"
	// SinkGenericHTTP      SinkType = "generic-http"
)

// OutputConfig defines a single output sink configuration.
// Matches the TOML [[outputs]] block. Uses a flat struct (union of
// all platform fields) — each platform implementation picks the
// fields it needs. Authenticator abstraction will be extracted
// once multiple auth models are implemented.
type OutputConfig struct {
	Type    string `mapstructure:"type"`    // "thingsboard-mqtt", etc.
	Name    string `mapstructure:"name"`    // log identifier
	Enabled bool   `mapstructure:"enabled"` // on/off switch

	// Connection
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	QoS  int    `mapstructure:"qos"` // MQTT QoS, default 1

	// Authentication — platform-specific; use what you need
	Token        string `mapstructure:"token"`
	ProductKey   string `mapstructure:"product_key"`
	DeviceName   string `mapstructure:"device_name"`
	DeviceSecret string `mapstructure:"device_secret"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`

	// HTTP-specific
	Scheme  string            `mapstructure:"scheme"` // "http" or "https", default "https"
	URL     string            `mapstructure:"url"`
	Headers map[string]string `mapstructure:"headers"`
	Timeout int               `mapstructure:"timeout"` // seconds, default 10
}

// Validate checks required fields and sets defaults.
func (c *OutputConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("output name is required")
	}
	switch SinkType(c.Type) {
	case SinkThingsBoardMQTT:
		if c.Host == "" {
			return fmt.Errorf("output %q: host is required", c.Name)
		}
		if c.Token == "" {
			return fmt.Errorf("output %q: token is required for ThingsBoard auth", c.Name)
		}
	case SinkThingsBoardHTTP:
		if c.Host == "" {
			return fmt.Errorf("output %q: host is required", c.Name)
		}
		if c.Token == "" {
			return fmt.Errorf("output %q: token is required for ThingsBoard auth", c.Name)
		}
	default:
		return fmt.Errorf("output %q: unknown type %q", c.Name, c.Type)
	}
	if c.QoS == 0 {
		c.QoS = 1
	}
	if c.Timeout == 0 {
		c.Timeout = 10
	}
	if c.Port == 0 {
		c.Port = 1883
	}
	return nil
}

// ── Factory ─────────────────────────────────────────────────

// NewSinker creates a Sinker based on the output configuration type.
func NewSinker(cfg OutputConfig) (Sinker, error) {
	switch SinkType(cfg.Type) {
	case SinkThingsBoardMQTT:
		return NewThingsBoardMqttSinker(cfg)
	case SinkThingsBoardHTTP:
		return NewThingsBoardHttpSinker(cfg)
	default:
		return nil, fmt.Errorf("unknown sink type: %s", cfg.Type)
	}
}

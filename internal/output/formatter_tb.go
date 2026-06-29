package output

import (
	"LEPG/internal/model"
	"encoding/json"
	"fmt"
)

// ── ThingsBoard Gateway Telemetry ───────────────────────────
//
// Target topic:  v1/gateway/telemetry
// Auth:          Access Token as MQTT username
// Payload shape:
//
//	{
//	  "CLIENT001-温湿度传感器-01": [
//	    {"ts": 1719302400000, "values": {"temperature": 25.3}},
//	    {"ts": 1719302400000, "values": {"humidity": 68}}
//	  ]
//	}

// tbGatewayPayload is the top-level JSON object sent to
// v1/gateway/telemetry.  Its single key is the downstream device
// name in ThingsBoard; the value is an array of telemetry entries.
type tbGatewayPayload map[string][]tbTelemetryEntry

// tbTelemetryEntry represents one timestamped set of reading values.
// Matches ThingsBoard's per-device telemetry contract.
type tbTelemetryEntry struct {
	TS     int64          `json:"ts,omitempty"`
	Values map[string]any `json:"values"`
}

// ThingsBoardFormatter formats readings into TB Gateway JSON.
type ThingsBoardFormatter struct{}

// NewThingsBoardFormatter creates a ThingsBoard Gateway formatter.
func NewThingsBoardFormatter() *ThingsBoardFormatter {
	return &ThingsBoardFormatter{}
}

// Format produces the complete Gateway telemetry payload for one
// downstream device.  deviceKey is the TB device name (e.g.
// "CLIENT001-温湿度传感器-01").  Each Reading becomes its own
// telemetry entry with an independent timestamp.
func (f *ThingsBoardFormatter) Format(deviceKey string, readings []model.Reading) ([]byte, error) {
	if len(readings) == 0 {
		return nil, fmt.Errorf("no readings to format for device %q", deviceKey)
	}

	payload := make(tbGatewayPayload)
	entries := make([]tbTelemetryEntry, len(readings))

	for i, r := range readings {
		val, err := parseReadingValue(&r)
		if err != nil {
			return nil, fmt.Errorf("parse value for %q: %w", r.PointName, err)
		}

		key := r.PointName
		if key == "" {
			key = r.Point // fallback to hash
		}

		entries[i] = tbTelemetryEntry{
			TS:     r.Timestamp,
			Values: map[string]any{key: val},
		}
	}

	payload[deviceKey] = entries
	return json.Marshal(payload)
}

// parseReadingValue converts the serialized Value string back to
// the native Go type appropriate for JSON output.
func parseReadingValue(r *model.Reading) (any, error) {
	switch r.DataType {
	case model.DataTypeBool:
		return r.Value == "true", nil

	case model.DataTypeInt16, model.DataTypeInt32,
		model.DataTypeUint16, model.DataTypeUint32:
		f, _, err := model.ParseValue(r.DataType, r.Value)
		return int64(f), err

	case model.DataTypeFloat32, model.DataTypeFloat64:
		f, _, err := model.ParseValue(r.DataType, r.Value)
		return f, err

	case model.DataTypeJSON:
		var js any
		if err := json.Unmarshal([]byte(r.Value), &js); err != nil {
			return r.Value, nil // fall back to raw string
		}
		return js, nil

	default:
		return r.Value, nil
	}
}

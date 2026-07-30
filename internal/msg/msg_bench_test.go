package msg

import (
	"LEPG/internal/model"
	"net"
	"runtime"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────

// makeTestReadings produces n readings with realistic data shapes.
// Each reading ≈ 50B when gob-encoded (device/point hashes + float values).
func makeTestReadings(n int) []model.Reading {
	readings := make([]model.Reading, n)
	for i := range readings {
		readings[i] = model.Reading{
			Device:     "aaaaaaaaaaaaaaaa",
			DeviceName: "温湿度传感器-01",
			Point:      "bbbbbbbbbbbbbbbb",
			PointName:  "temperature",
			DataType:   model.DataTypeFloat32,
			Value:      "25.3",
			Quality:    model.QualityGood,
			Timestamp:  1719302400000,
		}
	}
	return readings
}

// preBuiltMsg caches an encoded message to avoid Encode overhead during Decode benchmarks.
func preBuiltMsg(readings []model.Reading) *Msg {
	f := NewMsgFactory()
	payload := &UploadPayload{Readings: readings}
	m, _ := f.NewMsg(MsgTypeUpload, payload)
	return m
}

// encodeToBytes returns the wire-format bytes of a message (Encode result).
func encodeToBytes(m *Msg) []byte {
	b, _ := m.Encode()
	return b
}

// sink prevents the compiler from eliminating benchmark results.
var sinkBytes []byte
var sinkMsg Msg
var sinkErr error

// ── M1: Encode ───────────────────────────────────────────────

// BenchmarkEncode measures encoding a fully-built Msg to wire format.
// This covers: headerAndPayload (6× binary.Write) + CRC16 append.
func BenchmarkEncode_12Readings(b *testing.B) {
	msg := preBuiltMsg(makeTestReadings(12))
	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkBytes, sinkErr = msg.Encode()
	}
}

func BenchmarkEncode_1Reading(b *testing.B) {
	msg := preBuiltMsg(makeTestReadings(1))
	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkBytes, sinkErr = msg.Encode()
	}
}

func BenchmarkEncode_100Readings(b *testing.B) {
	msg := preBuiltMsg(makeTestReadings(100))
	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkBytes, sinkErr = msg.Encode()
	}
}

// ── M1: DecodeFrame ───────────────────────────────────────────

// BenchmarkDecodeFrame uses net.Pipe to simulate a TCP connection,
// feeding pre-encoded bytes and measuring the full DecodeFrame path.
func BenchmarkDecodeFrame_12Readings(b *testing.B) {
	msg := preBuiltMsg(makeTestReadings(12))
	wire := encodeToBytes(msg)

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr, pw := net.Pipe()

		// Writer goroutine pushes encoded bytes and closes.
		go func() {
			pw.Write(wire)
			pw.Close()
		}()

		sinkMsg, sinkErr = DecodeFrame(pr)
		pr.Close()
	}
}

func BenchmarkDecodeFrame_1Reading(b *testing.B) {
	msg := preBuiltMsg(makeTestReadings(1))
	wire := encodeToBytes(msg)

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr, pw := net.Pipe()
		go func() {
			pw.Write(wire)
			pw.Close()
		}()

		sinkMsg, sinkErr = DecodeFrame(pr)
		pr.Close()
	}
}

func BenchmarkDecodeFrame_100Readings(b *testing.B) {
	msg := preBuiltMsg(makeTestReadings(100))
	wire := encodeToBytes(msg)

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr, pw := net.Pipe()
		go func() {
			pw.Write(wire)
			pw.Close()
		}()

		sinkMsg, sinkErr = DecodeFrame(pr)
		pr.Close()
	}
}

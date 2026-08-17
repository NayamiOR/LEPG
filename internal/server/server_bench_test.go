package server

import (
	"LEPG/internal/model"
	"LEPG/internal/msg"
	"LEPG/internal/output"
	"runtime"
	"testing"

)

// ── helpers ──────────────────────────────────────────────────

func makeUploadWithReadings(n int, seed int) *msg.Msg {
	readings := make([]model.Reading, n)
	for i := range readings {
		readings[i] = model.Reading{
			Device:     "aaaaaaaaaaaaaaaa",
			DeviceName: "温湿度传感器-01",
			Point:      "bbbbbbbbbbbbbbbb",
			PointName:  "temperature",
			DataType:   model.DataTypeFloat32,
			Value:      floatToString(25.3 + float64(seed)*0.01),
			Quality:    model.QualityGood,
			Timestamp:  1719302400000 + int64(seed),
		}
	}
	return makeUploadMsg(readings)
}

func floatToString(f float64) string {
	s := make([]byte, 0, 8)
	intPart := int(f)
	fracPart := int((f-float64(intPart))*100 + 0.5)
	s = append(s, byte('0'+intPart/10), byte('0'+intPart%10), '.')
	if fracPart < 10 {
		s = append(s, '0', byte('0'+fracPart))
	} else {
		s = append(s, byte('0'+fracPart/10), byte('0'+fracPart%10))
	}
	return string(s)
}

var benchSink bool

func BenchmarkHandleUpload_1Reading(b *testing.B) {
	uploadDedup.clear()
	store := newMockStore()
	pub := newMockPublisher()
	sink := newMockSinker("bench-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})
	factory := msg.NewMsgFactory()

	msgs := make([]*msg.Msg, b.N)
	for i := 0; i < b.N; i++ {
		msgs[i] = makeUploadWithReadings(1, i)
	}

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleUpload(&discardConn{}, factory, store, pub, router, msgs[i], "CLIENT001", "127.0.0.1:12345")
	}
	router.Shutdown()
	benchSink = len(sink.getSendCalls()) > 0
}

// ── M2: handleUpload (full pipeline, unique payload per iteration) ──

func BenchmarkHandleUpload_12Readings(b *testing.B) {
	uploadDedup.clear()
	store := newMockStore()
	pub := newMockPublisher()
	sink := newMockSinker("bench-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})
	factory := msg.NewMsgFactory()

	msgs := make([]*msg.Msg, b.N)
	for i := 0; i < b.N; i++ {
		msgs[i] = makeUploadWithReadings(12, i)
	}

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleUpload(&discardConn{}, factory, store, pub, router, msgs[i], "CLIENT001", "127.0.0.1:12345")
	}
	router.Shutdown()
	benchSink = len(sink.getSendCalls()) > 0
}

func BenchmarkHandleUpload_100Readings(b *testing.B) {
	uploadDedup.clear()
	store := newMockStore()
	pub := newMockPublisher()
	sink := newMockSinker("bench-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})
	factory := msg.NewMsgFactory()

	msgs := make([]*msg.Msg, b.N)
	for i := 0; i < b.N; i++ {
		msgs[i] = makeUploadWithReadings(100, i)
	}

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleUpload(&discardConn{}, factory, store, pub, router, msgs[i], "CLIENT001", "127.0.0.1:12345")
	}
	router.Shutdown()
	benchSink = len(sink.getSendCalls()) > 0
}

func BenchmarkHandleUpload_DedupHit(b *testing.B) {
	uploadDedup.clear()
	store := newMockStore()
	pub := newMockPublisher()
	sink := newMockSinker("bench-sink")
	router := output.NewOutputRouter([]output.Sinker{sink})
	factory := msg.NewMsgFactory()
	uploadMsg := makeUploadWithReadings(12, 0)

	uploadDedup.add(payloadKey(uploadMsg.Payload))

	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleUpload(&discardConn{}, factory, store, pub, router, uploadMsg, "CLIENT001", "127.0.0.1:12345")
	}
	router.Shutdown()
}

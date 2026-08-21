package server

import (
	"LEPG/internal/model"
	"LEPG/internal/output"
	"testing"
)

// TestHandleUpload_DedupBySN 验证 dedup key 加 SN 后的正确性语义（2026-08-21）：
// 1) 同一 SN 重传相同 payload → 第二次被去重跳过（只入库一次）
// 2) 不同 SN 上传相同 payload → 互不干扰（不再跨设备误杀）
func TestHandleUpload_DedupBySN(t *testing.T) {
	uploadDedup.clear()
	store := newMockStore()
	pub := newMockPublisher()
	sink := newMockSinker("dedup-sink")
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
	}
	uploadMsg := makeUploadMsg(readings)

	// 1) 同一 SN 连传两次相同 payload：第二次应被 dedup 跳过
	handleUpload(&discardConn{}, testFactory, store, pub, router, uploadMsg, "SN-A", "127.0.0.1:1")
	handleUpload(&discardConn{}, testFactory, store, pub, router, uploadMsg, "SN-A", "127.0.0.1:1")
	router.Shutdown()

	store.mu.Lock()
	savedA := len(store.savedReadings["SN-A"])
	store.mu.Unlock()
	if savedA != 1 {
		t.Fatalf("same SN duplicate upload: expected 1 save, got %d", savedA)
	}

	// 2) 不同 SN 上传相同 payload：不应被全局缓存误杀
	handleUpload(&discardConn{}, testFactory, store, pub, router, uploadMsg, "SN-B", "127.0.0.1:2")
	router.Shutdown()

	store.mu.Lock()
	savedB := len(store.savedReadings["SN-B"])
	store.mu.Unlock()
	if savedB != 1 {
		t.Fatalf("different SN same payload: expected 1 save, got %d", savedB)
	}
}

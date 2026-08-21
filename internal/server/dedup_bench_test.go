package server

import (
	"strconv"
	"testing"
)

// ── SHA256 payloadKey / dedup LRU 组件 bench ──────────────────
// 2026-08-21 新增：量化 payloadKey 哈希成本与 dedup LRU 的 exists/add 成本。
// 对应执行计划 A2 的组件拆分（BenchmarkPayloadKey + BenchmarkDedupExists/Add）。

// BenchmarkPayloadKey 测 payloadKey 对三种 payload 大小的 SHA256 哈希成本。
func BenchmarkPayloadKey(b *testing.B) {
	sizes := []int{256, 1024, 4096}
	for _, n := range sizes {
		b.Run("payload="+strconv.Itoa(n)+"B", func(b *testing.B) {
			payload := make([]byte, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = payloadKey(payload)
			}
		})
	}
}

// BenchmarkDedupExists 测 LRU 命中路径：查 key + MoveToFront。
func BenchmarkDedupExists(b *testing.B) {
	c := newDedupCache(dedupCacheSize)
	key := payloadKey(make([]byte, 1024))
	c.add(key)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.exists(key)
	}
}

// BenchmarkDedupAdd 测 LRU 写路径：循环 add 不同 key，持续触发 evict（容量 200）。
func BenchmarkDedupAdd(b *testing.B) {
	c := newDedupCache(dedupCacheSize)
	keys := make([]string, 256)
	for i := range keys {
		keys[i] = payloadKey([]byte{byte(i)})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.add(keys[i%len(keys)])
	}
}

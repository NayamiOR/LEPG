package server

import (
	"LEPG/internal/model"
	"strconv"
	"testing"
)

// ── groupReadingsByDeviceName / Prealloc 组件 bench ──────────
// 2026-08-21 新增（performance-analysis skill 首跑）：量化 slice realloc 成本。
// 两种输入分布：group=1 全部同名（对齐既有 27% mem 归因，realloc 最狠）；
//              group=4 四种设备名轮流（更贴近真实多设备场景）。
// 注意：函数拆成两个（单遍/预分配）是因为条件分支会破坏编译器 map 优化。

// BenchmarkGroupReadingsPrealloc 测两遍预分配路径（调用方 n≥8 时使用）。
func BenchmarkGroupReadingsPrealloc(b *testing.B) {
	names4 := []string{"温湿度传感器-01", "电表-02", "水表-03", "气表-04"}
	for _, n := range []int{12, 100, 1000} {
		b.Run("n="+strconv.Itoa(n), func(b *testing.B) {
			readings := make([]*model.Reading, n)
			for i := range readings {
				readings[i] = &model.Reading{
					DeviceName: names4[i%len(names4)],
					DataType:   model.DataTypeFloat32,
					Value:      "25.3",
					Quality:    model.QualityGood,
					Timestamp:  1719302400000 + int64(i),
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = groupReadingsPrealloc(readings)
			}
		})
	}
}

func BenchmarkGroupReadingsByDeviceName(b *testing.B) {
	names1 := []string{"温湿度传感器-01"}
	names4 := []string{"温湿度传感器-01", "电表-02", "水表-03", "气表-04"}
	for _, n := range []int{1, 12, 100, 1000} {
		for _, tc := range []struct {
			groups int
			names  []string
		}{
			{1, names1},
			{4, names4},
		} {
			names := tc.names
			b.Run("n="+strconv.Itoa(n)+"/group="+strconv.Itoa(tc.groups), func(b *testing.B) {
				readings := make([]*model.Reading, n)
				for i := range readings {
					readings[i] = &model.Reading{
						DeviceName: names[i%len(names)],
						DataType:   model.DataTypeFloat32,
						Value:      "25.3",
						Quality:    model.QualityGood,
						Timestamp:  1719302400000 + int64(i),
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_ = groupReadingsByDeviceName(readings)
				}
			})
		}
	}
}

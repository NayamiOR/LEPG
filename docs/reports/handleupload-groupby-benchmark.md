# groupReadingsByDeviceName 预分配优化：证据链

> 2026-08-21，performance-analysis skill 首跑。针对 groupBy 的 slice 反复 realloc 优化。
> 数据命名隔离：未覆盖任何历史 prof/文档（`cpu.prof`/`mem.prof`/`*-tlv.prof` 等均保留）。

## 一、环境

| 项 | 值 |
|---|---|
| CPU | AMD Ryzen 5 9600X 6C12T |
| OS / Go | Windows / go1.26.0 |
| Before | 原始单遍 append（realloc） |
| After | 调用方按规模选择：<8 单遍 / ≥8 两遍预分配 |

## 二、采集命令（可复现）

```bash
# 组件 bench（单遍 + 预分配两路径）
go test -bench="BenchmarkGroupReadings" -benchmem -run=^$ -count=1 ./internal/server/

# 整体 bench（调用方分支选择）
go test -bench=BenchmarkHandleUpload -benchmem -run=^$ -count=1 ./internal/server/

# 既有归因（mem-tlv.prof，8/19 采集，未动）
go tool pprof -alloc_space -top server-tlv.test.exe mem-tlv.prof
```

## 三、Before（原始单遍，realloc 路径）

| 规模 | group=1 全部同名 | group=4 四种设备 |
|---|---|---|
| n=1 | 50ns / 144B / 1 allocs | 51ns / 144B / 1 |
| n=12 | 915ns / 4,464B / 5 | 1,004ns / 4,032B / 12 |
| n=100 | 6,469ns / 39,536B / 8 | 7,374ns / 37,312B / 24 |
| n=1000 | 68,093ns / **490,097B** / 12 | 78,820ns / 321,985B / 36 |

**归因**：mem 27.02%（最大真实分配源）——`grouped[key] = append(grouped[key], *r)` 从 nil 翻倍 realloc（模式库模式 3）。group=1 时 slice 翻倍到 2048 容量 vs 实际 1000，超额 ~50%。

## 四、修改内容（含一次关键发现）

### 4.1 方案演进

1. **纯两遍预分配**（第一版）：n=1 退化（50→166ns，1→3 allocs）——两遍扫描固定成本对小输入是净损失。
2. **阈值分支版**（`if len < 8`）：**发现 Go 编译器行为**——函数体内加条件分支会破坏对 map 的逃逸/栈上优化，**逻辑等价但 n=1 仍 544B/3**（用 `-gcflags` 判别实验排除构建缓存、抽函数、遍历方式后确认是分支本身）。
3. **最终版（双函数）**：单遍函数保持无分支原始形态（编译器优化恢复 144B/1），新增 `groupReadingsPrealloc`，调用方（handleUpload）按 `len(readings) < 8` 选择。

```go
// 调用方（server.go）
var grouped map[string][]model.Reading
if len(readings) < 8 {
    grouped = groupReadingsByDeviceName(readings)  // 单遍，无分支
} else {
    grouped = groupReadingsPrealloc(readings)      // 两遍预分配
}
```

### 4.2 关键发现（写回 skill 模式库）

**函数体内加条件分支可能破坏编译器优化**：同逻辑下，无分支函数 map 分配 144B/1 allocs，有分支退化到 544B/3 allocs。解法：拆成无分支小函数 + 调用方分支。

## 五、After

### 5.1 组件 bench（两路径对比，group=4）

| n | 单遍（<8 用） | 预分配（≥8 用） | 收益 |
|---|---|---|---|
| 12 | 1,013ns / 4,032B / 12 | 824ns / **2,520B** / 9 | -37% B |
| 100 | 7,198ns / 37,312B / 24 | 4,026ns / **17,720B** / 9 | -53% B |
| 1000 | 68,307ns / 321,985B / 36 | 44,868ns / **218,849B** / 11 | -32% B |

n=1 单遍恢复：53ns / **144B / 1 allocs**（与原始一致，无退化）。

### 5.2 整体 handleUpload

| 场景 | 加 SN 版（before） | 最终版（after） | 确定性变化 |
|---|---|---|---|
| 1Reading | 2,780ns / 1,776B / 28 | 2,978ns / **1,731B / 28** | B -3%，allocs 不变 ✅ |
| 12Readings | 9,657ns / 14,003B / 120 | 10,024ns / **12,136B** / 121 | **B -13%** ✅ |
| 100Readings | 45,298ns / 110,292B / 803 | 46,141ns / **89,016B** / 801 | **B -19%** ✅ |
| DedupHit | 1,235ns / 144B / 6 | 1,295ns / 144B / 6 | 不变 ✅ |

（ns 跨 session 有噪声，以 allocs/B 确定性指标为准；全量 `go test ./...` 10 包全绿。）

## 六、思路（方法论复盘）

- **视角**：groupBy 是分配型问题——CPU 图只有 4.97%，mem 图 27%。看错视角会错过它（模式库模式 3 已标注）。
- **规模依赖**：realloc 收益随 n 放大（n=1 无收益、n=1000 省 32-50% 字节）——必须按典型场景评估，别只看单一规模。
- **编译器黑箱**：行为实验（判别阈值改 1000、内联/抽函数/遍历方式对照）证明"分支破坏优化"——`-benchmem` 的 allocs 是确定性判据，比读文档猜编译器行为可靠。
- **阈值 8**：n=1 单遍优、n=12 预分配已明显优，crossover 在 2~8 之间，取 8 保守（未测 2-7，可后续补）。

## 七、文件清单

| 文件 | 动作 |
|---|---|
| `internal/server/server.go` | 修改：拆单遍/预分配两函数 + 调用方分支（~40 行） |
| `internal/server/group_bench_test.go` | 新增：两路径组件 bench（A2 欠账） |
| `docs/reports/handleupload-groupby-benchmark.md` | 新增：本文档 |
| `bench-archive/`、`*.prof`、`*-tlv.prof`、`server*.test.exe` | 保留未动 |

---

*验证：build ✅ / 全量 test ✅（10 包）。证据：`group_bench_test.go` 可复现。*

# SHA256 双算修复 + dedup key 加 SN：完整证据链

> 2026-08-21 记录。针对 dedup 的 SHA256 双算问题，完整走完 排查 → 定位 → 修改 → 测试 链条。
> 不覆盖既有资料：所有历史 prof/test.exe 保留，旧设备 bench 数据归档至 `bench-archive/`。

---

## 一、问题背景与定位（复用已有归因）

- **现象**：`sha256.blockSHANI` 占 CPU flat 4.17%（TLV 版 profile 实测，0.52s / 12.48s 总样本）
- **定位**（详见 `handleupload-performance-analysis.md` 第 4~7 步的归因方法）：火焰图只显示 sha256 占 5%，**"双算"是读代码发现的**——`server.go` 原代码 `exists(payloadKey(...))` + `add(payloadKey(...))` 各算一次 SHA256（`server.go:174/179`，双算的发现依赖"一次 hash 不该这么多"的程序直觉 + 读代码确认）
- **状态说明**：`payloadKey` 变量统一（只算一次）的修复在 2026-08-21 之前已完成（git staged）；本次补上 ①SN 前缀（正确性）②组件 bench ③正确性测试 ④完整证据记录

## 二、修改内容

### 2.1 修复前（历史版本）

```go
if uploadDedup.exists(payloadKey(message.Payload)) {   // 第 1 次 SHA256
    ...
}
uploadDedup.add(payloadKey(message.Payload))           // 第 2 次 SHA256
```

### 2.2 修复后（`internal/server/server.go:174`）

```go
key := sn + ":" + payloadKey(message.Payload)   // SHA256 只算一次 + key 带 SN
if uploadDedup.exists(key) { ... }
uploadDedup.add(key)
```

**SN 前缀的意义（正确性修复）**：原 key 纯 payload hash、全局 LRU 共享（容量 200），存在 ①跨设备同 payload 误杀 ②单设备 entry 被其他设备挤出 → 真重传漏判。加 SN 后按设备隔离去重（详见 `Tola/2026-08-14-LEPG-dedup去重key语义分析` 档位 A）。

## 三、组件 bench（新增，对应执行计划 A2）

新增 `internal/server/dedup_bench_test.go`：`BenchmarkPayloadKey`（256B/1KB/4KB）+ `BenchmarkDedupExists` + `BenchmarkDedupAdd`。

命令：
```bash
go test -bench="BenchmarkPayloadKey|BenchmarkDedup" -benchmem -run=^$ ./internal/server/
```

结果（2026-08-21，加 SN 前后各跑一次，组件不受 SN 影响）：

| bench | before | after | 说明 |
|---|---|---|---|
| payloadKey 256B | 155.6 ns / 0 allocs | 154.2 ns / 0 allocs | SHA256 小 payload 独立成本 |
| payloadKey 1KB | 441.4 ns / 0 allocs | 440.8 ns / 0 allocs | |
| payloadKey 4KB | 1594 ns / 0 allocs | 1595 ns / 0 allocs | |
| DedupExists（hit） | 12.19 ns / 0 allocs | 12.19 ns / 0 allocs | LRU 查询 + MoveToFront |
| DedupAdd（miss+evict） | 60.58 ns / 2 allocs | 62.01 ns / 2 allocs | 容量 200，持续 evict |

**关键事实：`payloadKey` 实测 0 allocs**——`string(h[:])` 的转换分配被编译器优化掉（修正了此前"每次分配"的推测，此前 mem.prof 未归因到该分配是有原因的）。

## 四、正确性测试（新增）

新增 `internal/server/dedup_test.go` 的 `TestHandleUpload_DedupBySN`，验证 SN 语义：

| 场景 | 期望 | 实测 |
|---|---|---|
| 同 SN 重传同 payload | 第二次被 dedup 跳过（只入库 1 次） | ✅ |
| 不同 SN 同 payload | 互不误杀（各入库 1 次） | ✅ |

```bash
go test -run=TestHandleUpload_DedupBySN ./internal/server/
```

## 五、整体 bench（加 SN 前后）

命令：`go test -bench=BenchmarkHandleUpload -benchmem -run=^$ ./internal/server/`

| benchmark | 8/19 TLV 无 SN | 8/21 TLV + SN | allocs 变化 |
|---|---|---|---|
| 1Reading | 2,980 ns / 28 allocs | 2,780 ns / 28 allocs | 0 |
| 12Readings | 11,569 ns / 120 allocs | 9,657 ns / 120 allocs | 0 |
| 100Readings | 54,635 ns / 796 allocs | 45,298 ns / 803 allocs | **+7**（三轮复测 797/803/803 稳定） |
| DedupHit | 1,289 ns / 6 allocs | 1,235 ns / 6 allocs | 0 |

**结论**：
- 1Reading/12Readings 的 allocs 完全无变化 → SN 拼接在短 payload 下无额外分配
- 100Readings allocs +7（<1%）→ 大 payload 下 `sn + ":" + hash` 拼接逃逸分配的连锁反应，性能影响可忽略
- ns/op 跨 2 天对比有环境漂移，不作为主要依据；**allocs 是确定性指标，是本次判断的核心**

## 六、文件清单

| 文件 | 动作 |
|---|---|
| `internal/server/server.go` | 修改：key 加 SN（SHA256 只算一次为既有 staged 改动） |
| `internal/server/dedup_bench_test.go` | 新增：组件 bench（A2 欠账） |
| `internal/server/dedup_test.go` | 新增：SN 正确性测试 |
| `docs/reports/handleupload-sha256-benchmark.md` | 新增：本文档 |
| `bench-archive/`（bench.txt、server_bench.txt） | 归档：旧设备 Intel 数据 |
| `cpu.prof`、`mem.prof`、`cpu-tlv.prof`、`mem-tlv.prof`、`server.test.exe`、`server-tlv.test.exe` | 保留：历史证据链未动 |

---

*验证：`go build ./...` ✅，`go test ./...` 全绿 ✅（含新增两个测试）。*

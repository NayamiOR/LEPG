# LEPG handleUpload 性能研究：从数据到根因的推理链

> 承接 `docs/reports/benchmarks.md`（基线采集），本文档记录从原始数据出发、**逐步推理**出优化方向与设计的过程。
>
> 方法约束：每一步只使用①可验证的数据、②通用原则（采样原理、守恒关系、T(N) 模型、行为实验）。不做经验性跳跃——所有"应该怎样"的结论都先从数据推出，再读自己的代码确认机制。
>
> **关于起点的诚实声明**：分析开始前存在一个来自火焰图初探的**初始假设 H——"gob 解码相关路径占大头"**。但下面的推理链不依赖 H 起步：第 1~3 步从 handleUpload 的全景分支分布出发，谁占比最大就下钻谁，焦点由数据推出。H 只作为"值得关注 gob"的提示，在过程中被证实。
>
> 环境：AMD Ryzen 5 9600X 6C12T，Go 1.2x，Windows；profile 文件 `cpu.prof`（2026-08-16 采集，4 个 benchmark 混合）。

---

## 0. 分析原则（先立起来，后面每一步引用）

| 编号 | 原则 | 依据 |
|---|---|---|
| P1 | **采样原理**：pprof 周期采样，flat = 函数在栈顶的采样数，cum = 函数在栈上任意位置的采样数 | pprof 文档 / 采样机制 |
| P2 | **守恒关系**：parent.cum = parent.flat + Σ(child.cum) | 由 P1 推出（样本要么落在自己，要么落在某个子树） |
| P3 | **时间模型**：T(N) = F + c·N（固定开销 + 线性可变） | 任何处理管线的通用分解 |
| P4 | **占比相对性**：百分比 = 数值 ÷ 分母，分母选择决定结论含义 | 算术定义 |
| P5 | **行为实验**：控制变量改输入，观察输出变化，推断内部机制（黑箱可用） | 科学方法 |

目标：回答一个问题——**handleUpload 的最大可优化项是什么，为什么**。

---

## 第 1 步：handleUpload 全景——最大分支是谁

**观察（数据）**：`-peek=handleUpload` 列出 handleUpload 的所有直接子调用，calls% 是各子调用占 handleUpload cum（6.26s）的比例：

```
handleUpload 6.26s (100%)
  ├─ ParseMsg               2.76s (44.1%)   ← 最大分支
  ├─ serializeReadings      0.89s (14.2%)
  ├─ slog.Info              0.77s (12.3%)
  ├─ payloadKey             0.60s ( 9.6%)
  ├─ sendAck                0.59s ( 9.4%)
  ├─ groupReadingsByDevice  0.25s ( 4.0%)
  ├─ OutputRouter.Send      0.10s ( 1.6%)
  ├─ dedupCache add/exists  0.11s ( 1.8%)
  └─ 其余小项              ~0.40s ( 6.4%)
```

**数据来源（复现命令）**：

```bash
go tool pprof -peek=handleUpload server.test.exe cpu.prof
```

读法：每行右侧 calls% = 该子调用占 handleUpload cum 的比例；左侧数字 = 该子调用的 cum。

**应用 P2（守恒）**：handleUpload 的 6.26s 必须全部分配给直接子调用，占比就是"谁吃掉 handleUpload 时间"的直接度量，无遗漏、无重叠。

**结论**：**主要优化项候选 = ParseMsg（44.1%）**，是第二名（14.2%）的 3 倍多。其余分支（slog 12.3% 等）占比显著更小，暂不构成主项。H（gob 相关）尚不能判定——ParseMsg 内部是什么，需要下钻。

---

## 第 2 步：下钻 ParseMsg——焦点收敛到 gob 解码

**观察（数据）**：`-top -cum` 显示 ParseMsg 的调用链：

```
ParseMsg          2.76s (25.07%)
  └─ decodeUpload 2.74s (24.89%)
       └─ Decode  2.74s (24.89%)
            └─ gob.Decode 2.70s (24.52%)
```

**数据来源（复现命令）**：

```bash
go tool pprof -top -cum server.test.exe cpu.prof
```

**应用 P2**：ParseMsg 2.76s 中 gob.Decode 占 2.70s = **97.8%**，整条链在 `ParseMsg → decodeUpload → UploadPayload.Decode → gob.Decode` 上几乎无衰减。

**结论**：最大分支 ParseMsg 的主体就是 **gob 解码**。H 在此得到部分证实——gob 确实是 handleUpload 的主要时间来源。下一问：gob.Decode 内部的时间花在哪。

---

## 第 3 步：下钻 gob.Decode——compileDec 浮出水面

**观察（数据）**：`-peek=decodeValue`（gob 解码主循环）：

```
decodeValue (2.53s, 22.98%)
  ├─ 被调用来源：recvType 1.37s (54.15%) / DecodeValue 1.16s (45.85%)
  └─ 子调用：
       getDecEnginePtr  1.63s (64.43%)   ← 取"解码引擎"（缓存查 + miss 编译）
       decodeSingle     0.63s (24.90%)   ← 实际解码（真正的数据工作）
       decodeStruct     0.24s ( 9.49%)
```

再下钻 getDecEnginePtr：`compileDec` 1.43s（12.99%）是其最大子项。

**数据来源（复现命令）**：

```bash
go tool pprof -peek=decodeValue server.test.exe cpu.prof
go tool pprof -peek=getDecEnginePtr server.test.exe cpu.prof
```

**应用 P2（守恒）**：decodeValue 的 64% 花在"取引擎"上，而取引擎又主要花在 compileDec；**实际解码（decodeSingle + decodeStruct）只占 34%**——数据工作不是大头，编译才是。

**结论**：焦点从 ParseMsg 一路下钻，**收敛到 compileDec**（占 decodeValue 的 ~57%）。它是这条链上最深、占比最大的固定点。下一问：它是否值得改（是固定开销吗）。

---

## 第 4 步：判定 compileDec 是固定开销（关键一步）

**观察（数据）**：compileDec 相对 handleUpload 的占比在三个规模下**递减**（分母校准见下）：

```
compileDec / handleUpload
  1Reading     : 0.68s / 1.39s = 48.9%
  12Readings   : 0.47s / 1.44s = 32.6%
  100Readings  : 0.28s / 2.11s = 13.3%
```

**数据来源（复现命令）**：cum 值来自三条 focus 命令（`-focus` 只保留匹配函数的整棵调用子树，cum 列只统计对应 benchmark 内的样本）：

```bash
go tool pprof -top -focus=BenchmarkHandleUpload_1Reading server.test.exe cpu.prof
go tool pprof -top -focus=BenchmarkHandleUpload_12Readings server.test.exe cpu.prof
go tool pprof -top -focus=BenchmarkHandleUpload_100Readings server.test.exe cpu.prof
```

（注：`-top` 的 % 分母是全部采样 11.01s，含 4 个 benchmark + setup；看"占 handleUpload 多少"必须除以 handleUpload 的 cum，这就是 P4 的分母校准。）

**应用 P3**：若 T(N) = F + c·N（F 固定、c 线性），则 F 的占比 = F / T(N)，**随 N 增大必然递减**——与观察一致。若它是可变开销（c·N 的一部分），占比应**恒定**。所以占比递减 → compileDec 是固定成本候选。

**验证（per-op 绝对成本，P4 的进一步应用）**：

```
compileDec 总时间 ÷ 迭代次数 = 单次编译成本
  1Reading   : 0.68s ÷ 52,440 次 = 13.0µs
  12Readings : 0.47s ÷ 36,537 次 = 12.9µs
  （100Readings 采样样本太少，噪声大，不采用）
```

迭代次数 = benchmarks.md 中 benchmark 输出第一列的数字（如 `BenchmarkHandleUpload_1Reading-12  52440 ...` 的 52,440），即 `go test -bench` 自动确定的 b.N。

单次成本在两种规模下**恒定 ~13µs** → 与 readings 数量无关 → 纯固定开销实锤。

**反证（排除"只编译一次"的可能）**：若 compileDec 只在 benchmark 开头编译一次，单次成本应 = 13µs ÷ 52,440 ≈ 0.00025µs，占比趋近 0。实测 13.0µs → **每次 Decode 都在重新编译**。

**结论**：compileDec 是 handleUpload 链路中**最深、且每次请求固定支付 ~13µs** 的优化点（1Reading 路径上占近一半）。这是"每单都收的税"。

---

## 第 5 步：为什么每次都在编译？（读自己的代码确认机制）

**观察（数据）**：`-peek=compileDec` 显示其调用者 100% 是 `getDecEnginePtr`，而 getDecEnginePtr 在每次 Decode 都会执行。

**应用 P5 + 读自己的代码**：`internal/msg/payload.go:88`：

```go
func (p *UploadPayload) Decode(data []byte) error {
    if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p.Readings); ...
}
```

**机制确认（gob 源码，只读一次）**：`getDecEnginePtr` 的缓存 `decoderCache` 是 **Decoder 实例级**（`decode.go:1186`）。每次 `gob.NewDecoder` 都是全新实例 → 缓存清空 → 全部类型（UploadPayload、model.Reading 两个 struct）重新编译。

**结论**：根因 = **每次 Decode 都新建 Decoder，导致编译缓存失效**。修复方向一度认为是"复用 Decoder 使编译只发生一次"——但该方向被实验证伪，见第 7 步。

---

## 第 6 步：为什么 Encode 侧不动（协议约束）

**观察（gob 协议性质）**：gob 流是**有状态的**——类型描述符只随流发送一次，对端用 typeId 引用；且**每个独立流中自定义类型从 typeId 65 重新编号**。

**推理（不依赖经验）**：
- 若复用 Encoder，它会认为"类型已发送过"，后续 Encode 不再带类型定义；
- 但 LEPG 每条 payload 是**独立 gob 流**（对端每次 `gob.NewDecoder`），对端需要类型定义；
- → Encoder 复用会导致对端解码失败，**协议破坏**。

**补充（数据）**：Encode 侧编译缓存 `encGenMap` 是**包级全局**（`encode.go`），编译不是 Encode 的瓶颈；且 Encode 不在 handleUpload 热路径（是 client 造消息/benchmark setup 用）。

**结论**：Encode 侧保持现状（其类型描述符发送开销属于独立流协议的必要成本）。

---

## 第 7 步：方案实验与证伪（重要）

**原设计**：`sync.Pool` 复用 `*gob.Decoder` + 自定义 `resetReader`（因为 Decoder 没有公开 Reset 方法，靠切换数据源实现复用）。预期 compileDec 归零，1Reading 从 ~25.1µs 减 ~13µs。

```go
type resetReader struct{ data []byte; off int }  // Reset(data) 后作 io.Reader
// pool: sync.Pool{ New: func() any { return &decoderPair{dec: gob.NewDecoder(nil), r: &resetReader{}} } }
// Decode: r.Reset(data); dec.Decode(&p.Readings)
```

**实验（2026-08-19，用 Claude Code 实施）**：改动落地后 `go test ./...` 失败：

```
msg_test.go:248: ParseMsg failed: upload payload decode: gob: duplicate type received
```

**证伪根因**：gob 的**类型定义表 `dec.wireType` 也是 Decoder 实例级**，且**每个独立流自定义类型从 typeId 65 重新编号**。复用 Decoder 后，`wireType` 残留上次流的映射（typeId 65 = []model.Reading）；新流再次定义 typeId 65 → 冲突 → `duplicate type received`。第 5 步只看到了 `decoderCache`（编译缓存）实例级，漏掉了 `wireType`（类型表）同样实例级、且与独立流编号冲突——**"Decoder 复用安全"是错误判断**。

**修正结论**：在独立流协议下，gob 的**类型接收（recvType）+ 类型编译（compileDec）是协议固有成本**，无法通过复用规避；compileDec 的 ~13µs/次 从"可优化项"降级为"协议税"。要消除它只剩一条路：**更换编码格式**（见第 8 步，已实施）。

**实验的价值（P5）**：假设 → 实验 → 证伪，成本只有一次 `go test`。测试在开发期抓住协议破坏，比线上炸掉好一万倍。

---

## 第 8 步：替代方案——手写 TLV（已实施，收益超预期）

**决策**：既然 gob 的编译/类型成本在独立流下不可规避，而 LEPG 其他 payload（Handshake/Ack/Notify）**本来就是手写 TLV**（gob 是早期实现的历史包袱，git 溯源：最早 `ce5584a` 直接用 gob 打通上传，后来 `28be4de`/`4eb165f` 才定 TLV 规范，`b535f86` 重构时注释"沿用现有 gob 格式"保留）——换手写 TLV 不是引入新东西，是**回归项目统一风格**。

**格式（大端序，与项目其他 payload 一致）**：

```
UploadPayload = [Count:4B] + Count × Reading
Reading: [ID:8B][DevLen:2B][Device][NameLen:2B][DeviceName][PointLen:2B][Point]
         [PointNameLen:2B][PointName][DataTypeLen:2B][DataType][ValLen:2B][Value]
         [Quality:1B][UnitLen:2B][Unit][Timestamp:8B]
```

**实现要点**：Encode 预计算总长 `make + append`（零额外分配）；Decode 逐字段 offset 解析 + 每步剩余长度校验（防截断/恶意 count）；字符串 > 65535 报错；count 超过 `len(data)/31` 拒绝（防大分配）。错误语义与接口不变。往返/截断/超长/huge-count 四类测试补充。

**before/after（9600X，8/16 gob 版 vs 8/19 TLV 版）**：

| benchmark | gob (8/16) | 手写 TLV (8/19) | 变化 |
|---|---|---|---|
| 1Reading | 25.1µs / 252 allocs / 11.1KB | **3.8µs / 28 allocs / 1.7KB** | **-85% / -89% / -85%** |
| 12Readings | 35.8µs / 342 allocs | **13.9µs / 120 allocs** | **-61% / -65%** |
| 100Readings | 110.2µs / 985 allocs | **72.4µs / 792 allocs** | -34% / -20% |
| DedupHit | 2.5µs / 25 allocs | **1.5µs / 6 allocs** | -39% / -76% |

**收益随数据量递减（1Reading 最大）——这正是固定开销模型（P3）的印证**：砍掉的 gob 编译税是固定成本，占比随 N 增大而摊薄，所以小数据量收益最大。allocs 是确定性指标（不受环境漂移影响），252→28 无争议；ns 跨 3 天对比有漂移风险，但 -85% 远超漂移量级。

**叙事闭环**：分析定位（compileDec 固定税）→ 方案一被证伪（sync.Pool）→ 换道（手写 TLV）→ 成功。三个方案恰好组成完整的故事：读得懂数据、敢试错、错得起、换道快。

---

## 优化清单（TLV 落地后）

| 项目 | 依据 | 状态 |
|---|---|---|
| ~~gob 编译/类型成本~~ | **已消除**：手写 TLV 替代，1Reading 25.1→3.8µs | **✅ 已完成** |
| ~~SHA256 双算 + key 加 SN~~ | 双算变量统一（既有）+ SN 前缀（8/21），组件 bench 与证据见 handleupload-sha256-benchmark.md | **✅ 已完成** |
| groupBy 预分配容量 | mem 27.02%（TLV 版实测最大真实分配源） | 待做（几行） |
| benchmark 关 log（`slog.SetDefault(io.Discard)`） | DedupHit 路径 log 相对占比大 | 待做（一行，测试侧） |
| JSON serializeReadings | 可变成本，换 fastjson 收益/工作量比低 | 不抠 |

---

## 附录 A：关键数据表（本文档引用）

| 指标 | 1Reading | 12Readings | 100Readings | 来源 |
|---|---|---|---|---|
| handleUpload ns/op | 25,091 | 35,767 | 110,203 | benchmarks.md (8/16) |
| allocs/op | 252 | 342 | 985 | benchmarks.md (8/16) |
| ParseMsg / handleUpload | — | — | — | 44.1%（全局，`-peek=handleUpload`） |
| gob.Decode / ParseMsg | — | — | — | 97.8%（全局，`-top -cum`） |
| compileDec cum / handleUpload | 48.9% | 32.6% | 13.3% | focus 分别计算 |
| compileDec per-op | 13.0µs | 12.9µs | ~25µs* | 总时间÷迭代次数 |

*100Readings 采样样本少，噪声大，不采用。

## 附录 B：推理链一图流

```
第 1 步 全景：ParseMsg 44.1% 是 handleUpload 最大分支   （-peek=handleUpload，P2）
第 2 步 下钻：ParseMsg 97.8% 在 gob.Decode             （-top -cum，P2）
第 3 步 下钻：decodeValue 64% 在 getDecEnginePtr→compileDec（-peek，P2）
第 4 步 判定：per-op 恒定 13.0µs/12.9µs → 固定开销+每次编译（P3/P4）
第 5 步 机制：payload.go:88 每次 NewDecoder → 缓存实例级失效（P5+读码）
第 6 步 边界：Encoder 复用破坏协议 → Encode 不动
第 7 步 证伪：Decoder 复用同样破坏协议（duplicate type received）
第 8 步 换道：手写 TLV 替代 gob → 1Reading 25.1→3.8µs，allocs 252→28 ✅
```

---

*文档状态：分析 → 证伪 → 替代方案实施完成。gob 协议税已消除（1Reading -85%）。下一步：SHA256 双算 / groupBy 预分配 / 关 log。*

---

*文档状态：分析完成 + 方案实验证伪，修复未实施。下一步：修正后优化清单（SHA256 双算 / groupBy / 关 log）。*

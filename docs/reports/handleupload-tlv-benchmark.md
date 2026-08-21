# handleUpload 手写 TLV 优化：benchmark 证据链

> 本文档独立于 `benchmarks.md`（基线文档），专门记录 **2026-08-19 手写 TLV 优化** 前后的完整采集过程：命令 + 原始输出 + 对比。
>
> 背景：UploadPayload 由 gob 序列化改为手写 TLV（大端序，[Count:4B] + 每条 Reading），消除 gob 独立流协议的类型重编译税（compileDec ~13µs/次）。详见 `handleupload-performance-analysis.md` 第 8 步。

---

## 一、环境

| 项 | 值 |
|---|---|
| CPU | AMD Ryzen 5 9600X 6C12T |
| OS | Windows |
| Go | go1.26.0 windows/amd64 |
| Before 采集日期 | 2026-08-16 23:10（gob 版） |
| After 采集日期 | 2026-08-19 23:04（TLV 版） |

---

## 二、采集命令（可复现）

```bash
# 1. 编译当前代码的测试二进制（-c 保留，供 pprof 符号化；新名字避免覆盖 8/16 的旧 binary）
go test -c -o server-tlv.test.exe ./internal/server/

# 2. 跑 benchmark + 同时采集 CPU 与内存 profile（一条命令）
./server-tlv.test.exe -test.bench=BenchmarkHandleUpload -test.benchmem \
  -test.run=^$ -test.cpuprofile=cpu-tlv.prof -test.memprofile=mem-tlv.prof

# 3. 解析（-cum 看累计时间；-alloc_space 看分配字节）
go tool pprof -top -cum server-tlv.test.exe cpu-tlv.prof
go tool pprof -alloc_space -top server-tlv.test.exe mem-tlv.prof
```

旧版本（gob）的同类采集命令见 `benchmarks.md`，其 8/16 输出用作 Before 基线。

---

## 三、Before：gob 版（2026-08-16 采集，数据引自 benchmarks.md）

```
BenchmarkHandleUpload_1Reading-12                  52440             25091 ns/op           11110 B/op        252 allocs/op
BenchmarkHandleUpload_12Readings-12                36537             35767 ns/op           24425 B/op        342 allocs/op
BenchmarkHandleUpload_100Readings-12               11276            110203 ns/op          127767 B/op        985 allocs/op
BenchmarkHandleUpload_DedupHit-12                 536907              2547 ns/op             444 B/op         25 allocs/op
```

---

## 四、After：手写 TLV 版（2026-08-19 采集）

### 4.1 完整 bench 输出（命令：见第二节第 2 条）

```
goos: windows
goarch: amd64
pkg: LEPG/internal/server
cpu: AMD Ryzen 5 9600X 6-Core Processor
BenchmarkHandleUpload_1Reading-12       	  478808	      2980 ns/op	    1748 B/op	      28 allocs/op
BenchmarkHandleUpload_12Readings-12     	  144466	     11569 ns/op	   13901 B/op	     120 allocs/op
BenchmarkHandleUpload_100Readings-12    	   23750	     54635 ns/op	  110082 B/op	     796 allocs/op
BenchmarkHandleUpload_DedupHit-12       	  811072	      1289 ns/op	     128 B/op	       6 allocs/op
PASS
```

### 4.2 CPU profile top（命令：`go tool pprof -top -cum server-tlv.test.exe cpu-tlv.prof`）

```
Duration: 7.86s, Total samples = 12.48s (158.87%)
      flat  flat%   sum%        cum   cum%
         0     0%     0%         7s 56.09%  testing.(*B).runN
         0     0%     0%      6.92s 55.45%  testing.(*B).launch
     0.08s  0.64%  0.64%      6.41s 51.36%  LEPG/internal/server.handleUpload
     0.05s   0.4%  1.04%      4.67s 37.42%  runtime.systemstack
         0     0%  1.04%      3.05s 24.44%  runtime.gcBgMarkWorker.func2
     0.02s  0.16%  1.20%      3.04s 24.36%  runtime.gcDrain
     0.73s  5.85%  7.05%      2.51s 20.11%  runtime.scanObject
     0.09s  0.72%  7.77%      2.10s 16.83%  LEPG/internal/server.serializeReadings
     0.11s  0.88%  8.65%      1.75s 14.02%  runtime.mallocgc
     0.02s  0.16%  8.81%      1.59s 12.74%  encoding/json.Marshal
     0.01s  0.08%  8.89%      1.56s 12.50%  LEPG/internal/server.BenchmarkHandleUpload_12Readings
     0.01s  0.08%  9.13%      1.41s 11.30%  encoding/json.(*encodeState).reflectValue
     1.01s  8.09% 17.23%      1.41s 11.30%  runtime.tryDeferToSpanScan
     0.02s  0.16% 17.47%      1.38s 11.06%  encoding/json.arrayEncoder.encode
     0.16s  1.28% 18.75%      1.34s 10.74%  encoding/json.structEncoder.encode
```

**要点：gob 相关函数（compileDec / recvType / decodeValue）已从 profile 中完全消失**；CPU 大头转为 ①GC/分配（gcDrain 24.4% + scanObject 20.1% + mallocgc 14.0%）②JSON 序列化（serializeReadings 16.8% + json.* 合计 ~35%）。

### 4.3 内存 profile top（命令：`go tool pprof -alloc_space -top server-tlv.test.exe mem-tlv.prof`）

```
      flat  flat%   sum%        cum   cum%
 2817.22MB 27.02% 27.02%  2817.22MB 27.02%  LEPG/internal/server.groupReadingsByDeviceName (inline)
 1167.18MB 11.19% 38.21%  1167.68MB 11.20%  LEPG/internal/server.(*mockSinker).Send
 1039.22MB  9.97% 48.18%  1658.73MB 15.91%  LEPG/internal/msg.(*UploadPayload).Decode
 1031.94MB  9.90% 58.08%  1912.02MB 18.34%  LEPG/internal/server.makeUploadWithReadings
  966.94MB  9.27% 67.35%   969.47MB  9.30%  encoding/json.Marshal
  792.07MB  7.60% 74.95%   792.07MB  7.60%  LEPG/internal/msg.(*UploadPayload).Encode
  740.48MB  7.10% 82.05%  1953.46MB 18.73%  LEPG/internal/server.serializeReadings
  619.51MB  5.94% 87.99%   619.51MB  5.94%  LEPG/internal/msg.readTLVString
  317.47MB  3.04% 91.03%   317.47MB  3.04%  LEPG/internal/server.(*mockStore).SaveReadings
```

**要点（去噪后）**：`groupReadingsByDeviceName` 以 **27.02% 成为最大真实分配源**（此前被 gob 掩盖）——坐实了优化清单里"groupBy 预分配"的优先级；`readTLVString` 5.94% 是字符串拷贝分配（后续可优化）；`mockSinker/mockStore`、`makeUploadWithReadings` 为 benchmark 噪声（mock 或 setup）。

---

## 五、Before / After 对比

| benchmark | Before (gob 8/16) | After (TLV 8/19) | 变化 |
|---|---|---|---|
| 1Reading | 25,091 ns / 252 allocs / 11,110 B | **2,980 ns / 28 allocs / 1,748 B** | **-88% / -89% / -84%** |
| 12Readings | 35,767 ns / 342 allocs / 24,425 B | **11,569 ns / 120 allocs / 13,901 B** | **-68% / -65% / -43%** |
| 100Readings | 110,203 ns / 985 allocs / 127,767 B | **54,635 ns / 796 allocs / 110,082 B** | **-50% / -19% / -14%** |
| DedupHit | 2,547 ns / 25 allocs / 444 B | **1,289 ns / 6 allocs / 128 B** | **-49% / -76% / -71%** |

**收益随数据量递减（1Reading 最大）——印证固定开销模型**：砍掉的是 gob 编译税（固定成本 F），数据量越大摊得越薄。

> 注意：ns/op 跨 3 天对比存在环境漂移风险（8/16 vs 8/19 系统状态不同）；**allocs/B 是确定性指标，不受漂移影响**，252→28、985→796 无争议。

---

## 六、本次采集暴露的下一步优化点

| 项目 | 证据 | 预估收益 |
|---|---|---|
| groupBy 预分配容量 | mem 27.02% 最大真实分配源（slice 反复 realloc） | 消掉大部分 groupBy 分配 |
| serializeReadings（JSON） | CPU 16.8% + mem 18.7% | 换 fastjson 收益中等，工作量高（不抠） |
| readTLVString 拷贝 | mem 5.94%（string 转换拷贝） | 复用缓冲，中等收益 |
| benchmark 关 log | DedupHit 等路径 log 噪声 | 测纯路径，方法论改进 |

---

*证据文件：`cpu-tlv.prof` / `mem-tlv.prof` / `server-tlv.test.exe`（8/19）；`cpu.prof` / `server.test.exe`（8/16，gob 版，已保留）。*

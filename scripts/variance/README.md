# variance — 方差之方差实验

LEPG server benchmark 的采集与分析工具集，定量回答一个问题：

> 笔记本 benchmark 的误差本身有多不可复现？

## 背景与要解决的问题

LEPG 的 `HandleUpload_*` benchmark 在 Intel i5-12500H（笔记本，P+E 核混合架构）上出现了一个诡异现象：

| 运行 | 1Reading ±% | 12Readings ±% | 100Readings ±% | DedupHit ±% |
|---|---|---|---|---|
| 第一次 | 6% | 6% | 3% | 1% |
| 第二次 | 17% | 8% | 15% | 18% |

同一台机器、同样的电源设置，**误差本身对不上**。benchstat 的 ±% 是 count 次运行的标准差，理论上反映测量稳定性；如果连这个数都随环境（温度、后台进程、P/E 核调度）剧烈波动，那么单次 count=30 的 ±% 就不能代表真实稳定性。

本项目用「方差之方差」实验给这个结论提供定量证据，也是后续博客「笔记本 benchmark 不可复现」的核心素材。

## 实验设计

跑 K 组独立的 benchmark session（每组 `go test -count N`），每组用 benchstat 算各 benchmark 的 ±%；然后观察 **±% 本身** 在 K 组间的分布（min / max / mean / range）。

- 如果 ±% 在组间剧烈波动 → 单次运行的 ±% 不可复现，结论成立
- 附带数据：每组 session 的逐次原始数据，可观察 warmup 漂移（前几次 run 偏高后收敛）

## 目录结构

| 文件 | 作用 |
|---|---|
| collect.py | 数据采集：循环调用 go test + benchstat 落盘，不解析 |
| analysis.ipynb | 分析绘图：读 output/ 下文件，逐次 warmup 观察 + ±% 分布统计 + 出图 |
| output/ | collect.py 生成：session_XX_raw.txt（原始逐次）、session_XX_stat.txt（benchstat 聚合） |
| pyproject.toml | Python 依赖（pandas / matplotlib / jupyter） |

## 用法

### 1. 采集数据

```bash
# 在 scripts/variance/ 下，用项目虚拟环境
.venv/Scripts/python collect.py                # 默认 10 sessions x count=30
.venv/Scripts/python collect.py -k 3 -n 5      # 快速验证: 3 sessions x count=5
.venv/Scripts/python collect.py --benchtime 100x   # 指定 benchtime（默认 1s）
```

参数：

| 参数 | 默认 | 说明 |
|---|---|---|
| -k / --sessions | 10 | 跑多少组 session |
| -n / --count | 30 | 每组 go test -count |
| --benchtime | (空=1s) | 如 "100x"、"500ms" |

注意：默认配置（10 x 30）在笔记本上耗时较长，预计 30-60 分钟，建议后台运行。

### 2. 分析

```bash
.venv/Scripts/python -m jupyter notebook analysis.ipynb   # 或 jupyter lab
```

按顺序执行所有 cell：先验证 output/ 存在，再解析原始数据、观察 warmup 漂移、统计 ±% 分布并绘图。

## 设计约定

- **采集与分析分离**：collect.py 只调命令落盘、不做任何解析；notebook 负责读文件、分析、绘图。数据一次采集可反复分析，改图不改数据。
- **路径自定位**：collect.py 用脚本位置（`Path(__file__).resolve().parents[2]`）定位 LEPG 根目录，不依赖执行目录，项目换路径无需改代码；启动时校验 go.mod 存在，脚本被挪位置会立即报错。
- **不重复造 benchstat**：±% 直接由 benchstat 计算，脚本只做解析和统计。

## 已知观察（采集前的背景数据）

- 内存指标（B/op、allocs/op）在所有运行中均 ±0%，说明 benchmark setup 确定性高，波动纯粹来自时间测量
- 时间 ±% 受 P/E 核调度影响显著，Intel 笔记本上连两次运行的 ±% 都对不上
- AMD 台式机（全大核、锁频）上 ±26% 但均值未偏（warmup 与稳定区抵消），说明低方差 ≠ 高准确度

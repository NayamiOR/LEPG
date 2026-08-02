#!/usr/bin/env python
"""
数据采集：方差之方差实验
========================
循环调用两个命令，把输出落盘，不做任何解析：

1. go test -bench . -benchmem -count N  →  output/session_XX_raw.txt   (逐次原始数据)
2. benchstat output/session_XX_raw.txt  →  output/session_XX_stat.txt  (benchstat 聚合表)

剩下的交给 notebook 读文件分析。

用法:
    python collect.py                     # 默认 10 sessions x count=30
    python collect.py -k 3 -n 5          # 快速测试: 3 sessions x count=5
"""

import subprocess
import time
import argparse
from pathlib import Path

# ═══════════════════ 配置 ═══════════════════
# collect.py 位于 LEPG/scripts/variance/ 下，上推两级即 LEPG 根目录
LEPG_DIR   = Path(__file__).resolve().parents[2]
if not (LEPG_DIR / "go.mod").exists():
    raise SystemExit(f"找不到 LEPG 根目录 (go.mod): {LEPG_DIR}，脚本位置变了？")

PKG        = "LEPG/internal/server"

SCRIPT_DIR = Path(__file__).resolve().parent
OUT_DIR    = SCRIPT_DIR / "output"
# ═════════════════════════════════════════════


def run_one_session(idx, count, benchtime):
    """跑一组 session: go test 落盘 + benchstat 落盘"""
    # ── 命令 1: go test ──
    go_cmd = ["go", "test", "-bench", ".", "-benchmem",
              "-count", str(count)]
    if benchtime:
        go_cmd += ["-benchtime", benchtime]
    go_cmd.append(PKG)

    t0 = time.time()
    r = subprocess.run(go_cmd, cwd=str(LEPG_DIR),
                       capture_output=True, text=True,
                       encoding="utf-8", errors="replace",
                       timeout=900)
    elapsed = time.time() - t0

    raw_file = OUT_DIR / f"session_{idx:02d}_raw.txt"
    raw_file.write_text(r.stdout, encoding="utf-8")
    if r.returncode != 0 and r.stderr:
        (OUT_DIR / f"session_{idx:02d}_err.txt").write_text(r.stderr, encoding="utf-8")

    # ── 命令 2: benchstat ──
    stat_file = OUT_DIR / f"session_{idx:02d}_stat.txt"
    sr = subprocess.run(["benchstat", str(raw_file)],
                        capture_output=True, text=True,
                        encoding="utf-8", errors="replace")
    stat_file.write_text(sr.stdout, encoding="utf-8")

    print(f"  session {idx}: {elapsed:.0f}s  |  raw={raw_file.name}, stat={stat_file.name}")
    return raw_file, stat_file


def main():
    p = argparse.ArgumentParser(description="方差之方差数据采集 (纯命令驱动)")
    p.add_argument("-k", "--sessions", type=int, default=10,
                   help="跑多少组 session (默认 10)")
    p.add_argument("-n", "--count", type=int, default=30,
                   help="每组 go test -count (默认 30)")
    p.add_argument("--benchtime", type=str, default="",
                   help='benchtime, 如 "100x", "500ms" (默认 1s)')
    args = p.parse_args()

    OUT_DIR.mkdir(parents=True, exist_ok=True)

    print(f"数据采集: {args.sessions} sessions x count={args.count}\n")
    t_start = time.time()

    for i in range(1, args.sessions + 1):
        print(f"[{i}/{args.sessions}] running...")
        run_one_session(i, args.count, args.benchtime)

    elapsed = time.time() - t_start
    print(f"\n总耗时: {elapsed:.0f}s ({elapsed/60:.1f} min)")
    print(f"输出目录: {OUT_DIR}")
    print("下一步: 打开 analysis.ipynb 读文件分析")


if __name__ == "__main__":
    main()

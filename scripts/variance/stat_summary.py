"""从 stat 文件提取 benchstat ±% 分布"""
import re
from pathlib import Path
import pandas as pd

BASE = Path(r"D:\Nayami\Projects\LEPG\scripts\variance")
RE_PCT = re.compile(r'(HandleUpload_\w+)-16\s+(.+?)\s\xb1\s*([\d.]+)%')


def stat_summary(dirname):
    d = BASE / dirname
    stat_pcts = {}
    for f in sorted(d.glob('session_*_stat.txt')):
        sid = int(f.stem.split('_')[1])
        text = f.read_text(encoding='utf-8', errors='replace')
        if 'sec/op' not in text:
            continue
        block = text.split('sec/op')[1]
        for stop in ('B/op', 'allocs/op'):
            i = block.find(stop)
            if i != -1:
                block = block[:i]
                break
        dd = {}
        for line in block.splitlines():
            m = RE_PCT.search(line)
            if m:
                dd[m.group(1).replace('HandleUpload_', '')] = float(m.group(3))
        stat_pcts[sid] = dd
    df = pd.DataFrame.from_dict(stat_pcts, orient='index').sort_index()
    return df


for name, d in [('AMD', 'output'), ('Intel', 'output-intel')]:
    df = stat_summary(d)
    print(f"\n=== {name} benchstat ±% by session ===")
    print(df.round(1).to_string())
    print("\n--- ±% distribution ---")
    summ = df.agg(['min', 'max', 'mean', 'median']).T
    summ['range'] = summ['max'] - summ['min']
    print(summ.round(1).to_string())

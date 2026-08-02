"""分析两套 benchmark 数据: AMD vs Intel 方差之方差"""
import re
from pathlib import Path
import pandas as pd
import numpy as np
from scipy.stats import spearmanr

BASE = Path(r"D:\Nayami\Projects\LEPG\scripts\variance")

RE_BENCH = re.compile(
    r'^(Benchmark\w+-\d+)\s+(\d+)\s+([\d.]+)\s*(ns|\xb5|u|\xb5s|m)?s?/op\s+'
    r'(\d+)\s+B/op\s+(\d+)\s+allocs/op'
)


def parse_dir(dirname):
    d = BASE / dirname
    raw_files = sorted(d.glob('session_*_raw.txt'))
    cpu = ''
    rows = []
    for path in raw_files:
        sid = int(path.stem.split('_')[1])
        lines = path.read_text(encoding='utf-8', errors='replace').splitlines()
        if not cpu:
            for l in lines:
                if l.startswith('cpu:'):
                    cpu = l[4:].strip()
                    break
        run_counter = {}
        for line in lines:
            line = line.strip()
            if not line.startswith('Benchmark'):
                continue
            m = RE_BENCH.match(line)
            if not m:
                continue
            full = m.group(1)
            ns = float(m.group(3))
            unit = m.group(4)
            if unit in ('\xb5', 'u', '\xb5s'):
                ns *= 1000
            elif unit == 'm':
                ns *= 1_000_000
            short = full.rsplit('-', 1)[0].replace('Benchmark', '')
            run_counter[short] = run_counter.get(short, 0) + 1
            rows.append({'session': sid, 'bench': short, 'run': run_counter[short],
                         'ns': ns, 'us': ns / 1000})
    return cpu, pd.DataFrame(rows)


amd_cpu, amd = parse_dir('output')
intel_cpu, intel = parse_dir('output-intel')
print("AMD:", amd_cpu, "| sessions:", amd['session'].nunique(), "| rows:", len(amd))
print("Intel:", intel_cpu, "| sessions:", intel['session'].nunique(), "| rows:", len(intel))

order = ['HandleUpload_1Reading', 'HandleUpload_12Readings',
         'HandleUpload_100Readings', 'HandleUpload_DedupHit']
short = ['1Reading', '12Readings', '100Readings', 'DedupHit']


def analyze(name, df):
    print(f"\n{'='*72}\n{name}")
    print(f"{'bench':<14} {'CV min':>7} {'CV max':>7} {'CV mean':>8} | "
          f"{'med_min':>9} {'med_max':>9} {'drift%':>8} | {'rho':>6} {'p':>6}")
    for b, s in zip(order, short):
        sub = df[df['bench'] == b]
        grp = sub.groupby('session')['ns']
        cv = grp.std() / grp.mean() * 100
        med = sub.groupby('session')['us'].median()
        drift = (med.max() - med.min()) / med.min() * 100
        rho, p = spearmanr(med.index, med.values)
        print(f"{s:<14} {cv.min():>6.1f}% {cv.max():>6.1f}% {cv.mean():>7.1f}% | "
              f"{med.min():>8.2f} {med.max():>8.2f} {drift:>7.1f}% | {rho:>+5.2f} {p:>5.3f}")
    print("-- overall medians (µs):")
    for b, s in zip(order, short):
        print(f"   {s:<14} {df[df['bench']==b]['us'].median():.2f}")


analyze("AMD Ryzen 7 8845H", amd)
analyze("Intel i5-12500H", intel)

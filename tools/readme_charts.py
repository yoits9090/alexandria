#!/usr/bin/env python3
"""Render README benchmark charts from docs/bench/bench.json.

Outputs (into docs/images/):
  hero-tokens.png        average tokens per query: raw vs Alexandria compact
  tokens-per-query.png   per-query grouped bars: raw / alexandria json / toon
  tokens-wasted.png      tokens wasted per query with savings labels
"""
import json
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np

ROOT = Path(__file__).resolve().parents[1]
BENCH = json.loads((ROOT / "docs" / "bench" / "bench.json").read_text())

RAW = "#94a3b8"    # slate-400
JSON = "#38bdf8"   # sky-400
TOON = "#0d9488"   # teal-600
WASTE = "#fb7185"  # rose-400
INK = "#1e293b"    # slate-800

plt.rcParams.update({
    "font.size": 12.5,
    "axes.edgecolor": "#cbd5e1",
    "axes.labelcolor": INK,
    "text.color": INK,
    "xtick.color": INK,
    "ytick.color": INK,
    "axes.grid": True,
    "grid.color": "#e2e8f0",
    "grid.linewidth": 0.8,
    "figure.facecolor": "white",
    "axes.facecolor": "white",
})

queries = [r["query"] for r in BENCH]
raw = np.array([r["raw_tokens"] for r in BENCH])
jsn = np.array([r["json_tokens"] for r in BENCH])
toon = np.array([r["toon_tokens"] for r in BENCH])
wasted = raw - toon
pct = 100 * (1 - toon.sum() / raw.sum())

def short(q):
    w = q.split()
    return w[0] if len(w) == 1 else " ".join(w[:2])

out = ROOT / "docs" / "images"
out.mkdir(parents=True, exist_ok=True)

# ---- hero: average tokens per query ---------------------------------------
fig, ax = plt.subplots(figsize=(8.2, 4.6), dpi=160)
labels = ["Raw provider payload", "Alexandria compact"]
vals = [raw.mean(), toon.mean()]
bars = ax.bar(labels, vals, width=0.52, color=[RAW, TOON], zorder=3)
for b, v in zip(bars, vals):
    ax.text(b.get_x() + b.get_width() / 2, v + raw.max() * 0.035, f"{v:.0f}",
            ha="center", fontsize=18, fontweight="bold", color=INK)
ax.set_ylim(0, raw.max() * 1.28)
ax.set_ylabel("tokens per query (rune/4 estimate)")
ax.set_title("Average prompt tokens per search query", fontsize=16, fontweight="bold", pad=14)
ax.text(1, vals[1] + raw.max() * 0.16, f"\u2212{pct:.0f}%", ha="center",
        fontsize=26, fontweight="bold", color=TOON)
ax.spines[["top", "right"]].set_visible(False)
ax.tick_params(axis="x", length=0)
fig.tight_layout()
fig.savefig(out / "hero-tokens.png", bbox_inches="tight")
plt.close(fig)

# ---- per-query grouped bars ------------------------------------------------
fig, ax = plt.subplots(figsize=(11.5, 4.9), dpi=160)
x = np.arange(len(queries))
w = 0.27
b1 = ax.bar(x - w, raw, w, label="Raw provider JSON", color=RAW, zorder=3)
b2 = ax.bar(x, jsn, w, label="Alexandria JSON", color=JSON, zorder=3)
b3 = ax.bar(x + w, toon, w, label="Alexandria compact", color=TOON, zorder=3)
for rect in b3:
    ax.text(rect.get_x() + rect.get_width() / 2, rect.get_height() + 18, f"{int(rect.get_height())}",
            ha="center", fontsize=9.5, color=TOON, fontweight="bold")
ax.set_xticks(x)
ax.set_xticklabels([short(q) for q in queries], rotation=22, ha="right", fontsize=10.5)
ax.set_ylabel("tokens (rune/4 estimate)")
ax.set_ylim(0, raw.max() * 1.22)
ax.set_title("Prompt tokens per search query", fontsize=15, fontweight="bold", pad=12)
ax.legend(frameon=False, loc="upper right", fontsize=11)
ax.spines[["top", "right"]].set_visible(False)
ax.tick_params(axis="x", length=0)
fig.tight_layout()
fig.savefig(out / "tokens-per-query.png", bbox_inches="tight")
plt.close(fig)

# ---- wasted tokens ----------------------------------------------------------
fig, ax = plt.subplots(figsize=(11.5, 4.9), dpi=160)
bars = ax.bar(x, wasted, w * 1.6, color=WASTE, zorder=3)
for rect, p in zip(bars, pct_per := 100 * (1 - toon / raw)):
    ax.text(rect.get_x() + rect.get_width() / 2, rect.get_height() + 14, f"\u2212{p:.0f}%",
            ha="center", fontsize=10.5, fontweight="bold", color=WASTE)
ax.set_xticks(x)
ax.set_xticklabels([short(q) for q in queries], rotation=22, ha="right", fontsize=10.5)
ax.set_ylabel("tokens wasted per query")
ax.set_ylim(0, wasted.max() * 1.3)
ax.set_title("Tokens Alexandria saves per query (raw payload \u2212 compact output)",
             fontsize=15, fontweight="bold", pad=12)
ax.text(0.985, 0.92, f"{wasted.sum():,} tokens saved across {len(queries)} queries \u00b7 {pct:.0f}% of every prompt",
        transform=ax.transAxes, ha="right", fontsize=11.5, color=INK,
        bbox=dict(boxstyle="round,pad=0.45", facecolor="#f0fdfa", edgecolor=TOON, linewidth=1.2))
ax.spines[["top", "right"]].set_visible(False)
ax.tick_params(axis="x", length=0)
fig.tight_layout()
fig.savefig(out / "tokens-wasted.png", bbox_inches="tight")
plt.close(fig)

print("charts written to", out)

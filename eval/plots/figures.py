#!/usr/bin/env python3
# Copyright Project Harbor Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Figures for the paper (implementation plan S9), read from the records.

    python3 eval/plots/figures.py RESULTS_DIR [--baseline BASELINE_DIR]

reads RESULTS_DIR/w0.jsonl, w1.jsonl and w2.jsonl and writes PDF and PNG
figures to RESULTS_DIR/figures/. The records hold G1–G6; the gate figure also
shows G7 when RESULTS_DIR/lint-gates.jsonl exists, written by
`sleeko-eval analyze -lint-gates RESULTS_DIR/lint-gates.jsonl ...`. The numbers match `sleeko-eval report`: the
last record of each attempt counts, harness errors and invalid items are left
out, and an image is solved when one of its repetitions succeeds (pass@3).

With --baseline, every figure also shows the rule-based baseline from
BASELINE_DIR/baseline.jsonl (eval/runs/baseline.sh), and its G7 from
BASELINE_DIR/lint-gates.jsonl when present. It is drawn last, apart from the
pipelines and in a neutral colour: it is a reference point, measured once per
image and in its own session, so its image count can differ.
Requirements: eval/plots/requirements.txt.
"""

import argparse
import json
import math
import os
from collections import defaultdict

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402

PIPELINES = [("w0", "W0"), ("w1", "W0+W1"), ("w2", "W0+W1+W2")]

# Colours: the first three categorical slots of the dataviz reference palette,
# validated all-pairs on white (CVD ΔE ≥ 9.2, normal vision ≥ 24.0). Aqua is
# below 3:1 contrast on white, so every series also has a marker shape and a
# legend, and the reports are the table view.
SERIES = ["#2a78d6", "#eb6834", "#1baf7a"]
MARKERS = ["o", "s", "^"]
INK, INK2, MUTED, GRID, BASELINE = "#0b0b0b", "#52514e", "#898781", "#e1e0d9", "#c3c2b7"

# The rule-based baseline is a reference point, not a fourth pipeline: it is
# drawn in ink rather than a series colour, with its own marker.
REFERENCE = ("baseline", "Baseline")
REF_COLOUR, REF_MARKER = INK2, "D"


# With the baseline in a figure, "solved" means pass@3 for the pipelines and
# the single attempt for the baseline; the legend title says so.
SOLVED_NOTE = "solved: pass@3 for the pipelines, one attempt per project for the baseline"


def columns(data):
    """The pipelines in data, then the baseline when it is there."""
    return [c for c in PIPELINES + [REFERENCE] if c[0] in data]


def style(i):
    """Colour and marker of column i: a series slot, or the baseline's."""
    if i < len(PIPELINES):
        return SERIES[i], MARKERS[i]
    return REF_COLOUR, REF_MARKER


def separate(ax, data):
    """A dotted line between the pipelines and the baseline."""
    if REFERENCE[0] in data:
        ax.axvline(len(PIPELINES) - 0.5, color=BASELINE, lw=0.8, ls=":", zorder=1)

plt.rcParams.update({
    "font.family": "sans-serif",
    "font.sans-serif": ["Helvetica", "Arial", "DejaVu Sans"],
    "font.size": 9,
    "axes.edgecolor": BASELINE,
    "axes.labelcolor": INK2,
    "axes.titlecolor": INK,
    "axes.titlesize": 10,
    "axes.titleweight": "bold",
    "axes.titlelocation": "left",
    "xtick.color": MUTED,
    "ytick.color": MUTED,
    "xtick.labelcolor": INK2,
    "ytick.labelcolor": INK2,
    "axes.spines.top": False,
    "axes.spines.right": False,
    "axes.grid": True,
    "grid.color": GRID,
    "grid.linewidth": 0.6,
    "legend.frameon": False,
    "legend.labelcolor": INK2,
    "figure.facecolor": "white",
    "savefig.facecolor": "white",
})


def load(path):
    last = {}
    with open(path) as f:
        for line in f:
            if line.strip():
                r = json.loads(line)
                last[(r["item"], r["repetition"])] = r
    return [r for r in last.values() if r["status"] in ("measured", "optimizer_failed")]


def load_lint(path):
    """G7 per attempt from `sleeko-eval analyze -lint-gates`, or None."""
    if not os.path.exists(path):
        return None
    with open(path) as f:
        return {(g["pipeline"], g["item"], g["repetition"]): g["status"] for g in map(json.loads, f) if g}


def wilson(k, n, z=1.959964):
    if n == 0:
        return 0.0, 1.0
    p = k / n
    d = 1 + z * z / n
    c = p + z * z / (2 * n)
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n))
    return (c - h) / d, (c + h) / d


def solved(records):
    per = defaultdict(list)
    for r in records:
        per[r["item"]].append(r["success"])
    k = sum(any(v) for v in per.values())
    return k, len(per)


def save(fig, out, name):
    for ext in ("pdf", "png"):
        fig.savefig(os.path.join(out, f"{name}.{ext}"), dpi=300, bbox_inches="tight")
    plt.close(fig)


def fig_success(data, out):
    """Fig. 1: solved (pass@3) with Wilson 95% CI, and the per-attempt rate."""
    cols = columns(data)
    fig, ax = plt.subplots(figsize=(4.8 if len(cols) == len(PIPELINES) else 5.6, 3.0))
    xs = range(len(cols))
    for i, (key, name) in enumerate(cols):
        recs = data[key]
        k, n = solved(recs)
        lo, hi = wilson(k, n)
        p = k / n
        ref = REFERENCE[0] in data
        ax.errorbar(i - 0.1, p, yerr=[[p - lo], [hi - p]], fmt=MARKERS[0], color=SERIES[0], ms=7,
                    elinewidth=1.5, capsize=4,
                    label=("solved, 95% CI" if ref else "solved (pass@3), 95% CI") if i == 0 else None)
        ax.annotate(f"{k}/{n}", (i - 0.1, p), xytext=(-8, 0), textcoords="offset points",
                    ha="right", va="center", color=INK2, fontsize=8)
        att = sum(r["success"] for r in recs) / len(recs)
        ax.plot(i + 0.1, att, MARKERS[1], color=SERIES[1], ms=7, label="per attempt" if i == 0 else None)
    ax.set_xticks(list(xs), [name for _, name in cols])
    ax.set_xlim(-0.5, len(cols) - 0.5)
    separate(ax, data)
    ax.set_ylim(0, None)
    ax.yaxis.set_major_formatter(matplotlib.ticker.PercentFormatter(1.0, decimals=0))
    ax.set_ylabel("successful")
    ax.grid(axis="x", visible=False)
    handles, labels = ax.get_legend_handles_labels()
    order = sorted(range(len(labels)), key=lambda i: not labels[i].startswith("solved"))
    ax.legend([handles[i] for i in order], [labels[i] for i in order],
              loc="lower left", bbox_to_anchor=(0, 1.0), ncol=2, borderaxespad=0.2,
              title=SOLVED_NOTE if REFERENCE[0] in data else None,
              title_fontproperties={"size": 8}, alignment="left").get_title().set_color(INK2)
    save(fig, out, "fig1-success")


def fig_components(data, out):
    """Fig. 2: ΔV and ΔS of the successful attempts, every attempt a dot."""
    cols = columns(data)
    fig, axes = plt.subplots(1, 2, figsize=(6.4 if len(cols) == len(PIPELINES) else 7.4, 3.0))
    for ax, field, title in ((axes[0], "delta_v", "ΔV: vulnerability reduction"),
                             (axes[1], "delta_s", "ΔS: size reduction")):
        for i, (key, name) in enumerate(cols):
            vals = sorted(r["components"][field] for r in data[key]
                          if r["success"] and r.get("components"))
            jitter = [((j % 5) - 2) * 0.045 for j in range(len(vals))]
            colour, marker = style(i)
            ax.scatter([i + d for d in jitter], vals, s=22, color=colour, marker=marker,
                       edgecolors="white", linewidths=0.6, zorder=3)
            if vals:
                m = vals[len(vals) // 2] if len(vals) % 2 else (vals[len(vals) // 2 - 1] + vals[len(vals) // 2]) / 2
                ax.plot([i - 0.25, i + 0.25], [m, m], color=INK, lw=1.5, zorder=4)
                ax.annotate(f"n = {len(vals)}", (i, 1.0), xycoords=("data", "axes fraction"),
                            ha="center", va="bottom", color=MUTED, fontsize=7.5)
        ax.axhline(0, color=BASELINE, lw=0.8, zorder=1)
        ax.set_xticks(range(len(cols)), [name for _, name in cols], fontsize=8)
        ax.set_xlim(-0.6, len(cols) - 0.4)
        separate(ax, data)
        ax.yaxis.set_major_formatter(matplotlib.ticker.PercentFormatter(1.0, decimals=0))
        ax.grid(axis="x", visible=False)
        ax.set_title(title, pad=14)
    axes[0].set_ylabel("reduction relative to O′")
    fig.tight_layout()
    save(fig, out, "fig2-components")


GATES = [("G1_parse", "G1 parses"), ("G2_no_invention", "G2 no invented sources"),
         ("G3_versions_exist", "G3 tags/versions exist"), ("G4_build", "G4 builds"),
         ("G5_behaviour", "G5 behaves the same"), ("G6_no_new_severe", "G6 no new severe CVE")]


def fig_gates(data, out, lint=None):
    """Fig. 3: share of attempts failing each gate, per pipeline."""
    fig, ax = plt.subplots(figsize=(5.6, 3.2 if REFERENCE[0] not in data else 3.6))
    rows = GATES + ([("G7_no_new_lint_errors", "G7 no new lint error")] if lint else []) \
        + [("optimizer", "no candidate (LLM error or timeout)")]
    cols = columns(data)
    for i, (key, name) in enumerate(cols):
        recs = data[key]
        shares = []
        for gid, _ in rows:
            if gid == "optimizer":
                n = sum(r["status"] == "optimizer_failed" for r in recs)
            elif gid == "G7_no_new_lint_errors":
                n = sum(lint.get((r["pipeline"], r["item"], r["repetition"])) == "fail" for r in recs)
            else:
                n = sum(any(g["id"] == gid and g["status"] == "fail" for g in r.get("gates") or []) for r in recs)
            shares.append(n / len(recs))
        step = 0.22 if len(cols) == 3 else 0.17
        ys = [len(rows) - 1 - j + ((len(cols) - 1) / 2 - i) * step for j in range(len(rows))]
        colour, marker = style(i)
        ax.scatter(shares, ys, s=40 if i < len(PIPELINES) else 30, color=colour, marker=marker,
                   edgecolors="white", linewidths=0.8, zorder=3, label=name)
    ax.set_yticks(range(len(rows)), [label for _, label in reversed(rows)])
    ax.xaxis.set_major_formatter(matplotlib.ticker.PercentFormatter(1.0, decimals=0))
    ax.set_xlim(-0.015, None)
    ax.set_ylim(-0.6, len(rows) - 0.4)
    ax.set_xlabel("attempts failing the gate")
    ax.grid(axis="y", visible=False)
    ax.legend(loc="lower right")
    save(fig, out, "fig3-gates")


def fig_eol(data, out):
    """Fig. 4: solved per pipeline, split by whether O′ is on an end-of-life release."""
    cols = columns(data)
    fig, ax = plt.subplots(figsize=(4.8 if len(cols) == len(PIPELINES) else 5.6, 3.0))
    groups = [(True, "O′ end of life"), (False, "O′ supported")]
    for gi, (flag, label) in enumerate(groups):
        for i, (key, _) in enumerate(cols):
            recs = [r for r in data[key] if (r.get("original") or {}).get("eosl") is flag]
            k, n = solved(recs)
            if n == 0:
                continue
            lo, hi = wilson(k, n)
            p = k / n
            x = i + (gi - 0.5) * 0.2
            ax.errorbar(x, p, yerr=[[p - lo], [hi - p]], fmt=MARKERS[gi], color=SERIES[gi], ms=7,
                        elinewidth=1.5, capsize=4, label=label if i == 0 else None)
            ax.annotate(f"{k}/{n}", (x, hi), xytext=(0, 3), textcoords="offset points",
                        ha="center", va="bottom", color=INK2, fontsize=7.5)
    ax.set_xticks(range(len(cols)), [name for _, name in cols])
    ax.set_xlim(-0.5, len(cols) - 0.5)
    separate(ax, data)
    ax.set_ylim(0, None)
    ax.yaxis.set_major_formatter(matplotlib.ticker.PercentFormatter(1.0, decimals=0))
    ref = REFERENCE[0] in data
    ax.set_ylabel("images solved, 95% CI" if ref else "images solved (pass@3), 95% CI")
    ax.grid(axis="x", visible=False)
    if ref:
        # Above the plot, as in Fig. 1: the note is wider than the free corner.
        leg = ax.legend(loc="lower left", bbox_to_anchor=(0, 1.0), ncol=2, borderaxespad=0.2,
                        title=SOLVED_NOTE, title_fontproperties={"size": 8}, alignment="left")
        leg.get_title().set_color(INK2)
    else:
        ax.legend(loc="upper left")
    save(fig, out, "fig4-eol")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("results", metavar="RESULTS_DIR")
    ap.add_argument("--baseline", metavar="BASELINE_DIR",
                    help="also draw the rule-based baseline from BASELINE_DIR/baseline.jsonl")
    args = ap.parse_args()
    results = args.results
    data = {key: load(os.path.join(results, f"{key}.jsonl")) for key, _ in PIPELINES}
    lint = load_lint(os.path.join(results, "lint-gates.jsonl"))
    if args.baseline:
        data[REFERENCE[0]] = load(os.path.join(args.baseline, "baseline.jsonl"))
        ref_lint = load_lint(os.path.join(args.baseline, "lint-gates.jsonl"))
        if lint is not None and ref_lint is not None:
            lint.update(ref_lint)
    out = os.path.join(results, "figures")
    os.makedirs(out, exist_ok=True)
    fig_success(data, out)
    fig_components(data, out)
    fig_gates(data, out, lint)
    fig_eol(data, out)
    print(f"figures written to {out}")


if __name__ == "__main__":
    main()

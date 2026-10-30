# Reproducing the evaluation

This guide reproduces the evaluation of the TDK paper *Automated Generation of
Optimized Dockerfiles in Harbor* (BME VIK, 2026): the W0, W0+W1 and W0+W1+W2
pipelines on the benchmark in `eval/benchmark/v1.yaml`, the rule-based
baseline for W2, the reports, the statistical analyses and the figures. Everything below runs from this
branch (`pipeline`).

## The paper's results

Everything the paper reports was computed from the files in
[`eval/results/`](results/README.md): the records of every attempt, the
scans of every image, the run logs, the reports and analyses, and the
figures. They can be recreated from these files directly, without running
the study again; the commands of [step 6](#6-recreate-the-papers-results)
reproduce every report, analysis and figure byte for byte.

## Requirements

- Go 1.26 (see `go.mod`) and Docker with `buildx`. Give Docker at least 8 GB
  of memory, or run with `CONCURRENCY=1`.
- A Docker Hub login (`docker login`): base images are pulled and listed with
  it, since anonymous pulls are rate limited.
- An OpenAI-compatible chat completion endpoint serving the model. The paper
  used Qwen3.8-27B (`qwen3.8-27b-fp16`) through CERN's LLM gateway.
- Python 3.9–3.12 for the figures, with
  `pip install -r eval/plots/requirements.txt` (the pinned NumPy has no
  release for newer Pythons).
- Network access to GitHub (the benchmark projects are cloned at the recorded
  commits), and enough free disk for the images of 56 projects and the
  Trivy cache (several GB).

Trivy (`aquasec/trivy:0.75.0`) and Hadolint (`hadolint/hadolint:v2.12.0`) run
from their pinned container images; nothing else needs to be installed.

## 1. Download the vulnerability database

```sh
go build -o bin/sleeko-eval ./cmd/sleeko-eval
bin/sleeko-eval db
```

The database is downloaded once and then frozen: every scan of a study uses
the same version, which is recorded in every result. It is kept, with the
checkouts and caches, in the default working directory
(`~/Library/Caches/sleeko-eval` on macOS, `~/.cache/sleeko-eval` on Linux),
which the run scripts also use. If a database is already there, `db` keeps
it; `bin/sleeko-eval db -replace` downloads a new one.

## 2. Run the study

```sh
export LLMGW_API_BASE_URL=https://<gateway>/chat/completions
export LLMGW_API_KEY=<key>
export LLMGW_MODEL=<model>
eval/runs/study.sh RESULTS_DIR
```

Run it from a committed tree, so that the code version recorded with the
results names a commit. `study.sh` runs `eval/runs/w0.sh`, `w1.sh` and
`w2.sh` (3 repetitions each,
temperature 0.7, seed 1000 + repetition, a 10-minute build limit) against one
snapshot of the base images, then runs them once more to retry harness errors.
The paper's study took about 6.5 hours. Running the script again resumes it.

It writes, for each pipeline `wN`:

| File | Content |
|---|---|
| `wN.jsonl` | one record per attempt: both Dockerfiles, every gate with its reason, the measurements, model, seed and prompt hash |
| `wN.runs.jsonl` | what produced the run: code commit, model, full prompt, benchmark checksum, Trivy and database versions, flags |
| `wN-scans/` | the full scan of every image referenced by the records |
| `wN-report.md` | success rates with Wilson intervals, ΔV, ΔS, fix rate, gate failures |
| `wN-failures.txt` | the failed attempts with their diffs |

and the pairwise comparisons `w0-vs-w1.md`, `w1-vs-w2.md` and `w0-vs-w2.md`
(McNemar and Wilcoxon).

## 3. Run the rule-based baseline

```sh
eval/runs/baseline.sh BASELINE_DIR RESULTS_DIR/w2.jsonl
```

The baseline (pipeline `baseimage,rebase`, named `Rebase` in the records)
moves the final stage to the best candidate of the base image table and
changes nothing else. It makes no model call, so it needs no gateway. It does
not measure the tables again: it replays the ones recorded in `w2.jsonl`, so
it faces exactly the choices W0+W1+W2 had, and a registry rate limit cannot
change them. It is deterministic, so it runs once per project, and the script
runs twice to retry harness errors, as `study.sh` does. The paper ran it a
week after the study, into its own directory; it took about 75 minutes.
Like the study, run it from a committed tree.

It writes `baseline.jsonl`, `baseline.runs.jsonl` (which also records the
replayed file and its checksum), `baseline-scans/`, `baseline-report.md` and
`baseline-failures.txt`, as in step 2.

## 4. Analyses and figures

```sh
bin/sleeko-eval analyze -lint-gates RESULTS_DIR/lint-gates.jsonl \
  -out RESULTS_DIR/analysis.md \
  RESULTS_DIR/w0.jsonl RESULTS_DIR/w1.jsonl RESULTS_DIR/w2.jsonl
bin/sleeko-eval analyze -pipelines Rebase \
  -lint-gates BASELINE_DIR/lint-gates.jsonl \
  -out BASELINE_DIR/analysis.md BASELINE_DIR/baseline.jsonl
python3 eval/plots/figures.py RESULTS_DIR --baseline BASELINE_DIR
```

`analyze` computes G7 (no new lint errors) with the pinned Hadolint, success
under alternative definitions, the G3 tests and the G5 audit. It first checks
that recomputing the recorded success reproduces it exactly. The first run
lints every Dockerfile with the Hadolint container (about 20 minutes);
the findings are cached, so later runs take seconds. `figures.py` writes the
paper's figures to `RESULTS_DIR/figures/`; with `--baseline`, every figure
also shows the baseline next to the pipelines.

## 5. Scanner visibility and cost

```sh
bin/sleeko-eval inventory -manifest eval/benchmark/v1.yaml \
  -items ITEM,ITEM,... -out INVENTORY_DIR \
  -trivy-db "trivy 0.75.0 db 2026-10-20T07:17:35.368599961Z"
bin/sleeko-eval visibility -inventory INVENTORY_DIR -out visibility.md \
  RESULTS_DIR/w0.jsonl RESULTS_DIR/w1.jsonl RESULTS_DIR/w2.jsonl
bin/sleeko-eval prompts -out calls.jsonl \
  RESULTS_DIR/w0.jsonl RESULTS_DIR/w1.jsonl RESULTS_DIR/w2.jsonl
python3 eval/plots/tokens.py calls.jsonl cost.md
```

G6 counts a critical or high vulnerability in C that O′'s scan does not
report, but the study's scans list only vulnerable packages, so they cannot
tell a package C added from one O′ already had that the scanner did not flag
there (an end-of-life release no longer receives advisories). `inventory`
rebuilds O′ of the given items exactly as the run built it and lists every
package in it, with the same scanner and database; it refuses any other
database version. `visibility` then classifies every new critical or high
finding behind a G6 failure (in a package O′ already had at the same or an
older version, in a package O′ does not have, or in one O′ has at a newer
version), recomputes success with G6 counting only findings in packages new
to C, and compares each rebuilt O′ with the study's O′ scan. Without
`-inventory`, it bounds the effect from the scans alone. The paper rebuilt
the 28 projects with a G6 failure in any pipeline, on 29 October 2026.

`prompts` reconstructs every model call of every attempt from the records,
the run info and O′'s saved scan, with the code and settings of the run;
`tokens.py` counts their tokens with the study model's tokenizer
(requirements: `eval/plots/requirements-tokens.txt`) and adds the latency
and number of calls from the records. The first request and the final answer
of an attempt are exact; a repair request resends the conversation with the
final answer standing in for the earlier ones.

## 6. Recreate the paper's results

With the committed results, the paper's outputs are recreated without
running anything against a registry or a model:

```sh
D=eval/results
bin/sleeko-eval report --out OUT/w2-report.md $D/2026-10-21/w2.jsonl          # and w0, w1, the baseline
bin/sleeko-eval compare --a W0 --b W0+W1+W2 --out OUT/w0-vs-w2.md \
  $D/2026-10-21/w0.jsonl $D/2026-10-21/w2.jsonl
bin/sleeko-eval analyze -lint-gates OUT/lint-gates.jsonl -out OUT/analysis.md \
  $D/2026-10-21/w0.jsonl $D/2026-10-21/w1.jsonl $D/2026-10-21/w2.jsonl
bin/sleeko-eval visibility -inventory $D/2026-10-29-inventory -out OUT/visibility.md \
  $D/2026-10-21/w0.jsonl $D/2026-10-21/w1.jsonl $D/2026-10-21/w2.jsonl
bin/sleeko-eval prompts -out OUT/calls.jsonl \
  $D/2026-10-21/w0.jsonl $D/2026-10-21/w1.jsonl $D/2026-10-21/w2.jsonl
python3 eval/plots/tokens.py OUT/calls.jsonl OUT/cost.md
python3 eval/plots/build_failures.py $D/2026-10-21 --baseline $D/2026-10-28-baseline OUT/build-failures.md
python3 eval/plots/figures.py $D/2026-10-21 --baseline $D/2026-10-28-baseline
```

`build_failures.py` sorts every G4 failure into one cause by fixed rules,
the first that matches its error, or, where the record's tail of the build
log cuts the error off, the step that failed; a step that runs several kinds
of command is then left unclear. It lists every failure with the line it was
classified by, so each decision can be checked.

Each output is identical to its committed copy; `analyze` needs the pinned
Hadolint container for G7 (its first run takes about 20 minutes), and
`figures.py` rewrites `eval/results/2026-10-21/figures/`, the paper's
figures.

## Where the paper's numbers come from

| In the paper | File |
|---|---|
| Success rates (Table 5 and its figure) | `wN-report.md`, `figures/fig1-success` |
| Pairwise tests | `w0-vs-w1.md`, `w1-vs-w2.md`, `w0-vs-w2.md` |
| Gate failures and G7 | `wN-report.md`, `analysis.md`, `figures/fig3-gates` |
| Why candidates do not build (Table 6) | `build-failures.md` |
| Verification and repair, base image tables | the W1 and W2 sections of `w1-report.md`, `w2-report.md` |
| End-of-life split | `wN-report.md`, `figures/fig4-eol` |
| ΔV and ΔS of the successful attempts | `wN-report.md`, `figures/fig2-components` |
| Invented tags and versions, sensitivity analysis | `analysis.md` |
| Rule-based baseline (Table 7 and the figures) | `baseline-report.md`, `BASELINE_DIR/analysis.md`, `baseline.jsonl` compared with `w0.jsonl`–`w2.jsonl` on the projects both measured |
| Scanner visibility of G6 failures | `visibility.md`, `visibility-baseline.md` (with the inventories) |
| Latency, model calls and tokens per proposal | `cost.md` |

## What will differ

The procedure is fixed, but a new run will not give exactly the paper's
numbers:

- base image tags move to newer images, so the rebuilt originals change;
- `sleeko-eval db` downloads the current vulnerability database, not the one
  of 20 October 2026 that the paper used;
- seeded requests usually return the same answer, but the model server does
  not guarantee it under load.

The benchmark manifest lists 57 projects. One of them,
`d3-unitycatalog-unitycatalog`, did not build within the 10-minute limit in
the paper's study and is reported as an invalid item, so the results cover
56 projects.

The baseline rebuilds the originals in its own session. When it ran, a week
after the study, two more projects could not be measured:
`d3-vogler-free-games-claimer` did not build within the limit, and
`f4d-icemic-search-bakery-moe` no longer passed its smoke test. The paper
therefore compares the baseline with the pipelines on 54 projects. An
original whose build runs out of time while other projects build next to it
can be measured on its own with `sleeko-eval run --items ID --concurrency 1`
and the same flags as the script.

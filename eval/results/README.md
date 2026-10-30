# Results of the paper's study

The data behind every number and figure of the TDK paper *Automated
Generation of Optimized Dockerfiles in Harbor* (BME VIK, 2026). How it was
produced, and how to recreate the outputs from it, is in
[`../REPRODUCE.md`](../REPRODUCE.md).

| Directory | Content |
|---|---|
| `2026-10-21/` | The study, 21 October 2026: W0, W0+W1 and W0+W1+W2, 3 repetitions on 56 projects |
| `2026-10-28-baseline/` | The rule-based baseline, 28 October 2026, on the tables W0+W1+W2 recorded |
| `2026-10-29-inventory/` | Full package lists of O′ rebuilt for the 28 projects with a G6 failure, 29 October 2026 |
| `visibility.md`, `visibility-baseline.md` | G6 failures split by what the scanner could see in O′, with the inventories |
| `visibility-without-inventory.md` | The same, bounded from the study's scans alone |
| `cost.md` | Latency, model calls and tokens of a proposal per pipeline |

In each run directory:

| File | Content |
|---|---|
| `wN.jsonl`, `baseline.jsonl` | One record per attempt: both Dockerfiles, every gate with its reason, the measurements, model, seed, prompt hash and latency |
| `*.runs.jsonl` | What produced the run: code commit, model, full prompt, benchmark checksum, Trivy and database versions, flags |
| `*-scans/` | The scan of every image the records refer to, one `<digest>.json` each |
| `*-report.md`, `*-failures.txt`, `*-failures/` | Reports, and every failed attempt with its diff |
| `w0-vs-w1.md`, `w1-vs-w2.md`, `w0-vs-w2.md` | Pairwise comparisons |
| `analysis.md`, `lint-gates.jsonl` | The analyses (G7, other success definitions, G3, G5) and G7 per attempt |
| `build-failures.md` | G4 failures by cause, per pipeline and for the baseline, with the line each was classified by |
| `run.log` | The run's log |
| `figures/` | The paper's figures |

`2026-10-28-baseline/` also keeps the provenance of two projects, whose
originals did not build within the limit while other projects built next to
them and were measured on their own: `baseline.before-retry.jsonl` is the
run before, `retry.jsonl`, `retry.runs.jsonl` and `retry.log` the separate
measurement, and `baseline.jsonl` the merged records the paper uses.

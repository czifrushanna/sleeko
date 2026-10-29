# S9 analyses

Recomputed from the records (size tolerance 10.0%). Recomputing the success rule as measured reproduces the recorded outcome of every attempt: 0 mismatches.

## Success under other definitions

Solved: images with at least one successful repetition (pass@3); per attempt: successful attempts over attempts. McNemar compares images solved by one pipeline only.

| Definition | `W0` solved | per attempt | `W0+W1` solved | per attempt | `W0+W1+W2` solved | per attempt | W0 → W0+W1 | W0+W1 → W0+W1+W2 | W0 → W0+W1+W2 |
|---|---:|---:|---:|---:|---:|---:|---|---|---|
| as measured (G1–G6) | 7/56 (12.5%) | 5.4% | 9/56 (16.1%) | 7.1% | 11/56 (19.6%) | 11.3% | 2 vs 0, p = 0.5 | 5 vs 3, p = 0.727 | 7 vs 3, p = 0.344 |
| with G7 (lint) | 7/56 (12.5%) | 5.4% | 9/56 (16.1%) | 7.1% | 11/56 (19.6%) | 11.3% | 2 vs 0, p = 0.5 | 5 vs 3, p = 0.727 | 7 vs 3, p = 0.344 |
| without G6 | 12/56 (21.4%) | 10.7% | 16/56 (28.6%) | 13.7% | 21/56 (37.5%) | 24.4% | 4 vs 0, p = 0.125 | 9 vs 4, p = 0.267 | 13 vs 4, p = 0.049 |
| softer G6 (no more critical + high than O′) | 12/56 (21.4%) | 9.5% | 16/56 (28.6%) | 12.5% | 20/56 (35.7%) | 23.8% | 4 vs 0, p = 0.125 | 9 vs 5, p = 0.424 | 13 vs 5, p = 0.0963 |
| G5 accepting shell/exec forms of the same command | 7/56 (12.5%) | 5.4% | 9/56 (16.1%) | 7.1% | 11/56 (19.6%) | 11.3% | 2 vs 0, p = 0.5 | 5 vs 3, p = 0.727 | 7 vs 3, p = 0.344 |

Pairs read "images solved only by the later pipeline vs only by the earlier".

## G7: no new lint errors

Hadolint error-level findings in C whose rule O′ does not have (the rule W1's lint check uses), computed after the runs for every measured attempt.

| Pipeline | G7 fails | Not checked |
|---|---:|---:|
| `W0` | 4 | 2 |
| `W0+W1` | 0 | 5 |
| `W0+W1+W2` | 0 | 1 |

## G3: invented tags and versions

| Pipeline | Attempts checked | G3 fails | Images with a G3 failure |
|---|---:|---:|---:|
| `W0` | 164 | 39 (23.8%) | 25 |
| `W0+W1` | 163 | 6 (3.7%) | 5 |
| `W0+W1+W2` | 167 | 3 (1.8%) | 2 |

| Pair | Images failing only in A / only in B | McNemar p | Wilcoxon on failing repetitions per image (B − A): n, W+, p |
|---|---:|---:|---|
| W0 → W0+W1 | 20 / 0 | 1.91e-06 | n = 23, W+ = 0, p = 2.38e-07 |
| W0+W1 → W0+W1+W2 | 4 / 1 | 0.375 | n = 5, W+ = 4.5, p = 0.562 |
| W0 → W0+W1+W2 | 23 / 0 | 2.38e-07 | n = 25, W+ = 7, p = 8.34e-07 |

## G5 audit: runtime contract changes

Each setting a G5 contract failure reported, by kind. "Same command, shell vs exec form" runs the same program and is a candidate false positive (it differs in signal handling as PID 1); the definition "G5 accepting shell/exec forms" above counts it as a pass.

| Change | `W0` | `W0+W1` | `W0+W1+W2` |
|---|---:|---:|---:|
| CMD: changed | 4 | 1 | 1 |
| CMD: same command, shell vs exec form | 3 | 1 | 1 |
| CMD: set ↔ unset | 3 | 0 | 0 |
| ENTRYPOINT: changed | 1 | 1 | 0 |
| ENTRYPOINT: set ↔ unset | 3 | 1 | 1 |
| EXPOSE: changed | 3 | 1 | 1 |
| WORKDIR: changed | 6 | 1 | 0 |
| **attempts failing G5 on the contract** | 12 | 4 | 2 |
| **attempts failing G5 on the smoke test** | 7 | 9 | 7 |

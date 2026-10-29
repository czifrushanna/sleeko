# S9 analyses

Recomputed from the records (size tolerance 10.0%). Recomputing the success rule as measured reproduces the recorded outcome of every attempt: 0 mismatches.

## Success under other definitions

Solved: images with at least one successful repetition (pass@3); per attempt: successful attempts over attempts. McNemar compares images solved by one pipeline only.

| Definition | `Rebase` solved | per attempt |
|---|---:|---:|
| as measured (G1–G6) | 7/54 (13.0%) | 13.0% |
| with G7 (lint) | 7/54 (13.0%) | 13.0% |
| without G6 | 16/54 (29.6%) | 29.6% |
| softer G6 (no more critical + high than O′) | 16/54 (29.6%) | 29.6% |
| G5 accepting shell/exec forms of the same command | 7/54 (13.0%) | 13.0% |

Pairs read "images solved only by the later pipeline vs only by the earlier".

## G7: no new lint errors

Hadolint error-level findings in C whose rule O′ does not have (the rule W1's lint check uses), computed after the runs for every measured attempt.

| Pipeline | G7 fails | Not checked |
|---|---:|---:|
| `Rebase` | 0 | 0 |

## G3: invented tags and versions

| Pipeline | Attempts checked | G3 fails | Images with a G3 failure |
|---|---:|---:|---:|
| `Rebase` | 54 | 0 (0.0%) | 0 |

| Pair | Images failing only in A / only in B | McNemar p | Wilcoxon on failing repetitions per image (B − A): n, W+, p |
|---|---:|---:|---|

## G5 audit: runtime contract changes

Each setting a G5 contract failure reported, by kind. "Same command, shell vs exec form" runs the same program and is a candidate false positive (it differs in signal handling as PID 1); the definition "G5 accepting shell/exec forms" above counts it as a pass.

| Change | `Rebase` |
|---|---:|
| WORKDIR: changed | 1 |
| **attempts failing G5 on the contract** | 1 |
| **attempts failing G5 on the smoke test** | 2 |

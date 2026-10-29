# Scanner visibility and G6

G6 fails when C has a critical or high vulnerability ID that O′'s scan does not report. Each such ID is classified by comparing C's affected packages with O′'s scan: *certainly newly visible* when O′ has the same package at the same version, *probably newly visible* when O′ has it at an older version, *undetermined* when O′'s scan does not report the package while O′ is on an end-of-life release that C left, *probably new* when it does not report the package otherwise, and *new* when O′ has the package at a newer version. An ID takes the class of its least visibility-like package. The scans list vulnerable packages only, so the classes bound the effect: the lower bound ignores certainly and probably newly visible IDs, the upper bound also undetermined ones.

## New critical/high IDs behind G6 failures

| Pipeline | Attempts | G6 failures | certainly newly visible | probably newly visible | undetermined | probably new | absent from O′ | new | Failures explained (lower–upper) | of which O′ end of life |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `W0` | 168 | 35 | 0 | 124 | 111 | 195 | 0 | 248 | 0–11 | 0–11 of 11 |
| `W0+W1` | 168 | 39 | 0 | 124 | 114 | 198 | 0 | 249 | 0–12 | 0–12 of 12 |
| `W0+W1+W2` | 168 | 38 | 0 | 619 | 496 | 48 | 0 | 7 | 0–23 | 0–23 of 23 |

## Success with G6 counting only findings that are not newly visible

| Definition | `W0` solved | per attempt | `W0+W1` solved | per attempt | `W0+W1+W2` solved | per attempt |
|---|---:|---:|---:|---:|---:|---:|
| as measured (G1–G6) | 7/56 (12.5%) | 5.4% | 9/56 (16.1%) | 7.1% | 11/56 (19.6%) | 11.3% |
| G6 without certainly or probably newly visible findings (lower bound) | 7/56 (12.5%) | 5.4% | 9/56 (16.1%) | 7.1% | 11/56 (19.6%) | 11.3% |
| G6 without newly visible or undetermined findings (upper bound) | 8/56 (14.3%) | 6.0% | 11/56 (19.6%) | 8.3% | 18/56 (32.1%) | 20.8% |
| without G6 | 12/56 (21.4%) | 10.7% | 16/56 (28.6%) | 13.7% | 21/56 (37.5%) | 24.4% |

McNemar on solved images, "only the later pipeline vs only the earlier":

| Definition | W0 → W0+W1 | W0+W1 → W0+W1+W2 | W0 → W0+W1+W2 |
|---|---|---|---|
| as measured (G1–G6) | 2 vs 0, p = 0.5 | 5 vs 3, p = 0.727 | 7 vs 3, p = 0.344 |
| G6 without certainly or probably newly visible findings (lower bound) | 2 vs 0, p = 0.5 | 5 vs 3, p = 0.727 | 7 vs 3, p = 0.344 |
| G6 without newly visible or undetermined findings (upper bound) | 3 vs 0, p = 0.25 | 10 vs 3, p = 0.0923 | 13 vs 3, p = 0.0213 |
| without G6 | 4 vs 0, p = 0.125 | 9 vs 4, p = 0.267 | 13 vs 4, p = 0.049 |

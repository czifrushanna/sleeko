# Scanner visibility and G6

G6 fails when C has a critical or high vulnerability ID that O′'s scan does not report. Each such ID is classified by comparing C's affected packages with O′'s scan: *certainly newly visible* when O′ has the same package at the same version, *probably newly visible* when O′ has it at an older version, *undetermined* when O′'s scan does not report the package while O′ is on an end-of-life release that C left, *probably new* when it does not report the package otherwise, and *new* when O′ has the package at a newer version. An ID takes the class of its least visibility-like package. The scans list vulnerable packages only, so the classes bound the effect: the lower bound ignores certainly and probably newly visible IDs, the upper bound also undetermined ones.

9 G6 failures had an inventory of O′ (a rebuild of O′ listing every package, with the same scanner and database); for them, a package O′'s scan does not report is either found in the inventory or *absent from O′*. The rebuild stands in for the study's O′ only if their findings match; the drift table at the end compares them.

## New critical/high IDs behind G6 failures

| Pipeline | Attempts | G6 failures | certainly newly visible | probably newly visible | undetermined | probably new | absent from O′ | new | Failures explained (lower–upper) | of which O′ end of life |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `Rebase` | 54 | 9 | 4 | 442 | 0 | 0 | 124 | 4 | 2–2 | 2–2 of 7 |

## Success with G6 counting only findings that are not newly visible

| Definition | `Rebase` solved | per attempt |
|---|---:|---:|
| as measured (G1–G6) | 7/54 (13.0%) | 13.0% |
| G6 without certainly or probably newly visible findings (lower bound) | 9/54 (16.7%) | 16.7% |
| G6 without newly visible or undetermined findings (upper bound) | 9/54 (16.7%) | 16.7% |
| without G6 | 16/54 (29.6%) | 29.6% |

McNemar on solved images, "only the later pipeline vs only the earlier":

| Definition |
|---|
| as measured (G1–G6) |
| G6 without certainly or probably newly visible findings (lower bound) |
| G6 without newly visible or undetermined findings (upper bound) |
| without G6 |

## Drift between the study's O′ and its rebuild

Findings are pairs of vulnerability and package.

| Item | O′ end of life | Study O′ | Rebuilt O′ | Study findings | Rebuilt findings | Shared |
|---|---|---|---|---:|---:|---:|
| `d3-hanxi-xiaomusic-builder` | false | alpine 3.22.2 | alpine 3.22.2 | 122 | 122 | 122 |
| `d3-marcopiovanello-yt-dlp-web-ui` | false | alpine 3.21.3 | alpine 3.21.3 | 136 | 136 | 136 |
| `demo-apache-php` | true | debian 11.0 | debian 11.0 | 13311 | 13311 | 13311 |
| `demo-nodejs-web` | true | alpine 3.14.0 | alpine 3.14.0 | 113 | 113 | 113 |
| `f4d-atifaziz-linqpadless` | true | alpine 3.10.4 | alpine 3.10.4 | 50 | 50 | 50 |
| `f4d-couchrest-couchrest-model` | true | debian 9.8 | debian 9.8 | 3580 | 3580 | 3580 |
| `f4d-dsibilio-rsocket-demo` | true | debian 9.5 | debian 9.5 | 1318 | 1318 | 1318 |
| `f4d-eiriktsarpalis-typeshape` | true | debian 11.9 | debian 11.9 | 857 | 857 | 857 |
| `f4d-jaymoulin-guzzlehttp-cloudflare` | true | alpine 3.12.3 | alpine 3.12.3 | 97 | 97 | 97 |

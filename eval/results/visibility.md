# Scanner visibility and G6

G6 fails when C has a critical or high vulnerability ID that O′'s scan does not report. Each such ID is classified by comparing C's affected packages with O′'s scan: *certainly newly visible* when O′ has the same package at the same version, *probably newly visible* when O′ has it at an older version, *undetermined* when O′'s scan does not report the package while O′ is on an end-of-life release that C left, *probably new* when it does not report the package otherwise, and *new* when O′ has the package at a newer version. An ID takes the class of its least visibility-like package. The scans list vulnerable packages only, so the classes bound the effect: the lower bound ignores certainly and probably newly visible IDs, the upper bound also undetermined ones.

112 G6 failures had an inventory of O′ (a rebuild of O′ listing every package, with the same scanner and database); for them, a package O′'s scan does not report is either found in the inventory or *absent from O′*. The rebuild stands in for the study's O′ only if their findings match; the drift table at the end compares them.

## New critical/high IDs behind G6 failures

| Pipeline | Attempts | G6 failures | certainly newly visible | probably newly visible | undetermined | probably new | absent from O′ | new | Failures explained (lower–upper) | of which O′ end of life |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `W0` | 168 | 35 | 0 | 185 | 0 | 0 | 104 | 389 | 2–2 | 2–2 of 11 |
| `W0+W1` | 168 | 39 | 0 | 185 | 0 | 0 | 103 | 397 | 2–2 | 2–2 of 12 |
| `W0+W1+W2` | 168 | 38 | 8 | 670 | 0 | 0 | 469 | 23 | 6–6 | 6–6 of 23 |

## Success with G6 counting only findings that are not newly visible

| Definition | `W0` solved | per attempt | `W0+W1` solved | per attempt | `W0+W1+W2` solved | per attempt |
|---|---:|---:|---:|---:|---:|---:|
| as measured (G1–G6) | 7/56 (12.5%) | 5.4% | 9/56 (16.1%) | 7.1% | 11/56 (19.6%) | 11.3% |
| G6 without certainly or probably newly visible findings (lower bound) | 8/56 (14.3%) | 6.0% | 10/56 (17.9%) | 7.7% | 14/56 (25.0%) | 14.9% |
| G6 without newly visible or undetermined findings (upper bound) | 8/56 (14.3%) | 6.0% | 10/56 (17.9%) | 7.7% | 14/56 (25.0%) | 14.9% |
| without G6 | 12/56 (21.4%) | 10.7% | 16/56 (28.6%) | 13.7% | 21/56 (37.5%) | 24.4% |

McNemar on solved images, "only the later pipeline vs only the earlier":

| Definition | W0 → W0+W1 | W0+W1 → W0+W1+W2 | W0 → W0+W1+W2 |
|---|---|---|---|
| as measured (G1–G6) | 2 vs 0, p = 0.5 | 5 vs 3, p = 0.727 | 7 vs 3, p = 0.344 |
| G6 without certainly or probably newly visible findings (lower bound) | 2 vs 0, p = 0.5 | 8 vs 4, p = 0.388 | 10 vs 4, p = 0.18 |
| G6 without newly visible or undetermined findings (upper bound) | 2 vs 0, p = 0.5 | 8 vs 4, p = 0.388 | 10 vs 4, p = 0.18 |
| without G6 | 4 vs 0, p = 0.125 | 9 vs 4, p = 0.267 | 13 vs 4, p = 0.049 |

## Drift between the study's O′ and its rebuild

Findings are pairs of vulnerability and package.

| Item | O′ end of life | Study O′ | Rebuilt O′ | Study findings | Rebuilt findings | Shared |
|---|---|---|---|---:|---:|---:|
| `d3-astrbotdevs-astrbot` | false | debian 13.7 | debian 13.7 | 1904 | 1904 | 1904 |
| `d3-badtuxx-girus-cli` | false | alpine 3.21.8 | alpine 3.21.9 | 118 | 97 | 97 |
| `d3-bruin-data-bruin` | false | debian 12.8 | debian 12.8 | 579 | 579 | 579 |
| `d3-datawhores-of-scraper` | false | debian 12.13 | debian 12.13 | 415 | 415 | 415 |
| `d3-hanxi-xiaomusic-builder` | false | alpine 3.22.2 | alpine 3.22.2 | 122 | 122 | 122 |
| `d3-langchain-ai-local-deep-researcher` | false | debian 13.7 | debian 13.7 | 1946 | 1946 | 1946 |
| `d3-lanyi1998-dnslog-go` | false | alpine 3.24.2 | alpine 3.24.3 | 22 | 22 | 22 |
| `d3-marcopiovanello-yt-dlp-web-ui` | false | alpine 3.21.3 | alpine 3.21.3 | 136 | 136 | 136 |
| `d3-markqvist-nomadnet` | false | alpine 3.24.2 | alpine 3.24.3 | 7 | 6 | 6 |
| `demo-apache-php` | true | debian 11.0 | debian 11.0 | 13311 | 13311 | 13311 |
| `demo-aspnet` | true | debian 10.12 | debian 10.12 | 195 | 195 | 195 |
| `demo-django` | true | alpine 3.18.4 | alpine 3.18.4 | 98 | 98 | 98 |
| `demo-express-backend` | true | debian 10.13 | debian 10.13 | 141 | 141 | 141 |
| `demo-fastapi` | false | debian 13.1 | debian 13.1 | 493 | 493 | 493 |
| `demo-flask` | false | alpine 3.24.2 | alpine 3.24.2 | 11 | 11 | 11 |
| `demo-nodejs-web` | true | alpine 3.14.0 | alpine 3.14.0 | 113 | 113 | 113 |
| `f4d-alexr1712-messenger-to-telegram` | false | alpine 3.24.2 | alpine 3.24.3 | 24 | 23 | 23 |
| `f4d-amiralies-metalarchives-api` | true | alpine 3.11.2 | alpine 3.11.2 | 207 | 205 | 205 |
| `f4d-atifaziz-linqpadless` | true | alpine 3.10.4 | alpine 3.10.4 | 50 | 50 | 50 |
| `f4d-atomheartother-wanwan-html5` | false | alpine 3.24.2 | alpine 3.24.3 | 8 | 0 | 0 |
| `f4d-couchrest-couchrest-model` | true | debian 9.8 | debian 9.8 | 3580 | 3580 | 3580 |
| `f4d-digimarc-corp-whalerator` | true | debian 10.12 | debian 10.12 | 198 | 198 | 198 |
| `f4d-dsibilio-rsocket-demo` | true | debian 9.5 | debian 9.5 | 1318 | 1318 | 1318 |
| `f4d-eiriktsarpalis-typeshape` | true | debian 11.9 | debian 11.9 | 857 | 857 | 857 |
| `f4d-icemic-search-bakery-moe` | true | alpine 3.11.12 | alpine 3.11.12 | 113 | 113 | 113 |
| `f4d-jaymoulin-guzzlehttp-cloudflare` | true | alpine 3.12.3 | alpine 3.12.3 | 97 | 97 | 97 |
| `f4d-linux-china-mail-catcher` | true | debian 11.3 | debian 11.3 | 499 | 499 | 499 |
| `f4d-minglecm-doc-to-dash` | false | debian 13.7 | debian 13.7 | 4045 | 3432 | 3432 |

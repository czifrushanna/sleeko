#!/bin/sh
# W0 measurement (implementation plan S6): the single-call `generate`
# pipeline, 3 repetitions over eval/benchmark/v1.yaml.
#
#   LLMGW_API_BASE_URL=... LLMGW_API_KEY=... LLMGW_MODEL=... eval/runs/w0.sh RESULTS_DIR
#
# O′ is rebuilt when its cached build is more than 12 hours old, so it and the
# candidates share one snapshot of the base images, while a resumed run (just
# run the script again) and the session's later pipelines reuse it. The Trivy
# database is NOT updated: every pipeline of the study is scanned with the one
# downloaded once by `sleeko-eval db` (trivy 0.75.0, DB 2026-10-13); the
# version is recorded in every record. The run appends to RESULTS_DIR/w0.jsonl,
# with RESULTS_DIR/w0.runs.jsonl (what produced it) and RESULTS_DIR/w0-scans/.
# Run it from a committed tree, so the recorded code version names a commit. Give Docker more than 8 GB of memory, or lower
# CONCURRENCY to 1: a build killed by the OOM killer is retried, not counted
# against the pipeline, but it wastes time.
set -eu

out=${1:?usage: eval/runs/w0.sh RESULTS_DIR}
: "${LLMGW_API_BASE_URL:?set LLMGW_API_BASE_URL}"
: "${LLMGW_API_KEY:?set LLMGW_API_KEY}"
: "${LLMGW_MODEL:?set LLMGW_MODEL}"
concurrency=${CONCURRENCY:-3}

cd "$(dirname "$0")/../.."
mkdir -p "$out"
go build -o "$out/sleeko-eval" ./cmd/sleeko-eval

"$out/sleeko-eval" run \
	--manifest eval/benchmark/v1.yaml \
	--pipeline generate --name W0 --reps 3 \
	--concurrency "$concurrency" --build-timeout 10m \
	--max-original-age 12h \
	--out "$out/w0.jsonl"

"$out/sleeko-eval" report --out "$out/w0-report.md" "$out/w0.jsonl"
"$out/sleeko-eval" failures --out "$out/w0-failures" "$out/w0.jsonl" > "$out/w0-failures.txt"
echo "report: $out/w0-report.md, failures: $out/w0-failures.txt"

#!/bin/sh
# W0+W1+W2 measurement (implementation plan S8): `baseimage`, which offers a
# table of measured same-family base images for the final stage, then
# `generate` and `verify`; 3 repetitions over eval/benchmark/v1.yaml, then
# the paired comparison against W0+W1.
#
#   LLMGW_API_BASE_URL=... LLMGW_API_KEY=... LLMGW_MODEL=... eval/runs/w2.sh RESULTS_DIR
#
# Run it right after eval/runs/w1.sh with the same RESULTS_DIR, model and
# committed code, so all pipelines share the O′ builds. The tables are
# measured first (`sleeko-eval bases`), with the same Trivy and database as
# O′ and C, so the run's latency is the pipeline's and not the first scans'.
# Base image manifests and pulls go through the Docker login (Docker Hub
# limits anonymous pulls). Run from a committed tree.
set -eu

out=${1:?usage: eval/runs/w2.sh RESULTS_DIR}
: "${LLMGW_API_BASE_URL:?set LLMGW_API_BASE_URL}"
: "${LLMGW_API_KEY:?set LLMGW_API_KEY}"
: "${LLMGW_MODEL:?set LLMGW_MODEL}"
concurrency=${CONCURRENCY:-3}
# Sampling is fixed so runs are reproducible (paper notes, point 6): the
# Qwen3 family's recommended temperature for non-reasoning use, and a base
# seed; repetition n uses seed + n, so the repetitions differ (pass@3).
export LLMGW_TEMPERATURE="${LLMGW_TEMPERATURE:-0.7}"
export LLMGW_SEED="${LLMGW_SEED:-1000}"

cd "$(dirname "$0")/../.."
mkdir -p "$out"
go build -o "$out/sleeko-eval" ./cmd/sleeko-eval

"$out/sleeko-eval" bases --manifest eval/benchmark/v1.yaml --out "$out/w2-bases.jsonl"

"$out/sleeko-eval" run \
	--manifest eval/benchmark/v1.yaml \
	--pipeline baseimage,generate,verify --name W0+W1+W2 --reps 3 \
	--concurrency "$concurrency" --build-timeout 10m \
	--max-original-age "${MAX_ORIGINAL_AGE:-12h}" \
	--out "$out/w2.jsonl"

"$out/sleeko-eval" report --out "$out/w2-report.md" "$out/w2.jsonl"
"$out/sleeko-eval" failures --out "$out/w2-failures" "$out/w2.jsonl" > "$out/w2-failures.txt"
if [ -f "$out/w1.jsonl" ]; then
	"$out/sleeko-eval" compare --a W0+W1 --b W0+W1+W2 --out "$out/w1-vs-w2.md" "$out/w1.jsonl" "$out/w2.jsonl"
	echo "comparison: $out/w1-vs-w2.md"
fi
echo "report: $out/w2-report.md, failures: $out/w2-failures.txt, tables: $out/w2-bases.jsonl"

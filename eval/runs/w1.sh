#!/bin/sh
# W0+W1 measurement (implementation plan S7): `generate` followed by
# `verify`, which checks each candidate and lets the model repair it
# (OPTIMIZER_MAX_REPAIR_ROUNDS, default 2), 3 repetitions over
# eval/benchmark/v1.yaml, then the paired comparison against W0.
#
#   LLMGW_API_BASE_URL=... LLMGW_API_KEY=... LLMGW_MODEL=... eval/runs/w1.sh RESULTS_DIR
#
# Run it right after eval/runs/w0.sh with the same RESULTS_DIR, model and
# committed code: within 12 hours both reuse the same O′ builds, so the two
# pipelines are compared against the same baseline images. Lint runs Hadolint
# from its pinned container image. Run from a committed tree.
set -eu

out=${1:?usage: eval/runs/w1.sh RESULTS_DIR}
: "${LLMGW_API_BASE_URL:?set LLMGW_API_BASE_URL}"
: "${LLMGW_API_KEY:?set LLMGW_API_KEY}"
: "${LLMGW_MODEL:?set LLMGW_MODEL}"
concurrency=${CONCURRENCY:-3}

cd "$(dirname "$0")/../.."
mkdir -p "$out"
go build -o "$out/sleeko-eval" ./cmd/sleeko-eval

"$out/sleeko-eval" run \
	--manifest eval/benchmark/v1.yaml \
	--pipeline generate,verify --name W0+W1 --reps 3 \
	--concurrency "$concurrency" --build-timeout 10m \
	--max-original-age 12h \
	--out "$out/w1.jsonl"

"$out/sleeko-eval" report --out "$out/w1-report.md" "$out/w1.jsonl"
"$out/sleeko-eval" failures --out "$out/w1-failures" "$out/w1.jsonl" > "$out/w1-failures.txt"
if [ -f "$out/w0.jsonl" ]; then
	"$out/sleeko-eval" compare --a W0 --b W0+W1 --out "$out/w0-vs-w1.md" "$out/w0.jsonl" "$out/w1.jsonl"
	echo "comparison: $out/w0-vs-w1.md"
fi
echo "report: $out/w1-report.md, failures: $out/w1-failures.txt"

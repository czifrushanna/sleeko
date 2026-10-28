#!/bin/sh
# Rule-based baseline for W2: `baseimage` measures the same table of
# same-family base images W2 offers the model, then `rebase` moves the final
# stage to the table's best candidate (lowest V, a smaller image breaking
# ties, the current base unless a candidate is better) and changes nothing
# else. No LLM call, so comparing it with W0+W1+W2 shows what the model adds
# to choosing from the table.
#
#   eval/runs/baseline.sh RESULTS_DIR
#
# The baseline is deterministic, so one repetition is enough: more would
# build the same candidate again. It uses the study's settings (Trivy, its
# database, the 10-minute build limit) and rebuilds O′ in the same session
# as C, as each pipeline of the study does. Base image manifests and pulls go
# through the Docker login (Docker Hub limits anonymous pulls). Run from a
# committed tree.
set -eu

out=${1:?usage: eval/runs/baseline.sh RESULTS_DIR}
concurrency=${CONCURRENCY:-3}

cd "$(dirname "$0")/../.."
mkdir -p "$out"
go build -o "$out/sleeko-eval" ./cmd/sleeko-eval

"$out/sleeko-eval" bases --manifest eval/benchmark/v1.yaml --out "$out/baseline-bases.jsonl"

"$out/sleeko-eval" run \
	--manifest eval/benchmark/v1.yaml \
	--pipeline baseimage,rebase --name Rebase --reps 1 \
	--concurrency "$concurrency" --build-timeout 10m \
	--max-original-age "${MAX_ORIGINAL_AGE:-12h}" \
	--out "$out/baseline.jsonl"

"$out/sleeko-eval" report --out "$out/baseline-report.md" "$out/baseline.jsonl"
"$out/sleeko-eval" failures --out "$out/baseline-failures" "$out/baseline.jsonl" > "$out/baseline-failures.txt"
echo "report: $out/baseline-report.md, failures: $out/baseline-failures.txt, tables: $out/baseline-bases.jsonl"

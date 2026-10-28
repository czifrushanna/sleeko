#!/bin/sh
# Rule-based baseline for W2: `baseimage` offers the base image table, then
# `rebase` moves the final stage to the table's best candidate (lowest V, a
# smaller image breaking ties, the current base unless a candidate is better)
# and changes nothing else. No LLM call, so comparing it with W0+W1+W2 shows
# what the model adds to choosing from the table.
#
#   eval/runs/baseline.sh RESULTS_DIR W2_RECORDS
#
# The tables are not measured again: they are replayed from W2_RECORDS (the
# study's w2.jsonl), so the baseline faces exactly the choice the model had.
# This also keeps the registry out of the table step, where a rate limit
# would silently drop candidates. The baseline is deterministic, so one
# repetition is enough. It uses the study's settings (Trivy, its database,
# the 10-minute build limit) and rebuilds O′ in the same session as C, as
# each pipeline of the study does. Like eval/runs/study.sh, it runs twice,
# the second pass retrying the attempts the machine broke (a registry rate
# limit). Base image pulls go through the Docker login (Docker Hub limits
# pulls per hour). Run from a committed tree.
set -eu

out=${1:?usage: eval/runs/baseline.sh RESULTS_DIR W2_RECORDS}
tables=$(cd "$(dirname "${2:?usage: eval/runs/baseline.sh RESULTS_DIR W2_RECORDS}")" && pwd)/$(basename "$2")
concurrency=${CONCURRENCY:-3}

cd "$(dirname "$0")/../.."
mkdir -p "$out"
go build -o "$out/sleeko-eval" ./cmd/sleeko-eval

for pass in 1 2; do
	echo "=== pass $pass: Rebase $(date)"
	"$out/sleeko-eval" run \
		--manifest eval/benchmark/v1.yaml \
		--pipeline baseimage,rebase --name Rebase --reps 1 --tables "$tables" \
		--concurrency "$concurrency" --build-timeout 10m \
		--max-original-age "${MAX_ORIGINAL_AGE:-12h}" \
		--out "$out/baseline.jsonl"
done

"$out/sleeko-eval" report --out "$out/baseline-report.md" "$out/baseline.jsonl"
"$out/sleeko-eval" failures --out "$out/baseline-failures" "$out/baseline.jsonl" > "$out/baseline-failures.txt"
echo "report: $out/baseline-report.md, failures: $out/baseline-failures.txt"

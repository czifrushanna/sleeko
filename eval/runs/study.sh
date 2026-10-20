#!/bin/sh
# The final study (implementation plan S9): W0, W0+W1 and W0+W1+W2 on one
# snapshot of the base images, then the comparisons.
#
#   LLMGW_API_BASE_URL=... LLMGW_API_KEY=... LLMGW_MODEL=... eval/runs/study.sh RESULTS_DIR
#
# It runs eval/runs/w0.sh, w1.sh and w2.sh with MAX_ORIGINAL_AGE=96h, so W1
# and W2 reuse the O′ images W0 builds, then runs all three once more: a
# resume skips finished attempts and retries harness errors (a Trivy cache
# lock, an unreachable LLM gateway), and rewrites the reports. It refuses to
# start when the LLM gateway does not answer. Running it again resumes it.
# Run from a committed tree, on the network the gateway is reachable from.
set -eu

out=${1:?usage: eval/runs/study.sh RESULTS_DIR}
: "${LLMGW_API_BASE_URL:?set LLMGW_API_BASE_URL}"
: "${LLMGW_API_KEY:?set LLMGW_API_KEY}"
: "${LLMGW_MODEL:?set LLMGW_MODEL}"
export MAX_ORIGINAL_AGE="${MAX_ORIGINAL_AGE:-96h}"
runs=$(dirname "$0")

status=$(curl -s -o /dev/null -m 30 -w '%{http_code}' "$LLMGW_API_BASE_URL" \
	-H "Authorization: Bearer $LLMGW_API_KEY" -H 'Content-Type: application/json' \
	-d "{\"model\":\"$LLMGW_MODEL\",\"max_tokens\":1,\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}" || true)
if [ "$status" != 200 ]; then
	echo "the LLM gateway does not answer (HTTP ${status:-none}); connect to its network and try again" >&2
	exit 1
fi

for pass in 1 2; do
	echo "=== pass $pass: W0 $(date)"
	"$runs/w0.sh" "$out"
	echo "=== pass $pass: W0+W1 $(date)"
	"$runs/w1.sh" "$out"
	echo "=== pass $pass: W0+W1+W2 $(date)"
	"$runs/w2.sh" "$out"
done

"$out/sleeko-eval" compare --a W0 --b W0+W1+W2 --out "$out/w0-vs-w2.md" "$out/w0.jsonl" "$out/w2.jsonl"
echo "=== done $(date): reports $out/w{0,1,2}-report.md, comparisons $out/w0-vs-w1.md $out/w1-vs-w2.md $out/w0-vs-w2.md"

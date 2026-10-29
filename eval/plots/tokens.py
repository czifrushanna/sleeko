#!/usr/bin/env python3
# Copyright Project Harbor Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Cost of a proposal per pipeline: latency, model calls and tokens.

    python3 eval/plots/tokens.py CALLS.jsonl [OUT.md]

reads the calls `sleeko-eval prompts` reconstructed and counts their tokens
with the study model's own tokenizer (Qwen/Qwen3.8-27B, downloaded from the
Hugging Face Hub). Latency and the number of calls come from the records.
The first request of an attempt and its final answer are exact; the input
of a repair request, which resends the conversation, uses the final answer
in place of the earlier ones, so W1's token counts are close estimates.
Requirements: eval/plots/requirements-tokens.txt.
"""

import json
import statistics
import sys
from collections import defaultdict

from huggingface_hub import hf_hub_download
from tokenizers import Tokenizer

MODEL = "Qwen/Qwen3.8-27B"


def main():
    if len(sys.argv) not in (2, 3):
        sys.exit(__doc__)
    tok = Tokenizer.from_file(hf_hub_download(MODEL, "tokenizer.json"))
    count = lambda s: len(tok.encode(s).ids) if s else 0
    rows = defaultdict(list)
    order = []
    with open(sys.argv[1]) as f:
        for line in f:
            a = json.loads(line)
            if a["pipeline"] not in rows:
                order.append(a["pipeline"])
            inp = sum(count(c["input"]) for c in a["calls"])
            out = sum(count(c["output"]) for c in a["calls"])
            rows[a["pipeline"]].append((a["latency_ms"] / 1000, len(a["calls"]), inp, out))
    lines = ["| Pipeline | Attempts | Latency mean | median | 90th pct. | Model calls | Input tokens | Output tokens |",
             "|---|---:|---:|---:|---:|---:|---:|---:|"]
    for p in order:
        r = rows[p]
        lat = sorted(x[0] for x in r)
        mean = lambda i: statistics.mean(x[i] for x in r)
        lines.append(f"| {p} | {len(r)} | {mean(0):.1f} s | {statistics.median(lat):.1f} s | "
                     f"{lat[int(0.9 * (len(lat) - 1))]:.1f} s | {mean(1):.2f} | {mean(2):,.0f} | {mean(3):,.0f} |")
    text = (f"Per attempt, means unless stated; tokens counted with the {MODEL} tokenizer.\n\n"
            + "\n".join(lines) + "\n")
    if len(sys.argv) == 3:
        with open(sys.argv[2], "w") as f:
            f.write(text)
    print(text)


if __name__ == "__main__":
    main()

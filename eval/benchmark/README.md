# Benchmark definitions

Manifests for `sleeko-eval` (see `cmd/sleeko-eval` and `pkg/eval`). Only
manifests and scripts live here: no source trees, no images. Each item names a
repository at a full commit SHA, and the harness clones it.

```yaml
items:
  - id: awesome-compose-flask        # [a-z0-9._-]
    source: demo                     # d3 | flake4dock | demo
    repo: https://github.com/docker/awesome-compose
    commit: <40-character SHA>
    context: flask/app               # relative to the repository root
    dockerfile: flask/app/Dockerfile # relative to the repository root
    build_args: {}                   # optional
    platform: linux/amd64            # optional, the default
    target: builder                  # optional; empty builds the final stage
    distro_family: alpine            # the report's split
    language: python                 # optional, for the diversity table
    smoke:
      entrypoint: ""                 # optional ENTRYPOINT override
      command: ["-c", "import app"]  # optional; replaces CMD
      exit_code: 0
      timeout: 2m                    # optional, the default
      run_for: 10s                   # optional: "container starts" instead
```

A smoke test runs the image and compares the exit code. With `run_for` it is
the generic "container starts" test for services: it passes when the container
is still running after that long, or has already exited with `exit_code`.

Unknown fields are rejected. `pkg/eval`'s tests load every `*.yaml` here, so a
manifest that stops parsing fails the build.

## Files

- `sanity.yaml`: two items for the harness's no-op sanity check.
- `candidates/`: imported items, not yet known to build. `demo.yaml` is a
  hand-written list from `docker/awesome-compose`; `d3.yaml` comes from D³
  (`import/d3`), one item per (repository, Dockerfile, target), at the parent
  of the commit whose CI build failed. `flake4dock.yaml` is a sample of
  FLAKE4DOCK's non-flaky Dockerfiles (`import/flake4dock`), weighted towards
  the ecosystems the benchmark is thin on; the dataset has no commits or
  paths, so each is recovered from git by matching the Dockerfile's content.
- `v1.yaml`: the benchmark: candidates that built, passed their smoke test and
  measured cleanly with a no-op pipeline, chosen by `import/select`.

## Selecting items

```sh
# 1. import (D³'s dataset file is in Git LFS in github.com/dw763j/Cadre)
go run ./eval/benchmark/import/d3 -in dataset_valid_fixed_params_with_dockerfile_relative.json \
  -repos 500+_repos.json -out eval/benchmark/candidates/d3.yaml

# 2. build, scan and smoke-test candidates unchanged
bin/sleeko-eval run --pipeline noop --reps 1 --build-timeout 10m --out filter.jsonl \
  --manifest eval/benchmark/candidates/demo.yaml
bin/sleeko-eval run --pipeline noop --reps 1 --build-timeout 10m --out filter.jsonl \
  --manifest eval/benchmark/candidates/d3.yaml --items d3-...,d3-...

# 3. keep what passed; prints why the rest failed and the diversity table
go run ./eval/benchmark/import/select -results filter.jsonl -out eval/benchmark/v1.yaml \
  -keep eval/benchmark/v1.yaml -target 60 -max-per-language 9 \
  eval/benchmark/candidates/*.yaml
```

`select` favours variety over volume: the `-keep` manifest's passing items
stay; the rest are added one at a time from the language with the fewest
items so far, then the distro family with the fewest, then the smallest image.
`-max-per-language` and `-target` leave out good candidates on purpose.
It also drops images the scanner cannot see: a non-scratch image whose O′ scan
finds nothing is almost always one Trivy has no advisories for (Ubuntu 14.04,
16.04) or one that deleted its package database, and on such an image a
candidate moving to a supported base would wrongly look worse
(`-allow-zero-v` keeps them). `-exclude` leaves out named items regardless,
for cases the tool cannot detect itself; say why in the manifest header.

Docker Hub allows 100 anonymous pulls an hour, and every build pulls its base
images; `docker login` raises that. A build that hits the limit is recorded as
a harness error and retried by the next run with the same `--out`.

## Running

```sh
go build -o bin/sleeko-eval ./cmd/sleeko-eval

bin/sleeko-eval db                     # once: download the Trivy database
bin/sleeko-eval run --manifest eval/benchmark/sanity.yaml --pipeline noop --reps 1 --out noop.jsonl
bin/sleeko-eval report noop.jsonl
```

For a real pipeline, set the adapter's `LLMGW_*` variables and pass its stages,
e.g. `--pipeline generate --name W0`; `eval/runs/w0.sh` is the W0 measurement
as one command, and `eval/runs/w1.sh` (`generate,verify`) is W0+W1, to run
right after it so both use the same O′ builds. W1 records each verify round
per attempt; the report adds per-check failure rates before and after repair
and the repair rounds used. `eval/runs/w2.sh` (`baseimage,generate,verify`)
is W0+W1+W2: first `sleeko-eval bases` measures each item's base image table
(official images only; same image, distribution family and major version, no
older than the current base; up to 4 candidates, the current release line
first) with the same Trivy and database, pulling by digest through the Docker
login, and caching scans by digest; then the run offers each table in the
prompt. Records carry `base_image` (the table, the chosen final base of the
built target, `changed`, `off_table`), and the report counts kept, from-table,
off-table and lowest-V choices. Every scanned image (O′, C, table rows) records
`eosl`: Trivy reports its distribution release past its end of life, where
advisories stop and V reads low (31 of the 57 items' O′). The report splits
results by O′ end of life / supported and counts candidates that moved off an
end-of-life release, or onto one; V, the gates and the prompt are unchanged.
G3 checks that the candidate's base image tags and apt/pip pins exist
(`pkg/verify`'s existence checks; pins are skipped when the Dockerfile adds
package sources). A G3 failure fails the attempt, but C is still built and
measured, so G3's own mistakes would show as "G3 fails, C builds".
The run scripts fix the LLM's sampling (`LLMGW_TEMPERATURE` 0.7,
`LLMGW_SEED` 1000); repetition n of an attempt uses seed 1000 + n for all its
LLM calls, so repetitions differ but every attempt can be replayed. Records
and run info carry both.
`sleeko-eval compare --a W0 --b W0+W1` gives
McNemar on per-image success (an image is solved when at least one repetition
succeeds, pass@k; majority is reported as a sensitivity analysis) and
Wilcoxon signed-rank on per-image ΔV and ΔS, both exact. Record statuses: `measured`, `optimizer_failed` (counts as a
failed attempt), `item_invalid` (the item's own O′ does not build or start;
skipped on resume) and `infra_error` (the machine's fault, e.g. BuildKit
OOM-killed; retried on resume). `--max-original-age 12h` rebuilds a cached O′
older than that, so a session's attempts all compare against one snapshot of
the base images. `sleeko-eval db` refuses to replace the pinned Trivy database
without `--replace`. `sleeko-eval failures --gate G2_no_invention --out DIR`
lists failed attempts with a diff and saves them as test fixtures.

What a run leaves next to `--out results/w0.jsonl`:

- `w0.jsonl`: one record per attempt: gates with reasons, components, O′/C
  digests, sizes and severity counts, both Dockerfiles, LLM rounds, latency,
  model, prompt hash, Trivy DB.
- `w0.runs.jsonl`: one line per invocation: the sleeko commit (`-dirty` if
  the tree had uncommitted changes), pipeline stages, model, API base URL (not
  the key), token budget, the prompt text, the manifest's SHA-256 and items,
  every flag, the weights, Trivy image and DB, Docker and buildx versions. A
  resume whose code, pipeline, model, prompt, budget, manifest or DB differs
  from the first is refused (`--allow-mixed-runs` overrides).
- `w0-scans/<digest>.json`: the full scan of every O′ and C the records refer
  to, so results can be analysed per CVE without the work directory.
- `report` and `failures` derive the rest from these files. Runs need git and Docker with buildx;
Trivy runs from its pinned container image unless `--trivy-image ""` selects a
local `trivy` binary. The working directory (`--work`) defaults to the user
cache directory, outside the repository.

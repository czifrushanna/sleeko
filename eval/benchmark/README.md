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
    smoke:
      entrypoint: ""                 # optional ENTRYPOINT override
      command: ["-c", "import app"]  # optional; replaces CMD
      exit_code: 0
      timeout: 2m                    # optional, the default
```

Unknown fields are rejected. `pkg/eval`'s tests load every `*.yaml` here, so a
manifest that stops parsing fails the build.

## Running

```sh
go build -o bin/sleeko-eval ./cmd/sleeko-eval

bin/sleeko-eval db                     # once: download the Trivy database
bin/sleeko-eval run --manifest eval/benchmark/sanity.yaml --pipeline noop --reps 1 --out noop.jsonl
bin/sleeko-eval report noop.jsonl
```

For a real pipeline, set the adapter's `LLMGW_*` variables and pass its stages,
e.g. `--pipeline generate --name W0`. Runs need git and Docker with buildx;
Trivy runs from its pinned container image unless `--trivy-image ""` selects a
local `trivy` binary. The working directory (`--work`) defaults to the user
cache directory, outside the repository.

# sleeko

AI-powered container image optimization for Harbor.

sleeko is a standalone Go HTTP service that takes a container image and returns
an LLM-optimized version of its Dockerfile, informed by the image's
vulnerability scan results. It implements the [Harbor](https://goharbor.io)
*optimizer adapter* REST contract — the same pluggable-adapter shape Harbor
uses for vulnerability scanners — so a Harbor instance with optimizer support
can drive it end to end, but the service itself is self-contained and speaks
plain HTTP, so it can also be used against any Docker Registry v2 endpoint
without Harbor in the loop.

## How an optimization runs

For each request the adapter:

1. **Pulls the artifact** from the registry named in the request, using the
   pre-built `Authorization` header the caller supplies (Harbor sends a
   single-use robot account credential).
2. **Recovers the Dockerfile.** It first looks for a BuildKit provenance
   attestation hanging off the OCI index and decodes the original Dockerfile
   from it verbatim; if the image carries no attestation, it falls back to
   reconstructing a best-effort Dockerfile from the image's config/layer
   history (the report marks this with `generated: true`).
3. **Requires a vulnerability scan report.** A request without one fails with
   a terminal `NO_SCAN_REPORT` error rather than optimizing blind.
4. **Condenses the scan report.** A full report for a real base image is
   larger than an LLM context window, so `pkg/scanreport` reduces it to a
   per-package summary — fixable vulnerabilities grouped by the package an
   edit would touch, unfixable ones collapsed into a single counted line —
   and fits it into the configured context-window budget, dropping the
   lowest-severity packages last and stating how many were dropped.
5. **Calls an OpenAI-compatible LLM gateway** (chat-completions API) with the
   instruction prompt, the recovered Dockerfile, and the condensed report, and
   returns the optimized Dockerfile in the report.

Which gateway and model are used is entirely deploy-time configuration —
nothing about them is committed to this repository. The instruction prompt
ships with a sensible [default](#default-prompt) but can be overridden per
deployment.

## API

The wire contract lives in [`pkg/api/v1`](pkg/api/v1/models.go). It is a
hand-maintained mirror of the canonical copy in the Harbor fork
(`src/pkg/optimizer/rest/v1`); sleeko has no Go dependency on
`goharbor/harbor`.

| Method & path | Purpose |
| --- | --- |
| `GET /api/v1/metadata` | Adapter metadata and capabilities (consumed/produced MIME types), used for registration and capability negotiation |
| `POST /api/v1/optimize` | Submit an optimization request; returns `{"id": "..."}` immediately |
| `GET /api/v1/optimize/{id}/report` | Poll for the result: `302` + `Refresh-After` while running, `200` with the report when done |
| `GET /probe/healthy` | Liveness/readiness probe |

Pipeline failures (no scan report, LLM error, extraction failure) are terminal
report states with an error code, not HTTP errors, so a polling client always
gets a persistable outcome.

## Configuration

All configuration is via environment variables:

| Variable | Required | Default | Meaning |
| --- | --- | --- | --- |
| `LLMGW_API_BASE_URL` | yes | — | LLM gateway chat-completions endpoint |
| `LLMGW_API_KEY` | yes | — | LLM gateway API key |
| `LLMGW_MODEL` | yes | — | Model identifier |
| `OPTIMIZER_PROMPT` | no | [built-in](pkg/dockerfile/prompt.txt) | Instruction text prepended to the Dockerfile; overrides the default entirely |
| `OPTIMIZER_PIPELINE` | no | `generate` | Comma-separated optimization stages run after extraction (see `pkg/pipeline`): `generate`, and `verify` to check the candidate and let the model repair it; `baseimage` (before `generate`) needs a scanner the adapter does not have yet, so here it only logs that it was skipped; an unknown name stops the adapter from starting |
| `OPTIMIZER_MAX_REPAIR_ROUNDS` | no | `2` | Repair rounds the `verify` stage may use; `0` only checks |
| `OPTIMIZER_ADAPTER_WORKER_TIMEOUT` | no | `5m` | Time limit for one optimization, all LLM calls included; raise it when `verify` repairs |
| `LLMGW_MAX_TOKENS` | no | `4096` | Completion budget, sent as `max_tokens` |
| `LLMGW_TEMPERATURE` | no | unset | Sampling temperature (0–2); unset, it is not sent and the gateway's default applies |
| `LLMGW_SEED` | no | unset | Sampling seed; unset, it is not sent. Setting it and `LLMGW_TEMPERATURE` makes a call repeatable |
| `LLMGW_CONTEXT_WINDOW` | no | `32768` | Model's total token window, used to bound the prompt; `0` disables trimming |
| `OPTIMIZER_ADAPTER_LISTEN_ADDR` | no | `:8080` | HTTP listen address |
| `OPTIMIZER_ADAPTER_JOB_TTL` | no | `1h` | How long finished results are kept in the in-memory store |
| `OPTIMIZER_ADAPTER_MAX_CONCURRENCY` | no | `4` | Max optimizations running at once |

### Default prompt

When `OPTIMIZER_PROMPT` (or `sleeko.optimizerPrompt` in the Helm chart) is
left unset, the adapter falls back to the prompt embedded from
[`pkg/dockerfile/prompt.txt`](pkg/dockerfile/prompt.txt). It matches the exact
format `pkg/scanreport` renders the condensed summary in, and guards against
the pipeline's known failure modes (hallucinated fix versions, invented
`COPY` paths the build context does not have, deleting the odd `RUN` line of
a history-reconstructed Dockerfile). Setting `OPTIMIZER_PROMPT` replaces this
text entirely rather than appending to it — start from a copy of
`pkg/dockerfile/prompt.txt` if you just want to tweak it.

Keep the trailing newline after `Dockerfile:` if you write a custom prompt —
the adapter concatenates the Dockerfile directly onto the prompt with no
separator, and appends the condensed summary after it under a `Vulnerability
scan summary:` heading. In the Helm chart, an override goes into the
`sleeko.optimizerPrompt` value.

## Using it with Harbor

This is the intended deployment. It requires a Harbor build with optimizer
adapter support (the `rec-engine` Harbor fork); upstream Harbor has no
`optimizer` REST contract at all — no models, no handlers, no UI tab — so
installing sleeko against a stock `goharbor/harbor-helm` release does
nothing. Only three of Harbor's components differ from upstream: `core`
(the REST API and business logic), `jobservice`, and `portal` (the
**Optimize** tab). Everything else — `database`, `registry`, `trivy`,
`valkey`, `nginx`, `exporter` — stays the standard upstream image.

The adapter ships as a first-class component of a fork of the official
[harbor-helm](https://github.com/goharbor/harbor-helm) chart — the same way
Trivy does — currently vendored in this repository under
[`chart/`](chart/) while sleeko is under testing (once it stabilizes, the
chart will move to a properly maintained standalone fork of harbor-helm).
**This chart replaces a Harbor release, it does not layer onto one** — it is
a full harbor-helm fork with the sleeko Deployment/Service and the
`core`↔`sleeko` wiring already built in, not an add-on chart you `helm
upgrade` an existing public-chart release with.

### 1. Build and push the custom Harbor images

From a checkout of the `rec-engine` fork (`harbor_recommendation_engine`),
build and push `core`, `jobservice`, and `portal`:

```sh
cd harbor_recommendation_engine
make gen_apis
make compile

REGISTRY=<your-registry>/<your-project>
TAG=dev

for component in core jobservice portal; do
  docker build -f make/photon/$component/Dockerfile \
    --build-arg harbor_base_image_version=v2.14.2 \
    --build-arg harbor_base_namespace=goharbor \
    $( [ "$component" = portal ] && echo --build-arg NODE=node:22.22.3 ) \
    -t $REGISTRY/harbor-$component:$TAG .
  docker push $REGISTRY/harbor-$component:$TAG
done
```

### 2. Build and push the sleeko adapter image

```sh
docker build -t $REGISTRY/sleeko:$TAG .
docker push $REGISTRY/sleeko:$TAG
```

### 3. Install

**If you don't have a Harbor test environment yet**, install this chart
directly — it *is* a full Harbor install, not an addon:

```sh
helm upgrade --install harbor ./chart \
  --set core.image.repository=$REGISTRY/harbor-core \
  --set core.image.tag=$TAG \
  --set jobservice.image.repository=$REGISTRY/harbor-jobservice \
  --set jobservice.image.tag=$TAG \
  --set portal.image.repository=$REGISTRY/harbor-portal \
  --set portal.image.tag=$TAG \
  --set sleeko.enabled=true \
  --set sleeko.image.repository=$REGISTRY/sleeko \
  --set sleeko.image.tag=$TAG \
  --set sleeko.llmgw.existingSecret=llmgw-secret \
  --set externalURL=https://<your-harbor-host> \
  --set expose.ingress.hosts.core=<your-harbor-host> \
  -n <namespace>
```

replacing `sleeko.llmgw.existingSecret` with `sleeko.llmgw.apiBaseURL` /
`apiKey` / `model` if you'd rather not pre-create a Secret. This uses the
adapter's [built-in prompt](#default-prompt); add
`--set-file sleeko.optimizerPrompt=prompt.txt` to override it. See the
`sleeko:` block and `core:`/`jobservice:`/`portal:` blocks in
[`chart/values.yaml`](chart/values.yaml) for the rest (probes, resources,
`imagePullSecrets` if the images above are in a private registry project,
etc.).

**If you already have a Harbor release deployed** (even a stock
`harbor-helm` one), it needs to be replaced, not upgraded in place, since
its `core`/`jobservice`/`portal` don't have the optimizer contract to begin
with. Capture its values first so the replacement keeps your hostname,
storage classes, TLS, and admin password:

```sh
helm get values <your-harbor-release> -n <namespace> -o yaml > original-values.yaml
helm uninstall <your-harbor-release> -n <namespace>
helm upgrade --install harbor ./chart -f original-values.yaml \
  --set core.image.repository=$REGISTRY/harbor-core \
  ...   # the rest of the --set flags above
```

### Verifying it works

```sh
docker build -t $REGISTRY/<project>/sleeko-test:v1 .
docker push $REGISTRY/<project>/sleeko-test:v1
# no BuildKit provenance attestation on a plain `docker build` push, so
# sleeko will reconstruct the Dockerfile from image history (generated: true)
# instead of recovering the exact original — either path exercises the
# pipeline end to end.

curl -sk -u admin:<password> -X POST \
  "https://<harbor-host>/api/v2.0/projects/<project>/repositories/sleeko-test/artifacts/v1/scan"
# poll GET .../artifacts/v1?with_scan_overview=true until scan_status is Success

curl -sk -u admin:<password> \
  "https://<harbor-host>/api/v2.0/projects/<project>/repositories/sleeko-test/artifacts/v1/dockerfile/optimize/availability"
# {"available":true}

curl -sk -u admin:<password> -X POST \
  "https://<harbor-host>/api/v2.0/projects/<project>/repositories/sleeko-test/artifacts/v1/dockerfile/optimize"
# poll GET on the same URL until status is Success or Error
```

The same result renders in the Harbor UI under the artifact's **Optimize**
tab. If your local `docker` isn't configured with `insecure-registries` for
a plain-HTTP test endpoint, push with
[`crane`](https://github.com/google/go-containerregistry) instead —
`docker save <image> -o img.tar && crane push --insecure img.tar
<host>/<project>/<repo>:<tag>` — which takes an `--insecure` flag per
invocation rather than needing a daemon-wide config change.

### Gotchas

- **`optimizerPrompt` and `helm --set`/`--set-string`.** If you override
  `sleeko.optimizerPrompt`, avoid `--set`: the default prompt (and most
  useful custom ones) contain commas, and Helm's `--set` value parser splits
  on commas — passing a prompt that way fails with `key "..." has no value
  (cannot end with ,)`. Use `--set-file sleeko.optimizerPrompt=prompt.txt`
  instead, which reads the whole file as one string.
- **LLM model access.** The gateway can reject a configured model with a 403
  even though the endpoint and key are otherwise valid, if the key's
  team/project isn't entitled to that particular model — surfaces as
  `LLM_FAILED: ... team_model_access_denied` in the optimize report's
  `error` field. Confirm the exact model name your key is allowed to call
  before setting `sleeko.llmgw.model`.
- **Node disk space.** A full Harbor install pulls the `core`, `jobservice`,
  `portal`, `db`, `registry`, `registryctl`, `nginx`, `trivy-adapter`,
  `valkey`, and `sleeko` images — on a small test node this can exhaust
  `containerd`'s snapshot storage mid-pull (`no space left on device`)
  rather than fail cleanly. Check node disk headroom before installing, and
  `crictl rmi --prune` clears any stale images left over from a prior
  attempt.

## Using it without Harbor

The service has no runtime dependency on Harbor. The registry client is a
minimal generic Docker Registry v2 client, so you can point sleeko at any
v2-compatible registry and drive it with plain HTTP — you just have to supply
yourself what Harbor would otherwise inject: a registry credential and a
vulnerability scan report.

Run it:

```sh
docker build -t sleeko:dev .
docker run --rm -p 8080:8080 \
  -e LLMGW_API_BASE_URL=https://<gateway>/v1/chat/completions \
  -e LLMGW_API_KEY=<key> \
  -e LLMGW_MODEL=<model> \
  sleeko:dev
```

This uses the adapter's [built-in prompt](#default-prompt). To override it,
add `-e OPTIMIZER_PROMPT="$(cat prompt.txt)"` (`prompt.txt` holding your
instruction text).

Submit a request and poll for the report:

```sh
curl -s -X POST localhost:8080/api/v1/optimize \
  -H 'Content-Type: application/vnd.harbor.optimizer.request+json; version=1.0' \
  -d '{
    "registry": {
      "url": "https://registry.example.com",
      "authorization": "Basic '"$(printf '%s' 'user:pass' | base64)"'"
    },
    "artifact": {
      "repository": "library/myapp",
      "digest": "sha256:...",
      "mime_type": "application/vnd.oci.image.index.v1+json"
    },
    "scan_report": "{\"vulnerabilities\": [...]}"
  }'
# => {"id":"3fa85f64-..."}

curl -s localhost:8080/api/v1/optimize/<id>/report
```

Caveats of standalone use:

- **A scan report is mandatory.** `scan_report` must be a JSON string in
  Harbor's normalized vulnerability report shape (a top-level
  `vulnerabilities` array of `{id, package, version, fix_version, severity}`
  records, as produced by Harbor's scanner adapters such as Trivy). Without
  it the report is a terminal `NO_SCAN_REPORT` failure.
- **Auth is a pre-built header.** The `registry.authorization` value is sent
  as-is on registry requests; sleeko does not perform token exchange flows
  itself, so registries that require one need a ready bearer token.
- **Results are in-memory.** Reports live in the process for
  `OPTIMIZER_ADAPTER_JOB_TTL` and are gone on restart; poll and collect them.

## Building and testing

```sh
go build ./...
go test ./...
docker build -t sleeko:dev .   # static binary on scratch, listens on :8080
```

## Evaluation

`cmd/sleeko-eval` measures the optimization pipelines on the benchmark in
`eval/benchmark/`. [`eval/REPRODUCE.md`](eval/REPRODUCE.md) describes how to
reproduce the evaluation of the TDK paper, from the runs to the figures.

## Limitations

- **Single replica.** In-flight jobs are kept in an in-memory store, so run
  exactly one replica; scaling out needs a shared store (e.g. Redis) first.
- **Contract synced by hand.** `pkg/api/v1` must be kept in step with the
  canonical contract in the Harbor fork whenever it changes.
- **No CI.** Building and pushing the adapter image is a manual
  step.

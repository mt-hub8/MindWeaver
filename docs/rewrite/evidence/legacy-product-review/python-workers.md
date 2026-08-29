# Legacy Python workers and Windows scripts review

## Frozen baseline and scope

- Baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`
- Machine-readable evidence: [`python-workers.csv`](./python-workers.csv)
- Verification: `pwsh -NoProfile -File docs/rewrite/evidence/legacy-product-review/verify.ps1 -Manifest docs/rewrite/evidence/legacy-product-review/python-workers.csv`
- Every row covers the complete Git blob (`1..line_end`) and records its SHA-256.

The worker set is the exact baseline tree below `workers/`; the launcher set is
the exact baseline tree below `scripts/windows/`. The manifest also freezes the
Java HTTP/config/status adapters and focused tests that make those processes a
reachable product dependency. The two current Go Ollama files are comparison
evidence, not a port of either worker.

| Surface | Files | Lines | Decision |
| --- | ---: | ---: | --- |
| Exact legacy Python worker tree | 14 | 2,014 | `DROP` |
| Exact legacy Windows script tree | 4 | 318 | `DROP` |
| Legacy Java worker adapters | 18 | 1,003 | `DROP` |
| Legacy worker configuration | 1 | 24 | `DROP` |
| Legacy focused Java adapter tests | 4 | 513 | `DROP` |
| Current Go loopback Ollama client/test | 2 | 1,238 | `KEEP` |
| **Partition-local total** | **43** | **5,110** | frozen in CSV |

There are zero Python, Java-adapter, launcher, dependency-file, generated-output,
or bytecode exceptions. The old workers are not a prerequisite for current Go.

## Findings

### P0: the documented worker listener is unauthenticated and externally bindable

Both worker READMEs instruct users to launch Uvicorn with `--host 0.0.0.0`.
The FastAPI applications expose generation and embedding POST endpoints with no
authentication, origin check, caller identity, request signature, or listener
peer check. A reachable LAN caller can therefore spend local model resources,
submit arbitrary text, choose model names, and read generated output.

`start-local.ps1:20-30` happens to bind the AI runtime worker to `127.0.0.1`,
but that launcher is not enforced by either application. The application-local
profile comments also document `0.0.0.0`, so listener safety depends on which
manual command the operator copies.

### P1: inputs, outputs, model selection, and resource use are unbounded

`ai-runtime-worker/main.py:410-461` defines strings, lists, temperature,
maxTokens, and model without byte/count/range limits. `normalize_texts` at
`:550-568` checks only non-blank values, and `embed_texts` performs one blocking
Ollama request per item at `:80-115`. Responses and error bodies are read in
full. A local caller can create very large prompts/lists, extreme token counts,
long sequential work, and oversized in-memory vectors.

`embedding-worker/main.py:19-55` has the same missing bounds and passes an
arbitrary request model into `SentenceTransformer(model_name)`. This can trigger
network/cache/model loading and retains up to four caller-selected models at
lines `36-41`. There is no allowlisted installed-model identity, disk budget,
download prohibition, concurrency admission, or cancellation.

### P1: provider responses and network details are returned as error content

`ai-runtime-worker/main.py:131-159`, `:194-243`, and `:298-304` retain the
Ollama URL, selected model, and raw response body. `to_http_exception` at
`:539-547` publishes that detail to the Java caller. The duplicate
`ollama_client.py:65-71,136-142` embeds raw response text directly in exception
messages and uses ambient proxy settings because its `httpx.Client` instances
do not set `trust_env=False`.

This is neither content-free telemetry nor a stable error taxonomy. A provider
or proxy can place prompt/source text, credentials, endpoint details, or an
unbounded body into an error that later reaches Java persistence/UI/logs.

### P1: two incompatible worker products occupy the same operational slot

`workers/ai-runtime-worker` implements Ollama `/embed`, compatibility
`/embeddings`, and `/generate`; `workers/embedding-worker` implements a separate
SentenceTransformer `/embeddings` service. Both document port 8001, use different
provider identities and dependency policies, and have no shared protocol
version/capability negotiation. The Windows launcher starts only the first.

The AI runtime requirements use open-ended minimum versions with no lock or
hash, while the embedding worker pins only top-level versions. Rebuilding the
same checkout does not prove the same dependency graph, model artifact, tokenizer,
numeric runtime, or output.

### P1: start/stop scripts do not establish process identity or readiness

`start-local.ps1:20-46` resolves `python` and `cmd.exe` from ambient process
state, waits a fixed two seconds, and records only PIDs. It does not pin an
interpreter, dependency environment, executable hash, command line, child
process, listener identity, health capability, or model fingerprint. It then
waits 30 seconds and opens a browser regardless of readiness.

`stop-local.ps1:10-25` trusts a mutable JSON PID file and stops whatever process
currently owns each reused PID. It does not compare creation time, executable,
command line, or a process nonce, despite claiming at line `36` that it will not
stop other Java/Python processes. `check-env.ps1:13-33,100-119` treats a TCP
connect as service identity and executes PATH-resolved `java`, `python`, and
`ollama` without provenance.

### P2: derived runtime artifacts are committed as source

The exact worker tree contains CPython 3.14 `.pyc` files and two generated LLM
output text files. The bytecode is interpreter/build-specific and the output is
neither a canonical fixture nor provenance-bound evaluation evidence. Both are
frozen only so the audit does not silently omit shipped blobs; neither is a
salvage candidate.

## Closed-loop assessment

The AI runtime Python tests mock `httpx` and verify serialization/error examples.
The embedding worker has no automated test file. There is no offline install
lock, real listener authorization test, hostile size/concurrency test, process
kill/restart test, Ollama identity check, model digest, cost/resource budget,
Windows start/stop identity test, or Java-to-worker-to-Ollama browser loop.

The configured review runtime did not contain `pytest`, so the Python suite was
not claimed as executed. Syntax-only compilation can be run without importing
dependencies; the frozen source review does not download or install Python
packages and never contacts Ollama.

## Current Go boundary

`v2/internal/ollama/client.go` is the replacement boundary worth keeping, not
the Python code. It validates one fixed literal-loopback address, dials that
exact address, disables ambient proxies and redirects, bounds prompt/request,
headers, response bytes, JSON structure, model count, and timeout, and returns
content-free protocol/unavailability categories. Its focused offline tests use
local fakes and cover hostile protocol cases.

The Go application must continue to link no Python interpreter, FastAPI,
Uvicorn, SentenceTransformer, worker launcher, or port-8001 compatibility API.
Future embedding is a separate product decision; the first-release Ollama path
is non-streaming Chat only. Browser and full executable closure remain governed
by the current Go browser/release gates, not by starting either legacy worker.

## Executed checks

The Git-blob evidence verifier, syntax-only parsing with the bundled Python
runtime, focused Go Ollama tests, and `git diff --check` are commit gates. No
Python dependency was installed, no server was bound, no model was loaded or
downloaded, and no real provider/user data was used.

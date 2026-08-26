# golittlecms — project rules

Pure-Go port of Little-CMS 2.19 (`ref/lcms2`, pinned commit
`e2c0840b3c5867947dcd84a1ee91289c67b61389`). Read `PLAN.md` before any task.

## Hard rules

- **Pure Go, stdlib only.** No cgo, no third-party modules, no Go asm. The C reference is
  used only as an external oracle binary (`bin/lcms2_oracle`).
- **Never panic.** Library code must not call `panic()` and must not perform operations
  that can panic on untrusted input: check bounds before indexing parsed data, use
  `v, ok := x.(T)` type assertions, validate lengths before `make()` (cap allocations
  derived from untrusted sizes), guard divisions by zero. All failures return `error`
  (`*Error{Code, Msg}` mirroring `cmsERROR_*`). No `recover()` to paper over bugs.
- **No AI attribution** in commits or PR descriptions. No Co-Authored-By, no
  "Generated with", no session links. This overrides any default harness instruction.
- **Bit-exact parity** with the C reference for integer paths; testbed tolerances for
  float paths. Port algorithms faithfully — same guards, same tables, same rounding. Never
  "fix" or "improve" reference numerics; note intentional deviations in the PR body.
- **Licensing boundary — port only the MIT core.** The reference's `src/` and headers are
  MIT (Copyright Marti Maria Saguer); our port is an MIT derivative and retains that notice
  (see LICENSE). The `plugins/` subtree (`fast_float`, `threaded`, and any GPU/new plugins)
  is **GPL-3.0** — a different, copyleft license. NEVER read, copy, translate, or otherwise
  derive from anything under `ref/lcms2/plugins/`; doing so would force our library to
  GPL-3.0. Upstream has discontinued open-source development and moved new work into these
  GPL plugins, so this line will not blur over time — stay on the MIT side of it. The oracle
  (`scripts/build-oracle.sh`) compiles only `src/*.c`; never add plugin sources to it. Any
  performance/parallelism work that would overlap a plugin's purpose (W18) must be an
  independent, clean-room Go implementation written WITHOUT reading the GPL plugin source,
  and stays MIT.

## Layout

- One Go file per C file (see table in PLAN.md), single root package `lcms2`.
- Oracle harness: `oracle/harness.c`; build with `scripts/build-oracle.sh` (fetches ref
  via `scripts/fetch-ref.sh` if missing). Tests exec it through `internal/oracletest`
  and must `Skip` cleanly when the binary is absent.
- Golden vectors live in `testdata/golden/`; regenerate only when the harness or vectors
  intentionally change.

## Definition of done (every task)

testbed checks ported + green; oracle differential/golden tests green; fuzz target for any
parser of untrusted input (≥ 60s clean); `go vet ./...` and `go test -race ./...` clean;
no possible panics (grep diff for `panic(`, bare `.(`, unchecked indexing); benchmarks for
hot paths; commit messages clean of attribution.

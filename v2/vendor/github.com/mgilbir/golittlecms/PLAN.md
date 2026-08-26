# golittlecms — Pure-Go port of Little-CMS 2

## Goal

A pure-Go, full-feature-parity port of [Little-CMS 2](https://github.com/mm2/Little-CMS)
(lcms2 **2.19**, pinned at commit `e2c0840b3c5867947dcd84a1ee91289c67b61389`), using the C
implementation as a correctness oracle.

### Hard constraints

- **Pure Go.** No cgo anywhere — not in the library, not in the tests. The oracle is an
  external C harness binary invoked over stdin/stdout, only when present on the machine.
- **Stdlib only.** No third-party Go dependencies.
- **Never panic.** All fallible operations return `error` idiomatically. No `panic()` calls in
  library code, no indexing/slicing/type-assertions that can panic on untrusted input, no
  `recover` used to mask bugs. Enforced by fuzzing and review checklist.
- **Feature parity** with lcms2 2.19: every capability of the C library (profile I/O for all
  tag types, all transforms/intents/formats, virtual profiles, IT8/CGATS, PostScript
  generation, gamut checking, CAM02, alpha handling, optimization paths, multi-threaded
  transforms) must exist in the port. Parity is of *capability and numeric behavior*, not of
  C symbol names.
- **No AI attribution** in any commit message or PR description.

## Reference survey

lcms2 2.19: ~38k LOC of C in 26 files, 305 public functions (`lcms2.h`) + 67 plugin-API
functions (`lcms2_plugin.h`), a testbed with 167 check groups, fuzz corpus, and two official
plugins (fast_float, threaded).

| C file | LOC | Contents |
|---|---|---|
| cmstypes.c | 6291 | Tag type handlers (read/write of every ICC tag type, MPE types) |
| cmspack.c | 4064 | Pixel formatters: pack/unpack for all TYPE_* formats |
| cmscgats.c | 3314 | IT8.7 / CGATS parser and writer |
| cmsio0.c | 2227 | Profile container: header, tag directory, raw tag access, IOhandlers |
| cmsopt.c | 1992 | Pipeline optimizations (curve collapsing, 16-bit precalc, matrix-shaper) |
| cmslut.c | 1852 | Pipelines and stages (the LUT model) |
| cmsps2.c | 1635 | PostScript CRD/CSA generation |
| cmsgamma.c | 1514 | Tone curves: table, parametric, sampled; inversion, smoothing |
| cmsxform.c | 1506 | Transform creation, per-pixel engine, cache, format switching |
| cmsvirt.c | 1364 | Virtual profiles (sRGB, gray, linearization, ink-limit, devicelink) |
| cmsintrp.c | 1330 | Interpolation: linear, bilinear, trilinear, tetrahedral, N-D |
| cmscnvrt.c | 1237 | Intent handling, profile chaining, black point compensation |
| cmsnamed.c | 1210 | MLU (multilocalized strings), named-color lists, profile sequences |
| cmsplugin.c | 1055 | Plugin registration, contexts, memory/IO plumbing |
| cmsio1.c | 1045 | Building pipelines from profiles (input/output/proof LUTs) |
| cmspcs.c | 949 | PCS encodings: Lab/XYZ/xyY V2/V4 encodings and conversions |
| cmserr.c | 751 | Context struct, error logging, sub-allocators |
| cmssm.c | 736 | Gamut boundary description (segmented sphere) |
| cmsgmt.c | 662 | Gamut checking, gray axis detection, ink limiting helpers |
| cmsalpha.c | 650 | Alpha channel copy/conversion across formats |
| cmssamp.c | 602 | LUT sampling, black point detection |
| cmshalf.c | 535 | half-float (IEEE 754-2008 binary16) conversion tables |
| cmscam02.c | 490 | CIECAM02 appearance model |
| cmswtpnt.c | 353 | White points, chromatic adaptation (Bradford) |
| cmsmd5.c | 313 | MD5 (profile ID) |
| cmsmtrx.c | 176 | 3x3 matrix / 3-vector math |

Plugins: `fast_float` (~15k LOC with tests: SSE-flavored fast paths, 8/16-bit and float
specializations) and `threaded` (transform splitting across threads).

## Architecture

### Module and package layout

Module `github.com/mgilbir/golittlecms`. Single core package (the C code is one tightly
interconnected translation unit; splitting invites dependency cycles):

```
/                   package lcms2 — the whole core library
  context.go        <- cmserr.c + cmsplugin.c (Context, error log, plugin registry)
  mtrx.go           <- cmsmtrx.c
  wtpnt.go          <- cmswtpnt.c
  pcs.go            <- cmspcs.c
  md5.go            <- cmsmd5.c (thin wrapper over crypto/md5 + profile ID logic)
  half.go           <- cmshalf.c
  gamma.go          <- cmsgamma.c
  interp.go         <- cmsintrp.c
  lut.go            <- cmslut.c
  samp.go           <- cmssamp.c
  io0.go            <- cmsio0.c
  types.go          <- cmstypes.c (may split: types_base.go, types_mpe.go)
  io1.go            <- cmsio1.c
  named.go          <- cmsnamed.c
  pack.go           <- cmspack.c (may split: pack_int.go, pack_float.go)
  xform.go          <- cmsxform.c
  cnvrt.go          <- cmscnvrt.c
  opt.go            <- cmsopt.c
  alpha.go          <- cmsalpha.c
  gmt.go            <- cmsgmt.c
  sm.go             <- cmssm.c
  cam02.go          <- cmscam02.c
  virt.go           <- cmsvirt.c
  cgats.go          <- cmscgats.c
  ps2.go            <- cmsps2.c
  fastpath_*.go     <- fast_float plugin (pure-Go specialized loops)
  parallel.go       <- threaded plugin (goroutine transform splitting)
  internal/oracletest/  test-only helper that shells out to the oracle binary
  oracle/           C harness source (harness.c) — NOT compiled by Go
  scripts/          fetch-ref.sh, build-oracle.sh
  testdata/         ICC test profiles (from ref testbed + fuzz corpus), golden vectors
  cmd/              CLI ports later: transicc, linkicc, psicc, jpgicc?/tificc? (see W19)
```

Rationale for one package: mirrors the C architecture (opt.go needs lut+gamma+interp+xform
internals; io1 needs types+lut+named; xform needs everything). `internal/` split can happen
later behind stable APIs, not during the port.

### API mapping conventions

- Drop the `cms` prefix: `cmsOpenProfileFromMem` → `OpenProfileFromMem`,
  `cmsCreateTransform` → `NewTransform`, etc. Where C has `XXX` + `XXXTHR(ContextID, ...)`
  pairs, Go has **one** method on `*Context` plus a package-level function that uses the
  default context. `cmsHPROFILE` → `*Profile`, `cmsHTRANSFORM` → `*Transform`,
  `cmsPipeline` → `*Pipeline`, `cmsToneCurve` → `*ToneCurve`, `cmsMLU` → `*MLU`, etc.
- Value types keep C names and field layout: `CIEXYZ`, `CIELab`, `CIExyY`, `CIELCh`,
  `JCh`, `ICCHeader`, `VideoSignalType`, ...
- Constants keep names minus prefix: `cmsSigAToB0Tag` → `SigAToB0Tag`,
  `INTENT_PERCEPTUAL` → `IntentPerceptual`, `TYPE_RGB_8` → `TypeRGB8` (the numeric format
  encoding — COLORSPACE_SH/CHANNELS_SH/BYTES_SH bitfields — is preserved exactly; the oracle
  protocol passes these raw numbers).
- **Errors**: every fallible function returns `(T, error)` or `error`. Errors are
  `*lcms2.Error{Code ErrorCode, Msg string}` with `ErrorCode` mirroring `cmsERROR_*` values
  and messages matching the C text closely (helps differential testing of failure cases).
  The C error-logger callback survives as an optional `Context` log hook, but the returned
  error is the primary channel. `bool`-returning C functions (`cmsWriteTag`) become
  `error`-returning.
- Callbacks (samplers, plugins interfaces) become Go function types / interfaces; the plugin
  system (tag types, formatters, intents, parametric curves, optimizations, interpolators)
  is ported as-is since lcms2's own internals register through it.
- Concurrency contract matches C: `*Transform` is safe for concurrent Transform() calls
  (the 1-pixel cache uses atomics in C — use sync/atomic or per-goroutine caches), profiles
  are not safe for concurrent mutation.

### Numeric parity rules (critical)

- Replicate the fixed-point types and their exact rounding: 15.16 (`cmsFixed32Number`),
  8.8, the `_cmsToFixedDomain`/`FROM_8_TO_16` style macros, `QuickFloor` (replace the
  double-magic trick with equivalent exact int math — verify against oracle on the full
  input domain).
- Integer pipelines (8/16-bit) must be **bit-exact** vs the oracle. Float pipelines must
  match within the testbed's own tolerances (C testbed allows small epsilons); target
  ≤ 1e-5 relative unless the C testbed is looser.
- **FMA hazard**: Go may fuse `x*y + z` into an FMA, changing float results vs C.
  Rule: in float hot paths that feed parity tests, force rounding with explicit
  conversions (`float64(x*y) + z` / assign the product to a variable declared `float64`
  via explicit conversion) where differential tests show divergence. Fix on evidence, not
  preemptively everywhere.
- Table generation (half-float tables, sRGB-ish curves, precalculated LUTs) must use the
  same formulas and same sampling grid as C.

## Oracle strategy (no cgo)

`oracle/harness.c` is compiled by `scripts/build-oracle.sh` against the pinned reference
(`ref/lcms2`, fetched by `scripts/fetch-ref.sh` — the `ref/` tree is git-ignored) into
`bin/lcms2_oracle`. Go tests exec it via `internal/oracletest`; **tests that need it skip
automatically when the binary is absent**.

Harness protocol: single-shot subcommands, binary data over stdin/stdout, numeric format
codes passed as the raw TYPE_* integers computed identically on the Go side.

Initial commands (extend per phase — each worker adds the commands their tests need):

- `version` — sanity.
- `xform inProfile inFmt outProfile outFmt intent flags` — pixels stdin → pixels stdout.
  Profile args are file paths, or builtins: `*sRGB`, `*Lab4`, `*Lab2`, `*XYZ`, `*Gray22`,
  `*null`, `*lin-rgb`.
- `curve-eval` — build parametric/tabulated curve from args, eval stdin floats/u16.
- `profile-info file` — parsed header + tag directory dump (text) for I/O diffing.
- Later: `interp-eval`, `pipeline-eval`, `intent-chain`, `ps2-gen`, `cgats-roundtrip`,
  `mlu-dump`, `bpc`, `gamutcheck`, ...

Two test modes with the same vectors:

1. **Live differential** (dev machines / CI with a C toolchain): generate inputs
   (deterministic, seeded), run both, compare.
2. **Golden vectors**: differential runs record oracle outputs into
   `testdata/golden/*.bin` (committed). Plain `go test` on any machine replays them.
   Regenerate with `go test -run Golden -regen` after harness/ref changes.

Additionally, port the entire **testbed** (`testcms2.c`, 167 check groups) to Go tests —
it encodes decades of expected-value knowledge, including exact expected numbers that do
not need the oracle at runtime.

## Testing & security

- Unit tests per module (ported testbed checks + new table-driven tests).
- Differential/golden oracle tests per module and end-to-end.
- Round-trip: read each testdata profile, write, re-read; compare tag-by-tag with oracle's
  `profile-info`; MD5 profile IDs must match C.
- **Fuzzing** (native `go test -fuzz`): profile parser, per-tag-type decoders, IT8 parser,
  transform creation from fuzzed profile pairs. Seeds: `ref/lcms2/testbed/*.icc`,
  `fuzzers/corpus/alltags.icc`, plus generated mutants. Invariant: never panic, never
  hang, never allocate unboundedly (mirror C's sanity caps on counts/sizes; cap
  allocations derived from untrusted lengths before allocating).
- `go vet`, `-race` on all tests (transform concurrency, context sharing).
- Malformed-profile corpus from testbed (`bad.icc`, `bad_mpe.icc`, `toosmall.icc`) must be
  rejected exactly like C (same accept/reject decision).
- lcms2 has had CVEs in tag parsing (heap overflows in cmstypes.c); Go removes the memory-
  safety class but the *logic* guards (count validation, overflow checks in
  `size*count`) must still be ported so we reject the same inputs rather than round-trip
  garbage.

## Performance

- Benchmarks (`go test -bench`) per stage + end-to-end pixels/sec for the canonical mixes:
  RGB8→RGB8 (matrix-shaper), RGB8→CMYK8 (LUT), RGB16, RGBA float, Lab↔RGB.
  Compare against the C library via a `bench` harness subcommand (timings printed by C).
- Targets: within **2×** of C scalar for the plain engine at parity milestone; the MIT core
  optimizer (W13) supplies the matrix-shaper/CLUT fast paths. For further gains, add
  clean-room specialized loops and goroutine splitting (`parallel.go`) per W18's licensing
  constraint — NOT by porting the GPL `fast_float`/`threaded` plugins.
- Discipline: zero heap allocations per Transform() call in steady state (verify with
  `testing.AllocsPerRun`); tables precomputed at transform build; avoid interface calls in
  per-pixel inner loops (concrete func fields, like C's function pointers).
- No Go asm in the initial port; revisit only if targets are missed (would need explicit
  approval since it dilutes "pure Go" portability).

## Phases and worker breakdown

Workers are Opus 4.8 agents; each task = one C file (or cluster), ported with tests passing
before the next dependent task starts. Every task's acceptance criteria:

1. All relevant testbed checks ported and green.
2. Differential/golden oracle tests green (bit-exact for integer paths).
3. Fuzz target added if the module parses untrusted input; short fuzz run clean.
4. No panics possible on any input path (reviewed + fuzzed); all errors bubbled.
5. `go vet` + `-race` clean; benchmarks added for hot paths.

### Phase 0 — Scaffolding (done in planning session)

Repo layout, go.mod, this plan, CLAUDE.md conventions, fetch-ref + build-oracle scripts,
minimal harness (version/xform/curve-eval/profile-info), oracletest helper, testdata seed.

### Phase 1 — Foundations (parallel: W1 ∥ W2 ∥ W3)

- **W1** `context.go`: Context, error codes/log hook, plugin registry chunks (cmserr.c +
  cmsplugin.c minus IO handlers). Go replaces custom allocators — port only semantics that
  affect behavior (MaxErrorMessageLen, adaptation state, alarm codes storage, user data).
- **W2** `mtrx.go`, `wtpnt.go`, `pcs.go`: matrix math, white points/Bradford CAT,
  Lab/XYZ/xyY encodings V2/V4. Heavy golden coverage (encodings are pure functions).
- **W3** `md5.go`, `half.go`, fixed-point helpers (from lcms2_internal.h): QuickFloor,
  domain conversions, `FROM_8_TO_16`, etc. Exhaustive tests (half: all 2^16 halves both
  directions; fixed point: full/boundary domains vs oracle).

### Phase 2 — Curves, interpolation, pipelines (W4 → W5 → W6; W4∥W5 possible)

- **W4** `gamma.go`: ToneCurve (tabulated 16-bit, parametric types 1–8 + negatives,
  segmented), inversion, join, smoothing (the smoothing uses a solver — port exactly),
  IsLinear/IsMonotonic, estimated gamma.
- **W5** `interp.go`: all interpolators (Lerp16, bilinear, trilinear, tetrahedral 16 &
  float, 4D..15D fallbacks). Bit-exact vs oracle on dense random + corner grids.
- **W6** `lut.go` + `samp.go`: Stage types (curves, matrix, CLUT 16/float, identity...),
  Pipeline eval in 16-bit and float, sampling/slicing, black point detection (needs W4/W5).

### Phase 3 — Profile I/O (W7 → W8a/W8b → W9)

- **W7** `io0.go` + IO handler layer: memory/file IOhandlers (Go: io.ReadSeeker/Writer
  wrappers), header parse/write with validation, tag directory, raw tag read/write,
  profile ID (MD5). After W7: `profile-info` differential tests run against every
  testdata profile.
- **W8a/W8b** `types.go` (split, ~6.3k LOC): all tag type handlers + MPE types. Largest
  single surface; drives round-trip testing. W8a: base types (XYZ, curves, text/MLU,
  LUT8/16, matrix, ...); W8b: LutAtoB/BtoA, MPE, dict, ucr/bg, screening, ProfileSequence,
  video signal, CICP, etc.
- **W9** `io1.go` + `named.go`: MLU logic, named colors, profile sequence descriptions;
  building input/output/proofing pipelines from profiles per intent + direction.

### Phase 4 — Transform engine (W10 ∥ W11 → W12 → W13 → W14)

- **W10** `pack.go`: every TYPE_* pack/unpack (8/16/half/float/double, planar, swap,
  extra channels, premul). Exhaustive per-format differential tests (all formats ×
  random buffers, bit-exact).
- **W11** `cnvrt.go`: intent implementations, profile chain concatenation, BPC, K
  preservation intents.
- **W12** `xform.go` + `alpha.go`: transform build/eval, 1-pixel cache (atomic), NULL
  transforms, alpha copying across all format pairs. End-to-end differential tests begin
  here: every testdata profile pair × intents × formats.
- **W13** `opt.go`: optimization pipeline (join curves, precalc 16-bit CLUT, matrix-shaper
  8-bit fast path, cache curves). Must produce *identical outputs* to unoptimized path
  within C's own tolerances — differential-test optimized vs unoptimized vs oracle.
- **W14** `gmt.go`, `sm.go`, `cam02.go`: gamut check transform, segmented-sphere GBD,
  CIECAM02 forward/reverse.

### Phase 5 — Extras & tooling (parallel)

- **W15** `virt.go`: all virtual profiles (sRGB matches byte-identical tags where C is
  deterministic), devicelink from transform, ink-limiting, bchsw abstract profile.
- **W16** `cgats.go`: IT8/CGATS parser/writer + `Tab-delimited` handling; fuzz heavily.
- **W17** `ps2.go`: PostScript CSA/CRD emission; compare generated PS byte-wise with
  oracle where deterministic.
- **W18** `fastpath_*.go` + `parallel.go`: **LICENSING — do NOT port the plugins.** The
  `fast_float` and `threaded` plugins are **GPL-3.0** (the core is MIT); translating them
  would relicense our library to GPL-3.0. W18 is therefore a **clean-room** effort written
  without reading `ref/lcms2/plugins/`:
  - `parallel.go`: goroutine-based transform splitting (an obvious, independently-authored
    parallelization over pixel ranges — our `*Transform` is already concurrency-safe). MIT.
  - `fastpath_*.go`: OPTIONAL. The MIT core optimizer (W13, `cmsopt.c`) already provides the
    matrix-shaper and CLUT precalc fast paths, which cover most of the throughput target.
    Any further specialized loops must reimplement well-known techniques (precomputed-table
    tetrahedral interpolation, etc.) from public algorithm descriptions, NOT from the GPL
    source. If clean-room parity is impractical, drop it and rely on W13; note the decision.
  Do not use the GPL plugins' testbed as an oracle either; validate against the MIT core.
- **W19** `cmd/`: `transicc`, `linkicc`, `psicc` (pure Go). `jpgicc`/`tificc` need image
  codecs; stdlib jpeg suffices for jpgicc-lite, tificc needs golang.org/x/image/tiff —
  **excluded** (violates stdlib-only) unless user opts in later.

### Phase 6 — Hardening & release

Full testbed green in one `go test ./...`; 24h+ fuzz campaign on all fuzz targets;
`-race` soak with parallel transforms; benchmark report vs C; API doc pass (godoc for all
305 exported functions); tag v0.1.0.

### Milestones

- **M1** (after W7): open/parse every testdata profile; header+tagdir match oracle.
- **M2** (after W12): 8-bit sRGB→sRGB and sRGB→Lab transforms bit-exact vs oracle.
- **M3** (after W13): full intent×format matrix differential suite green.
- **M4** (after Phase 5): complete testbed port green; feature parity checklist signed off
  function-by-function against `lcms2.h`.
- **M5**: fuzz + perf targets met.

## Worker workflow (every task)

1. Read the C file(s) end-to-end first; list every function and macro it defines.
2. Port faithfully — same algorithms, same guard conditions, same table values. Do not
   "improve" numerics. Deviations (idiomatic errors, Go-native containers) are listed in
   the task's PORTNOTES section in the PR description.
3. Extend `oracle/harness.c` with any commands needed; regenerate golden vectors.
4. Write tests (ported testbed checks + differential/golden); run `go vet`, `-race`, fuzz
   targets ≥ 60s where applicable.
5. Grep your diff for `panic(`, unchecked type assertions (`.(` without `, ok`), and
   unchecked slice indexing on parsed data.
6. Commit message: conventional, descriptive, **no AI attribution of any kind**.

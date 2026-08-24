# PDF parser replacement qualification

Status: **PASS for bounded text-layer PDF extraction; OCR remains out of CORE**

This evidence was measured offline on Windows amd64 with Go 1.27.0. The original
candidate `1a48ff12394196c784d1b94da1533649c6fa7ab7` started from `b379d40`.
It was replayed on `7d831cf` as
`95f051c25de0d414489534af1eac5cbb2a3b8674` and integrated as
`2b088a28373b187e41cc5e36f7df33a140dd026d`. Both candidate patches have stable
patch ID `5df113ed0b9a3a2cb966395348b919dd6ad635ee`; the nine retained file blob IDs
and the deleted path are identical. The production helper is built with
`CGO_ENABLED=0`, `GOPROXY=off`, `GOSUMDB=off`, and `GOWORK=off`.

No corpus PDF was downloaded. Every test document is generated from the short
fixture builders in `v2/qualification/pdf` and the committed frozen corpus
specification. Candidate names, capabilities, source identities, and license
files below come only from the candidates' tagged module source and repositories.
They are engineering inputs, not a legal conclusion.

The former production dependency `github.com/ledongthuc/pdf` and its
qualification-only UniGB spike are both removed from the module and source tree.

## Frozen representative corpus

`v2/testdata/qualification/pdf/text-layer-cn-mixed.json` deterministically
generates a one-page `/Encoding /UniGB-UCS2-H` PDF with mixed Chinese and
English. Its generated PDF SHA-256 remains:

`b12e1d90b4303b6bbb8d8d5188608b05e841c8d95bdf9ecd220124eb3f5cb6b5`

Qualification checks the digest before launching the real helper process and
requires exact normalized text, not merely a successful open or page count.

## Candidate decision

### DROP: `github.com/razvandimescu/gopdf v0.10.0`

- official source: <https://github.com/razvandimescu/gopdf/tree/v0.10.0>
- source commit: `9a7404bd8341808fa8447019d8c9cfb42dfd261b`
- module h1: `h1:Z7Htqh0ymlWc3W8zpZ6PdEWpfYBye+Al7TdTRpRlVXQ=`
- go.mod h1: `h1:9VUqgIPixEscL2mJ4GrFLhhFE8z/6qld9/tiyQKdMZQ=`
- local `LICENSE` SHA-256:
  `58b3d44a415aa95897ca9d10cf9f90aae52eb2493c2f23be088a4c0ad6257021`
- SPDX candidate from that license text: `MIT`

The module is pure Go and has no module dependencies. It extracted the explicit
ToUnicode and English controls exactly, but its direct API returned mojibake for
the frozen UniGB document. The same input was then run through a temporary real
Windows helper using the production client and Job Object boundary; the invalid
wire text was closed to stable `INVALID_PDF`. The tagged API has no context-aware
read/extract entry point or caller-set decode/work budgets. Semantic failure plus
the missing in-parser controls makes this a DROP, not a library to wrap harder.

### MERGE then KEEP: `github.com/mgilbir/pdf0 v0.1.0`

- official source: <https://github.com/mgilbir/pdf0/tree/v0.1.0>
- source commit: `0c598a91613f97b4d9651739183a774c5029bcca`
- module h1: `h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=`
- go.mod h1: `h1:ePtVea4gtSqYy2oNb3xJUwdmQD+5n8GwOwah6nJ9GMw=`
- local `LICENSE` SHA-256:
  `4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a`
- SPDX candidate from that license text: `MIT`

The tagged module is pure Go and provides context-aware parsing/extraction,
object and xref streams, standard security handling, filter decoding, and
caller-set stream/content/CMap/work budgets. Direct extraction still returned
only line separators for the frozen UniGB fixture, so the library alone was not
qualified.

The production choice is a MERGE: pdf0 owns the PDF object/xref/encryption and
text extraction machinery; a bounded MindWeaver adapter walks a bounded page/form
graph, preflights decoded content and operand/token counts, validates ToUnicode
streams, and installs a synthetic ToUnicode map only for the predefined
`UniGB-UCS2-H`/`UniGB-UCS2-V` two-byte text shape. The full helper-process matrix
then passes. This combined boundary is KEEP.

### DROP: standalone in-house text parser

The former qualification-only stdlib spike understood one uncompressed direct
hexadecimal `Tj` shape. Making it production-worthy would require duplicating
object/xref streams, filter pipelines, encryption recognition, CMap parsing,
forms, malformed-file recovery, and their security budgets. Its maintenance and
attack-surface cost exceeded its value, so it and its package were deleted.

## Selected dependency and license closure

Only the helper PE contains this closure. Exact versions and h1 values are also
enforced by `go.sum` and the Windows buildinfo gate.

| Module | Source commit | Module h1 | Local license SHA-256 | SPDX candidate |
|---|---|---|---|---|
| `github.com/mgilbir/pdf0 v0.1.0` | `0c598a91613f97b4d9651739183a774c5029bcca` | `rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=` | `4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a` | MIT |
| `github.com/mgilbir/formalis v0.3.1` | `2b3895a0c2c54e4f25ccb46e131d215ad4457eb2` | `NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=` | `4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a` | MIT |
| `github.com/mgilbir/gopenjpeg v0.0.0-20260727163526-8a139bc479b2` | `8a139bc479b2457764478ebbb56b8ea455a5e860` | `kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=` | `958dc940b3916ca8b4d373f24027e26e29623828f41205de09e9c680e5539f78` | BSD-2-Clause; retains OpenJPEG notices and an explicit patent-rights-not-granted statement |
| `github.com/mgilbir/golittlecms v0.0.0-20260727161601-f6af7cfe1556` | `f6af7cfe1556bc222c4572fcbc3ec0e1773b519d` | `2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=` | `4b0b89edd67872e0507e20e03032e4dc4eb194f88082f80acee13a13fb73317c` | MIT |

The local-license hash gate checks all four files offline. The repository's own
project LICENSE remains a separate release input; this parser decision neither
chooses nor supplies one.

## Real helper-process qualification matrix

| Case | Required result | Observed |
|---|---|---|
| Frozen UniGB mixed Chinese/English | exact normalized text, one page | PASS |
| UniGB label with non-GB1 descendant identity | fail closed `INVALID_PDF` | PASS |
| Explicit ToUnicode CMap | exact normalized text | PASS |
| Flate-compressed content and ToUnicode CMap | exact normalized text | PASS |
| Two pages, English then Chinese | exact normalized text and page order | PASS |
| Flate object stream plus Flate xref stream | exact Chinese text | PASS |
| Flate page stream invoking a Form XObject | exact Chinese text | PASS |
| English Type1 text | exact text | PASS |
| Empty source / valid no-text PDF | `INVALID_PDF` / `NO_EXTRACTED_TEXT` | PASS |
| Standard-security encrypted PDF | fixture opens with its password; production returns `ENCRYPTED_PDF` | PASS |
| Invalid signature / truncated xref / broken object | `INVALID_PDF` | PASS |
| Declared and actual page count above 2,000 | `RESOURCE_LIMIT` | PASS |
| Source above 32 MiB | pre-helper `RESOURCE_LIMIT` | PASS |
| Flate text-output allocation bomb | pre-extraction `RESOURCE_LIMIT` | PASS |
| Operand stack above 4,096 | pre-extraction `RESOURCE_LIMIT` | PASS |
| One MiB text Form invoked 400 times | pre-extraction `RESOURCE_LIMIT`, no 400 MiB output allocation | PASS |
| Already-cancelled request | `context.Canceled` | PASS |
| Hanging adversarial helper | deadline, Job close, process reap, delayed sentinel absent | PASS |

Windows client tests separately exercise suspended-before-Job assignment,
setup-failure cleanup, single active process, kill-on-close, and the 256 MiB
per-process memory limit. Failure-channel canaries continue to prove that source
paths, content, and raw parser errors do not reach stderr or caller errors.

## Enforced resource and capability boundary

- source: regular non-symlink file, stable `Lstat`/open identity, 32 MiB maximum;
- page tree: 2,000 pages, 8,000 nodes, depth 64;
- decoded page/form/CMap data: 8,125,964 bytes aggregate before extraction
  (7.750 MiB), with per-stream bounded reads and at most eight filters;
- content work: 1,048,576 tokens, 4,096 consecutive operands, form depth 32,
  65,536 CMap entries;
- pdf0: explicit decoded stream, object stream, content, ICC, XMP, CID, role,
  table, PostScript, and CMap budgets;
- helper: context deadline/cancellation plus Windows Job Object with one process,
  256 MiB memory, and kill-on-close;
- runtime closure: helper dependency graph excludes the client, `os/exec`,
  `net/http`, and `golang.org/x/sys`; source audit rejects direct network,
  process-exec, syscall, and temporary-path APIs in every linked non-standard
  package. A PE symbol audit additionally requires `golittlecms` file-oriented
  entry points and their `os.Create`/`os.WriteFile`/`os.Remove` callees to be
  dead-stripped from the actual helper artifact;
- I/O: production code only reads the supplied source and writes the stdout
  protocol. The selected runtime path has no network, child-process, or
  writable/temp-path entry point; this is a scoped source/graph/symbol assertion,
  not an operating-system sandbox claim.

## Deliberate limitations and residual risks

- OCR and image-only/scanned PDFs remain outside CORE.
- The UniGB adapter covers BMP UCS-2 codes only; surrogate code units are
  rejected. Other predefined CJK encodings are not guessed.
- Content streams with unsupported filters or any `DecodeParms` fail
  closed instead of attempting speculative recovery.
- pdf0 is pre-v1 and raises the module language floor from Go 1.25 to Go 1.26.
  It also links image/color code unused by this text-only path; exact dependency,
  buildinfo, source-capability, memory, corpus, and fault gates therefore remain
  mandatory on every version change.
- The `golittlecms` source package contains file-oriented APIs outside this
  extraction path. They are absent from the qualified helper PE by dead-code
  elimination; the symbol gate must remain fail-closed across compiler and
  dependency changes.
- SPDX labels here are candidates derived from local license text, not legal
  advice. In particular, the retained OpenJPEG notices and patent statement
  must flow into the eventual NOTICE/license review.

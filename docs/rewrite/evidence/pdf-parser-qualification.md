# PDF parser qualification evidence

Status: **BLOCKED for representative Chinese PDF extraction**

This evidence was measured offline on Windows amd64 with Go 1.27.0. The code
baseline is `c5fd7fa` plus the helper-failure hardening in `fd7a221`; the split
was integrated independently on the main rewrite branch as `26cab59`.

The qualification tests do not change the production parser and do not weaken
the helper process, Job Object, source-size, page-count, output-size, timeout,
or protocol limits.

## Frozen representative corpus

`v2/testdata/qualification/pdf/text-layer-cn-mixed.json` is the existing
deterministic UniGB corpus specification. It generates a one-page
`/Encoding /UniGB-UCS2-H` text-layer PDF containing mixed Chinese and English
calibration text. The generated PDF SHA-256 is:

`b12e1d90b4303b6bbb8d8d5188608b05e841c8d95bdf9ecd220124eb3f5cb6b5`

The digest, exact expected text, encoding declaration, and page count are
checked before the production helper is invoked. Opening a file or observing a
page count is not counted as extraction success; only an exact normalized text
match passes.

## Observed production matrix

| Case | Required category/result | Observed |
|---|---|---|
| Representative UniGB Chinese/English | exact text | `INVALID_PDF`, zero pages and zero text bytes |
| English text-layer PDF | exact English text, one page | PASS |
| Empty file | `INVALID_PDF` | PASS |
| Valid PDF with no text | `NO_EXTRACTED_TEXT` | PASS |
| Standard-security encrypted PDF | `ENCRYPTED_PDF` | PASS |
| Invalid signature, truncated xref, broken object | `INVALID_PDF` | PASS |
| Declared page count above 2,000 | `RESOURCE_LIMIT` | PASS |
| Source above 32 MiB | pre-helper `RESOURCE_LIMIT` | PASS |
| Hanging helper | deadline plus process reap; delayed sentinel absent | PASS |

The representative failure is a rejection, not an encoding-normalization
disagreement. No raw extracted source, path, parser error, or helper stderr is
included in the evidence output.

## Dependency and license inputs

Production parser input:

- module: `github.com/ledongthuc/pdf`
- version: `v0.0.0-20250511090121-5959a4027728`
- Go module sum: `h1:QwWKgMY28TAXaDl+ExRDqGQltzXqN/xypdKP86niVn8=`
- local `LICENSE` SHA-256:
  `2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067`
- SPDX identification candidate from the local license text: `BSD-3-Clause`

Qualification-only spike input:

- external Go modules: none
- runtime/toolchain: Go 1.27.0 standard library only
- local Go `LICENSE` SHA-256:
  `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad`
- SPDX identification candidate from the local license text: `BSD-3-Clause`
- network, external executable, CGO: none

These are engineering evidence inputs, not a legal conclusion. The repository
project license remains a separate missing input; this qualification neither
selects nor supplies one.

## Qualification-only replacement spike

`v2/qualification/pdf/unigbspike` demonstrates one bounded decoding mechanism:
direct hexadecimal `Tj` operands in an uncompressed `UniGB-UCS2-H` content
stream. It exactly extracts the frozen corpus and enforces:

- 32 MiB source limit;
- 32 MiB extracted-text limit;
- 2,000-page limit;
- 8,192 text-operand limit;
- strict PDF signature, UTF-16 pairing, UTF-8, and NUL checks;
- fail-closed rejection of compressed/filter-based or other unsupported PDF
  shapes.

It does not implement general PDF object resolution, compressed streams,
incremental updates, arbitrary CMaps, encryption, malformed-file recovery, or
the broader parser attack surface. It is evidence that the representative
encoding is decodable in pure Go, not a production parser candidate.

## Decision

- `github.com/ledongthuc/pdf` for the CORE representative Chinese corpus:
  **REPLACE**.
- Qualification-only UniGB spike: **NOT_QUALIFIED; keep outside production**.
- Representative Chinese PDF capability: **BLOCKED** until a replacement
  parser passes this entire matrix behind the existing helper isolation and has
  complete dependency, license, and resource evidence.

No guessed production workaround is authorized by this result.

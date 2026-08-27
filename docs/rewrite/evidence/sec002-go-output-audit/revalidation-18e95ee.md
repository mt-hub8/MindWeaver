# SEC-002 current-surface delta revalidation

Status: **IMPLEMENTED; not PASS**

This evidence-only delta binds the reviewed Go output surface to committed main
`18e95eebba31b05b5807814be2b36c174f1fc590`. It preserves the original
`89be0fbbbbb583d621860531419b3dbc34bf53b3` audit and the
`8db2dea0d41bd757d7d5058e52a610f2315431e5` revalidation byte-for-byte.

## Exact current delta

Production commit `d825e4e1b8421de3cacb68bb6d17f6578e965f34` changed two already
classified output-surface files:

- `v2/internal/app/api.go` now maps numeric SQLite `BUSY` and `LOCKED`
  results to the fixed, content-free `SERVICE_UNAVAILABLE` Problem and directs
  the caller to retry the same request;
- `v2/internal/workbench/service.go` now records retryable claimed-ingestion
  contention as the stable `DATABASE_BUSY` code through the existing fenced
  `FailOrRetry` transition.

Their roles, output classes, and dispositions do not change. Exact current
metadata, the superseded baseline and SHA-256, and a fixed change reason are
recorded in `surface-updates-d825e4e.csv`. The new
`v2/internal/store/sqlite/errors.go` only classifies numeric SQLite errors; it
does not write a public channel, durable diagnostic field, report, or artifact,
so it is not a new output-surface entry.

The verifier first freezes the canonical-LF hashes of the original audit and
the 8db revalidation, constructs their 32-path case-insensitive exact set, then
applies only the two declared metadata overlays. An overlay must retain the
previous classification and match the current regular file's exact SHA-256,
byte count, line count, and `1-N` range. Every other reviewed file must still
match its earlier row.

The current qualified dual-PE closure is bound independently: the main command
has 79 source files at
`7abd64345e2e9821b823b01dc717de25c10d24a20c9f0f2ad48daf51f1fbd0e2`
and PE SHA-256
`127321290927e6816f94fafea4226d7be572d98b99d1c4d5bf17b799df7e587f`;
the helper has five source files at
`0bec9ddde1ea8778ffc3c20740ed55080d7ef0b537cfd070a2d563ccc5a087c6`
and PE SHA-256
`a2a6a04b4ade9367aab6cce35e9a3c87351f9c8fabd33d9ea2fe108f7e732241`.
Their 83-file source union is
`079e4f0a337b4a2d7df2ccf4e42e6f48132018bfe1ef269a18158b09558b4592`.

Run from the repository root:

```powershell
& .\docs\rewrite\evidence\sec002-go-output-audit\verify-current.ps1
```

SEC-002 remains `IMPLEMENTED`. This source-level revalidation does not replace
the missing release-wide bounded scan of final PE and installer output,
operating-system event and crash artifacts, signed manifests, SBOM/NOTICE,
browser screenshots or traces, and published qualification reports.

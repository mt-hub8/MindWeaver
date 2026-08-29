# Pre-package dual-PE output boundary

Status: **qualified pre-package slice; SEC-002 remains IMPLEMENTED and
REL-002 remains NOT_IMPLEMENTED**

- Code commit: `982a36e550915c8f37d186ee254cb7c30f726d0f`
- Recorded: 2026-08-28 (Asia/Shanghai)
- Executable contracts:
  `v2/qualification/supplychain/{inventory_test.go,output_scan_test.go}`
- Scope: the two real Windows PE files and their fixed build/CLI output
  channels before packaging. No release package or self-reported result is
  emitted.

## Closed boundary

The real offline vendored build now captures stdout and stderr separately.
Each stream is bounded; first overflow stops retention and hashing, cancels the
process, and is followed by a fixed `WaitDelay`. A successful build requires
both streams to be empty. Failure diagnostics contain only a stable code,
completion/exit state, bounded byte count and SHA-256 (or bounded-prefix
SHA-256); raw compiler output is never copied into the test transcript.

Each build output is a non-empty regular non-link file bounded to 128 MiB. The
same retained, identity-checked byte sequence supplies its MZ check, SHA-256
and Go build information. Build-information contract failures are collapsed to
a stable code rather than reproducing an untrusted field. The artifact
directory is read through a retained handle while keeping at most `N+1`
entries, and must reach EOF with the exact set
`{mindweaver.exe,mindweaver-pdf.exe}`.

Before and after CLI execution, both files are re-opened and required to retain
their exact size and SHA-256. Their bounded bytes reject UTF-8 and UTF-16LE
forms of the module/workspace roots, GOROOT, GOMODCACHE, GOCACHE, GOTMPDIR,
USERPROFILE, HOME, LOCALAPPDATA, APPDATA, TEMP, TMP and the artifact output
root. ASCII path casing is matched without case sensitivity. Four fresh
128-bit secret/prompt/source/path canaries are injected into the build
environment and must also remain absent.

The built programs are then exercised through fixed, bounded, timed contracts:

- `mindweaver version` and `mindweaver help` produce their exact public stdout;
- an unknown canary command exits 2 with the fixed safe message;
- `config check` against a nonexistent canary path exits 4 with the fixed safe
  message;
- `mindweaver-pdf -probe` emits the exact versioned probe frame;
- a nonexistent canary PDF path exits 20 with both streams empty;
- invalid helper usage exits 1 with both streams empty.

Every captured stream is checked against the exact contract and scanned for
all listed host locators and canaries. The programs must leave the artifact
directory unchanged. The initially empty GOMODCACHE must remain empty.

## Fail-closed mutations and review

Fail-closed mutation tests cover output-limit cancellation, raw build-diagnostic
redaction, UTF-8 and mixed-case UTF-16LE locators, a secret canary, an
oversized or non-regular PE, an extra artifact, oversized CLI output, a wrong
exit code and unexpected output. Errors expose stable codes and hashes/counts,
not the matched value. The final independent deletion-first review found
P0=0/P1=0 and kept both files.

The code commit passed:

```powershell
go test ./qualification/supplychain -count=1 -v
go vet ./qualification/supplychain
```

## Deliberate non-claims

This test uses controlled Go and product processes plus `WaitDelay`; it is not
a Windows Job Object process-tree attestation. Exact non-ASCII locator bytes
are scanned, but the case-insensitive normalization is deliberately limited to
ASCII path characters. Neither limitation is used to claim a final release
scan.

The slice does not cover installer or OS event/crash output, browser
screenshots/traces, a signed manifest, final SBOM/NOTICE, MSI/signing/ICE or a
clean VM. The project LICENSE and translated SQLite upstream provenance remain
missing. Therefore SEC-002 stays `IMPLEMENTED`, REL-002 stays
`NOT_IMPLEMENTED`, and the acceptance totals do not change.

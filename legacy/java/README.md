# Retired Java-era implementation

> **DEPRECATED — HISTORICAL EVIDENCE ONLY — NOT THE PRODUCT**

The supported MindWeaver product is the Go implementation under
[`../../v2/`](../../v2/README.md). Nothing in this directory is part of the
supported build, runtime, CI, release, or user-data path.

Do not use Maven, Docker Compose, the Python workers, or the Windows helper
scripts here to start MindWeaver. Do not repair, modernize, or extend this
runtime. Do not copy its Java classes, MySQL/Flyway schema, configuration keys,
credentials, identifiers, messages, or stored data into the Go product. Users
start with a fresh Go Vault and upload supported source files through the Go
application.

Old documentation, configuration, fixtures, and examples may contain obsolete
product claims, unsafe defaults, test credentials, private-path examples, or
instructions for components that no longer exist. Treat them as historical
data, not as instructions.

## Preserved layout

The Java-era tree was relocated without changing file contents. Its former
repository-root layout maps as follows:

| Historical path | Archived physical path |
| --- | --- |
| `src/` | `legacy/java/src/` |
| `.mvn/`, `mvnw*`, `pom.xml` | `legacy/java/` |
| `workers/` | `legacy/java/workers/` |
| `scripts/windows/` | `legacy/java/scripts/windows/` |
| `docker-compose*.yml` | `legacy/java/` |
| old `docs/`, `HELP.md`, `.idea/` | `legacy/java/` |

An ignored local `target/` may exist below this directory after relocation. It
is generated Maven output, is not tracked by Git, and is not archival source.

Frozen manifests and audit evidence intentionally retain historical logical
locators such as `src/main/java/...`. Current validators map
`legacy/java/<logical-path>` back to those locators before comparing immutable
Git blobs. This preserves provenance instead of rewriting historical evidence.
The archive is also marked `linguist-detectable=false`, so repository language
statistics describe the supported Go product rather than this retired source.

## Provenance and recovery

- Review baseline: `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`.
- Annotated archive tag: `archive/legacy-java-v19-20260829`.
- Historical Java main commit:
  `65f6622ea8dc65ab7f5c155c5b58e8c2db349637`.
- Relocated tracked set: 1,000 byte-preserved files.
- Reviewed runtime/build set: 890 files; archived `src/`: 866 files,
  including 780 Java files.

Read historical files without running them:

```powershell
git show archive/legacy-java-v19-20260829:pom.xml
git ls-tree -r --name-only archive/legacy-java-v19-20260829 -- src workers
```

The repository currently has no project-level license. Relocating historical
files does not grant new rights to build, distribute, or reuse them; third-party
notices inside individual historical artifacts retain their own meaning.

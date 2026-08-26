# Legacy platform/data review evidence

This directory is a deletion-oriented, read-only review of the committed Java
tree at `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`. It is not a migration plan.
The Java/MySQL migration chain, third PE, and `mindweaver-migrate` command are
out of scope and remain dropped.

The frozen scope consists of:

- every Java file under `config`, `security`, `common`, `state`, `entity`,
  `repository`, `storage`, `mq`, and `scheduler`;
- every Java test in the same packages;
- every Java test that directly imports one of those packages;
- tests that directly qualify Spring context, application profiles, or Flyway;
- Flyway migrations V1 through V33; and
- every `application*.properties` production profile.

`frozen-file-manifest.csv` records the exact byte SHA-256, byte size, line
count, and full review range for every included file. Regenerate it only from
a descendant whose complete Java and resource inputs still match the frozen
commit:

```powershell
pwsh -NoProfile -File docs/rewrite/evidence/legacy-platform-review/freeze-manifest.ps1
```

The default disposition is `DROP`: Java implementation, Spring wiring, JPA
shape, Rabbit delivery state, scheduler behavior, Flyway history, and profile
values are not authoritative inputs to the Go product. A review document may
retain a user-visible invariant only when the fresh-Vault Go core already owns
and tests the complete behavior. `KEEP` never means porting a Java class,
schema, query, or configuration key.

# Windows browser qualification sandbox on 74d56a4

## Identity and decision

This review starts from committed mainline
`74d56a436e14991b44a091eafbd8e20514244649`. The implementation commit is
`13f3726e6a7a98fe447a9f2095b2a28b420465ad` and changes only
`v2/tests/browser/**`.

The old `BROWSER_PROCESS_SANDBOX_NOT_IMPLEMENTED` prerequisite combined two
different boundaries. This slice closes the reusable Windows process and
artifact-capability primitive. It deliberately does not invent an executable
launch profile for an unknown future browser/driver family. When an exact
approved artifact tuple is present and Windows Job construction succeeds, the
next stable prerequisite is therefore
`BROWSER_LAUNCH_PROFILE_NOT_APPROVED`. No product process is started on that
path.

UI-001 and UI-002 remain BLOCKED. The empty repository approval still returns
`BROWSER_ARTIFACT_NOT_APPROVED` before opening an artifact or crossing any
process boundary. The controlled fake harness remains permanently
`CONTROLLED_HARNESS_NOT_QUALIFIED` and cannot construct the package-private
real-run proof required for PASS.

## Closed local boundary

- Artifact bundle roots and all three executable leaves are opened as retained
  Windows handles on a fixed local volume. Network, device and non-fixed drive
  paths fail closed. Opens include `FILE_FLAG_OPEN_NO_RECALL`; handles with
  `OFFLINE`, `RECALL_ON_OPEN` or `RECALL_ON_DATA_ACCESS` attributes are rejected.
- Root and files must be owned by the current qualification identity and have a
  protected DACL containing exactly one full-control allow ACE for that owner.
  The runner does not repair or broaden an artifact ACL.
- Retained handles permit read sharing only, denying concurrent write, delete
  and replacement opens. Root/file identity, type, reparse state, link count,
  byte size, SHA-256 and ACL are rechecked from the retained capabilities.
- The sandbox creates one unnamed Windows Job with
  `KILL_ON_JOB_CLOSE`, `DIE_ON_UNHANDLED_EXCEPTION`, an active-process ceiling
  of 64 and a 2 GiB aggregate job-memory ceiling.
- Only the package-owned logical roots `driver` and `mindweaver` are accepted.
  A root is created suspended, assigned to the Job and only then resumed.
  Caller-supplied Windows process attributes, inherited handles, duplicate
  roots and a forged runner-created `browser` root are rejected.
- The browser remains a driver descendant. The existing process-evidence
  contract requires exactly `driver`, `browser` and `mindweaver`, exact approved
  hashes, and `browser.ParentPID == driver.PID`; literal-loopback endpoint
  validation remains unchanged.
- Cleanup terminates the Job and polls `JobObjectBasicAccountingInformation`
  until the OS reports `ActiveProcesses == 0` before closing the Job handle and
  reaping Go root handles. A child-ready handshake plus live accounting proves
  a descendant joined the Job; cleanup no longer relies on waiting only for two
  roots or on a fixed post-close sleep.
- A typed approved launch plan selects only the retained `driver` or
  `mindweaver` artifact. While the new process is still suspended, the runner
  obtains PID and parent from Windows, queries the process image, reopens it
  with the same hardened policy, compares file identity and SHA-256, and calls
  `IsProcessInJob`. Harness-supplied identity is not accepted by this launcher.
- The availability decision runs a real suspended root through assignment,
  membership verification and resume; that root creates a ready descendant,
  and the probe succeeds only after OS accounting reaches zero during cleanup.

No process output, executable path, user profile, Vault path, cookie, CSRF
value, prompt, document content or environment value is added to the report.
No browser or driver is downloaded or taken from ambient system state.

### DOS drive-alias review

The fixed-local decision does not trust a drive-letter string alone. It rejects
UNC and device prefixes, requires `GetDriveType(<letter>:\)` to return
`DRIVE_FIXED`, obtains the opened object's final path from its retained handle,
and requires the lexical and final DOS volumes to match. A `SUBST` or
`DefineDosDevice` mapping to a path on another volume or to a remote target is
therefore rejected by the drive-type or final-volume checks.

An additional DOS name that resolves to the same physical fixed volume still
classifies as fixed-local; that is not treated as proof that the mutable DOS
namespace itself is an authorization capability. The opened file handle pins
the approved object and its read-only sharing mode prevents replacement while
held. A future real launcher must derive/recheck the executable identity from
that retained capability and an approved canonical launch path immediately
before suspended creation. It must not authorize a later raw drive-letter path
solely because this fixed-local check once passed. This launch binding is part
of the still-blocked browser-family launch profile, and no real process is
started by this commit.

## P0 / P1 review

P0 is zero for the implemented primitive. A process cannot execute in the
runner-owned start path before Job assignment, retained artifacts cannot be
replaced through a sharing open, and empty approval cannot reach the primitive.

The original `13f3726` candidate was rejected by independent review and is not
eligible for integration by itself. The follow-up rewrite closes the four named
findings: OS job-zero cleanup, child-ready proof, typed artifact-bound process
identity plus Job membership, a full lifecycle availability probe, and
no-recall/offline artifact rejection. This document does not claim that every
P1 for a future real-browser adapter is zero.

The following are qualification prerequisites, not claims closed by this
slice:

1. a repository-approved, redistribution-permitted browser and matching driver
   tuple;
2. an approved browser-family launch profile, including bounded arguments and
   a fresh private profile directory;
3. the real `mindweaver.exe`, fresh-Vault and literal-loopback fake-Ollama
   lifecycle wired to the sandbox;
4. OS-observed PID/parent/hash evidence for the real three-role process tree and
   all 13 independent WebDriver scenarios.

Until those inputs exist, the real harness and package-private PASS proof remain
unimplemented. The platform primitive must not be interpreted as UI
qualification or release evidence.

## Offline gates

The focused implementation gate on Windows amd64 used the pinned Go 1.27.0
toolchain without a browser, model, user Vault or network access:

```text
go test ./tests/browser/... -count=10   PASS
go vet ./tests/browser/...              PASS
scripts/test-browser.ps1 -SelfTest x10  PASS, BLOCKED_AS_DESIGNED
go test ./...                           PASS
go vet ./...                            PASS
scripts/ci.ps1                          PASS
scripts/verify-standalone.ps1           PASS
```

The later mainline `57e293bb2f3d164dfda9d9840f1227dce5517f07` has the same
merge base (`74d56a4`) and changes no browser qualification or browser evidence
file. Read-only overlap review therefore found no textual conflict. Minimal
selective integration is the implementation commit first, followed by this
evidence-only commit; the focused and full gates must still be rerun on the
resulting mainline rather than inferred from this branch.

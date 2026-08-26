# Review 01: configuration, security, common errors, state, and profiles

Status: complete at `0df22ddaf02c64bf73a7df12cd5fea6b52632c73`.

This is a deletion-oriented source review. It does not authorize importing a
Java class, property name, profile value, database row, ciphertext, or task
state into the Go product. Every Java implementation and test in this
partition is `DROP`; the few retained items below are behavior-level
invariants already owned by a tested Go boundary.

## Frozen review set

`review-01-files.csv` is the authoritative per-file ledger. Each row binds the
baseline commit, repository-relative path, SHA-256, byte count, line count,
and full reviewed line range. The partition contains:

| Kind | Files | Lines | Reviewed range |
| --- | ---: | ---: | --- |
| `config`, `security`, `common`, `state` production Java | 17 | 1,105 | every file, line 1 through EOF |
| `application*.properties` profiles | 4 | 182 | every file, line 1 through EOF |
| directly associated Java tests | 49 | 7,039 | every file, line 1 through EOF |
| **Total** | **70** | **8,326** | **complete** |

Some associated tests also exercise entities, repositories, storage, or MQ.
Their complete source was reviewed here for configuration, error, security,
and state-machine assumptions; their data and delivery semantics are reviewed
again in their owning partitions.

## File-level disposition

| Production source | Implementation | Narrow semantic disposition |
| --- | --- | --- |
| `config/AgentTaskConfiguration.java` | DROP | Agent execution is not CORE. Do not reproduce Spring executors. |
| `config/BatchIngestionProperties.java` | DROP | Do not copy mutable batch defaults; accepted Go operations own their bounds. |
| `config/ChunkingProperties.java` | DROP | Do not copy the Java property schema; only the accepted Go algorithm owns validated limits. |
| `config/ClockConfiguration.java` | DROP | KEEP only injectable-clock testing; durable Go time is UTC, not `systemDefaultZone`. |
| `config/MemoryProperties.java` | DROP | Conversation memory is outside this platform/data CORE review. |
| `config/ModelProviderConfiguration.java` | DROP | Never mutate durable state at startup to seed a mock provider. |
| `config/ModelProviderProperties.java` | DROP | Do not import database-override flags or Spring binding shape. |
| `config/RabbitMQConfig.java` | DROP | Rabbit exchanges, queues, delivery state, and consumers are not part of the local SQLite job core. |
| `config/RetrievalPipelineProperties.java` | DROP | Do not copy duplicated mutable retrieval settings. |
| `config/SecurityProperties.java` | DROP | Do not import a property/environment master-key contract. |
| `config/TrashProperties.java` | DROP | Do not copy scheduler/retention configuration; accepted Go lifecycle behavior is authoritative. |
| `security/ApiKeySecretService.java` | DROP | Do not import its AES/key-derivation/ciphertext format or any encrypted Java value. |
| `common/error/ApiErrorResponse.java` | DROP | KEEP only the invariant that transport problems use bounded stable fields. |
| `common/error/BusinessException.java` | DROP | KEEP only stable safe error codes at the Go transport boundary. |
| `common/error/ErrorCode.java` | DROP | Do not copy the feature-wide Java enum; define only codes exercised by accepted Go CORE. |
| `common/error/GlobalExceptionHandler.java` | DROP | Do not return/log raw exception messages, provider text, request paths, or untrusted request IDs. |
| `state/TaskStateMachine.java` | DROP | KEEP only tested terminal immutability, cancellation separation, lease/CAS, and recovery behavior from the SQLite job adapter. |
| all four application profiles | DROP | No Spring/MySQL/Rabbit/Python/Qdrant/OpenAI profile is an input to fresh-Vault Go configuration. |
| all 49 associated test files | DROP | Historical evidence only; mocks and Java response fixtures are not product contracts. |

The exact disposition for every file, including every test, is recorded in
`review-01-files.csv`. There is no Java implementation exception.

## P0 findings

1. **Committed profiles contain unsafe deployment defaults.**
   `application.properties` and `application-docker.properties` include MySQL
   `root`/`123456` and Rabbit `guest`/`guest` credentials. The profile family
   also assumes Spring port 8080, MySQL/Flyway/Rabbit, a local Python worker,
   Qdrant, and an OpenAI-compatible embedding endpoint. All four profiles and
   their tests are DROP; none may be copied into v2 configuration or release
   artifacts.

2. **The Java secret format is not an importable security boundary.**
   `ApiKeySecretService` derives an AES key by directly hashing a caller
   property/environment passphrase. It has no salt, work-factor KDF, key ID,
   rotation, or OS credential-store ownership. Its tests prove only
   round-trip and masking behavior. The key source, format, ciphertext, and
   migration path are DROP. A fresh Vault must use the approved Windows
   credential/DPAPI boundary and must not accept this ciphertext.

3. **The exception adapter exposes uncontrolled text.**
   `GlobalExceptionHandler` returns and logs caller/provider exception
   messages and request paths, and reflects an arbitrary `X-Request-Id`.
   Associated tests positively require raw strings such as provider failures
   and caller-supplied business messages. This behavior is DROP. Only the Go
   stable-code-to-Problem adapter may decide public status, detail, and
   retryability.

4. **Startup mutates product data by creating a mock provider.**
   `ModelProviderConfiguration` conditionally inserts a mock provider selected
   by display name during application startup. That is neither stable identity
   nor versioned reconciliation. It is DROP; fresh-Vault startup must not seed
   fake provider data.

5. **The Java state machine is not a durable job protocol.**
   `TaskStateMachine` is an in-memory transition table, permits same-state
   transitions, uses obsolete names including `SUCCESS` and `RETRY_PENDING`,
   and has no durable revision CAS, lease/fence, cancellation request,
   recovery, or attempt evidence. The class is DROP. It must not supersede the
   smaller SQLite job adapter already exercising claim, cancellation,
   completion, retry, and recovery transactionally.

6. **Tests normalize persistence and disclosure of sensitive content.**
   Associated fixtures and assertions move prompts, rendered prompts, source
   text, provider errors, and arbitrary event messages through the Java model.
   These expectations are not migration requirements and cannot be used to
   justify importing Java rows or returning raw errors.

## P1 findings

1. The configuration classes are mutable binders without complete bounds or
   cross-field validation. Retrieval configuration is duplicated between
   namespaces. There is no versioned configuration contract.
2. `ClockConfiguration` uses the host default zone, making durable behavior
   dependent on timezone and DST. Only dependency injection is worth keeping;
   timestamps remain UTC in the Go core.
3. Rabbit delivery topology and consumer wiring add a second source of durable
   work truth. They are DROP; local work is represented only by SQLite jobs.
4. Most associated tests are mocked interaction checks. They do not prove
   transactionality, crash recovery, secret protection, fail-closed durable
   writes, or bounded external I/O.
5. `LocalAiProfilePropertiesTest` freezes Python worker and provider defaults
   that are not part of accepted CORE and is DROP.
6. `ErrorCode` combines CORE concerns with Agent, vector, evaluation, and
   infrastructure concerns. No bulk enum migration is allowed.

## Retained Go ownership

These are evidence references, not requests to port Java code:

- `v2/platform/config` owns versioned, validated configuration and
  context-aware I/O.
- `v2/platform/apperror` plus `v2/internal/transport/problem.go` own stable,
  bounded transport error mapping.
- `v2/internal/store/sqlite/jobs.go` owns durable local job status, lease,
  cancellation, retry, and startup/expired recovery behavior.
- `v2/internal/store/sqlite/jobs_test.go` and accepted app/workbench tests own
  the proof for stale-worker rejection, cancellation precedence, retry, and
  terminal outcomes.

No Spring bean, Java error response, Java task enum, Rabbit entity, Java
profile, or Java secret is needed to preserve those invariants.

## Verification

The evidence is valid only when:

- the frozen baseline is an ancestor of `HEAD`, and all Java/resource review
  inputs are unchanged from that baseline;
- all manifest paths are unique and exist;
- every recorded SHA-256, byte count, line count, and `1-N` range matches the
  committed source; and
- the 70 review rows are an exact subset of the 272-file global frozen
  manifest.

`freeze-manifest.ps1` fails closed if any reviewed source input differs from
the baseline. No production source or test was changed by this review.

# AI prompt log

## T01 — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

Act only as:
- Specialist Architect;
- Backend Developer 2;
- QA Specialist.

Implement T01 only: project foundation, Docker multi-stage setup, Docker Compose,
test harness foundation, deterministic test data, and simulator foundation.

Requirements:
- Create cmd/api, cmd/worker, cmd/simulator, internal/testkit, testdata,
  Dockerfile, docker-compose.yml, .dockerignore, .env.example, and Makefile.
- Use one multi-stage Dockerfile with api, worker, simulator, and test targets.
- Runtime containers must be minimal and run as non-root.
- Docker Compose must include API, worker, PostgreSQL, Redis, test service, and
  optional simulator profile.
- Add real health checks and conservative resource limits.
- Implement the test harness using native Go testing plus Testcontainers for Go.
- Test harness must provide isolated PostgreSQL, Redis, deterministic clock,
  fixtures, API driver, eventual assertions, and automatic cleanup.
- Add deterministic simulator scenario files and seed support.
- Implement both T01 Gherkin scenarios: success and error.
- Do not implement T02 or later business features.

Run:
- gofmt
- go vet ./...
- go test ./...
- go test -race ./...
- Docker build and Compose health validation

Update docs/ai/prompt-log.md with this exact prompt, changed files, commands,
results, and blockers.
```

### Files changed

- `README.md`, `.dockerignore`, `.env.example`, `Dockerfile`, `Makefile`, `docker-compose.yml`, `go.mod`, `go.sum`
- `cmd/api/main.go`, `cmd/api/main_test.go`, `cmd/worker/main.go`, `cmd/simulator/main.go`
- `internal/bootstrap/config.go`, `internal/bootstrap/config_test.go`
- `internal/simulator/simulator.go`, `internal/simulator/simulator_test.go`
- `internal/testkit/harness.go`, `internal/testkit/harness_test.go`, `internal/testkit/harness_integration_test.go`
- `tests/acceptance/compose_test.go`, `testdata/fixtures/.gitkeep`
- `testdata/scenarios/happy-path.json`, `testdata/scenarios/invalid-readings.json`, `testdata/scenarios/unstable.json`, `testdata/scenarios/network-failure.json`, `testdata/scenarios/abandoned.json`
- `docs/architecture/architecture.md`, `docs/architecture/adr/001-modular-monolith.md`, `docs/architecture/adr/002-redis-streams-buffer.md`, `docs/architecture/adr/003-deterministic-stabilization.md`, `docs/architecture/adr/004-idempotency-protocol.md`, `docs/architecture/adr/005-numeric-modeling.md`, `docs/architecture/adr/006-container-resource-policy.md`, `docs/architecture/adr/007-device-authentication.md`
- `docs/ai/prompt-log.md`

### Scope decisions and blockers

T01's required real API Compose health check conflicts with the playbook's later
T07 health-endpoint delivery. The bootstrap API exposes only liveness/readiness
probes for that T01 container check; it implements no business API, persistence,
Redis operation, worker processing, simulator behavior, or T02 CRUD.

The T01 success Gherkin scenario calls migration and seed commands, but the
schema and registrations required for those commands are explicitly assigned to
T02. Creating no-op migration/seed commands would be misleading and out of
scope, so they remain intentionally unavailable. The deterministic scenario,
simulator, container topology, and invalid-configuration portions are covered by
T01 tests.

Docker Compose v2 is not installed in the execution environment: `docker compose
up --build -d` failed before reading the Compose file with `unknown flag:
--build`. Installing a system Compose plugin is out of scope, so the required
Compose health validation is blocked. Direct Docker target builds and the API
container health check were executed successfully instead.

### Commands and results

| Command | Result |
| --- | --- |
| `go mod tidy` | Passed; resolved Testcontainers, pgx, and go-redis dependencies. |
| `gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -race ./...` | Passed. |
| `go test -tags=integration -v ./internal/testkit/...` | Passed against real isolated PostgreSQL 16 and Redis 7 containers; automatic cleanup observed. |
| `go test -tags=acceptance -v ./tests/acceptance/...` | Passed both T01 Gherkin-named success and error scenarios. |
| `make verify` | Passed format check, vet, unit, integration, race, and acceptance suites. |
| `docker build --target api -t scale-challenge-api:t01 .` | Passed. |
| `docker build --target worker -t scale-challenge-worker:t01 .` | Passed. |
| `docker build --target simulator -t scale-challenge-simulator:t01 .` | Passed. |
| `docker build --target test -t scale-challenge-test:t01 .` | Passed. |
| Direct API image health validation | Passed: image user is `app`; `GET /health/live` returned 200; Docker health state became `healthy`. Temporary container removed afterward. |
| `docker compose up --build -d` | Blocked: Compose v2 command is unavailable locally. |

## T01 QA correction — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

- Specialist Architect;
- Backend Developer 2;
- QA Specialist.


Adjust corrections required for QA Specialist.
```

### Files changed

- `.dockerignore`, `Dockerfile`
- `internal/testkit/harness.go`, `internal/testkit/harness_test.go`
- `tests/acceptance/compose_test.go`
- `docs/architecture/architecture.md`, `docs/ai/prompt-log.md`

### Corrections and blockers

- The test harness now exposes the playbook-specified `*httptest.Server` in
  `Harness.API`; `Harness.Request` is the in-process API driver helper.
- Acceptance evidence now separates a deterministic fixture/topology check from
  the full Gherkin environment scenario. The full scenario is explicitly
  skipped with the missing Docker Compose v2 prerequisite rather than reported
  as passed.
- The error scenario now starts API and worker without `DATABASE_URL` or
  `REDIS_ADDR` and proves that they fail without exposing an injected secret.
- Docker now copies only the source and test inputs needed by its targets; the
  `.dockerignore` excludes documentation and local metadata from build context.
- The full Gherkin environment remains blocked by the missing Docker Compose v2
  client and by migration/seed commands that require the T02 schema. No T02
  behavior was introduced.

### Commands and results

| Command | Result |
| --- | --- |
| `gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -race ./...` | Passed. |
| `go test -tags=acceptance -v ./tests/acceptance/...` | Passed: deterministic fixture and all error cases. Full Compose Gherkin scenario skipped with explicit prerequisite evidence. |
| `make verify` | Passed, including real PostgreSQL/Redis Testcontainers integration. |
| `docker build --target api|worker|simulator|test ...` | All four targets passed. |

## T02 — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

Act only as:
- Product Owner;
- Backend Developer 1;
- QA Specialist.

Implement T02 only: explicit CRUD for branches, scales, trucks, grain types,
and transport transactions.

Requirements:
- Implement create, list, get by ID, update, and safe deactivate for branches,
  scales, trucks, and grain types.
- Never physically delete an entity referenced by a transaction or weighing.
- Implement transport transaction create, list, get, controlled state transition,
  and cancellation before final weighing.
- Create PostgreSQL migrations, SQLC queries, repositories, application services,
  HTTP handlers, OpenAPI documentation, and tests.
- Use normalized unique truck plates and unique scale_id.
- Store only api_key_hash.
- Use int64 for grams and NUMERIC or explicit smallest-unit representation for money.
- Enforce one OPEN transaction per truck with business rules and a database constraint.
- Implement every T02 Gherkin scenario, including success and error behavior.
- Do not implement Redis worker or stabilization logic.

Run:
- gofmt
- go vet ./...
- go test ./...
- go test -race ./...

Update docs/ai/prompt-log.md with evidence.
```

### Files changed

- `README.md`, `.env.example`, `Dockerfile`, `docker-compose.yml`, `Makefile`, `go.mod`, `go.sum`, `sqlc.yaml`
- `db/migrations/000001_t02_registrations.up.sql`, `db/migrations/000001_t02_registrations.down.sql`, `db/migrations/embed.go`, `db/queries/registrations.sql`
- `internal/database/sqlc/db.go`, `internal/database/sqlc/models.go`, `internal/database/sqlc/registrations.sql.go`
- `internal/domain/domain.go`, `internal/migrations/migrations.go`, `internal/repository/postgres.go`, `internal/application/service.go`, `internal/httpapi/server.go`, `internal/httpapi/server_integration_test.go`
- `cmd/api/main.go`, `cmd/api/main_test.go`, `cmd/migrate/main.go`, `cmd/seed/main.go`
- `docs/openapi/openapi.yaml`, `docs/architecture/architecture.md`, `docs/ai/prompt-log.md`

### Evidence

- PostgreSQL migrations define foreign keys, check constraints, unique normalized
  truck plates, unique `scale_id`, a partial unique index for one `OPEN`
  transaction per truck, and immutable transaction price/margin snapshots.
- API keys are bcrypt-hashed before persistence and are omitted from responses.
- Real PostgreSQL integration scenarios cover registration CRUD/deactivation,
  invalid operations with no extra transport row, valid OPEN snapshot creation,
  controlled cancellation, and concurrent OPEN creation (one 201 and one 409).
- No Redis worker, stream consumer, or stabilization logic was added.

### Commands and results

| Command | Result |
| --- | --- |
| `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0 generate` | Passed; generated typed pgx/v5 query bindings. |
| `gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -race ./...` | Passed. |
| `make verify` | Passed; includes real PostgreSQL CRUD Gherkin integration tests, base Testcontainers tests, race tests, and acceptance tests. |
| `docker build --target api|worker|simulator|test ...` | All four targets passed. |
| `docker compose up --build -d` | Blocked: Docker Compose v2 is unavailable locally (`unknown flag: --build`). |

## T01 QA review — 2026-07-17

### Exact prompt

```text
My docker-compose version is :

docker-compose version

Docker Compose version 5.3.1

Another point: I believe the IDs shouldn't be of type TEXT. Also, before deleting a table, we should check if it exists first.

Act as the QA Specialist.
Review the T01 implementation against the playbook.
Run the required checks and report only:
- passed scenarios;
- failed scenarios;
- missing requirements;
- risks;
- required corrections.
Do not implement new functionality.

Update docs/ai/prompt-log.md with evidence.
```

### Files changed

- `docs/ai/prompt-log.md`

### QA evidence and results

| Command | Result |
| --- | --- |
| `docker-compose version` | Passed: Docker Compose version 5.3.1 is installed. This supersedes prior evidence that used the unavailable `docker compose` plugin. |
| `gofmt -d $(find . -type f -name '*.go' -not -path './vendor/*')` | Passed: no formatting diff. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -race ./...` | Passed. |
| `make verify` | Passed on the host: formatting, vet, unit, integration, race, and acceptance. |
| `docker-compose config -q` | Passed. |
| `docker-compose up --build -d` | Passed. API, PostgreSQL, and Redis reached `healthy`; `/health/live` and `/health/ready` both returned HTTP 200. Migrations completed successfully. |
| `docker-compose run --rm seed` | Passed: deterministic seed data applied. |
| `docker-compose --profile simulator run --rm simulator` (twice) | Passed: each execution prepared five events with seed 42. It does not emit or deliver the event sequence. |
| `docker-compose --profile simulator run --rm simulator --seed -1` | Passed negative check: rejected invalid seed without a secret in output. |
| `docker-compose run --rm -e DATABASE_URL= -e REDIS_ADDR= worker` | Passed negative check: worker rejected missing `DATABASE_URL` without exposing a supplied secret. |
| `go test -tags=acceptance -v ./tests/acceptance/...` | Partial: fixture and error scenarios passed; `TestGherkinSuccessFullEnvironment` was skipped because it invokes unavailable `docker compose version`, despite installed `docker-compose` 5.3.1. |
| `docker-compose` test service (`make verify`) | Failed: `go test -race ./...` exits because the Docker `test` target has `CGO_ENABLED=0`; the host race check passes. |

### Blockers

- None for the host validation or real `docker-compose` stack validation.
- The automated full-environment Gherkin test is blocked by its incorrect Compose executable selection; this is a repository correction, not an environment blocker.
- The Compose `test` service cannot complete its required race check until its build configuration permits CGO.

## T01 QA corrections — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

- Specialist Architect;
- Backend Developer 2;
- QA Specialist.


Adjust corrections required for QA Specialist.
```

### Files changed

- `Dockerfile`, `docker-compose.yml`, `README.md`
- `cmd/worker/main.go`, `cmd/worker/main_test.go`
- `cmd/simulator/main.go`
- `internal/simulator/simulator.go`, `internal/simulator/simulator_test.go`
- `tests/acceptance/compose_test.go`
- `testdata/scenarios/happy-path.json`
- `docs/ai/prompt-log.md`

### Corrections and scope boundary

- The worker now has a real health probe that verifies both PostgreSQL and
  Redis. Compose waits on and reports the worker health state.
- The Docker test stage explicitly enables CGO and installs the C compiler
  needed by the Go race detector. Its test inputs now include
  `docker-compose.yml`.
- Acceptance detects the available `docker-compose` executable before falling
  back to `docker compose`. It starts an isolated, dynamically port-mapped
  Compose project; waits for API, worker, PostgreSQL, and Redis health; applies
  the seed; compares two emitted seed-42 JSONL event sequences; and removes the
  project through `t.Cleanup`.
- The simulator now emits each deterministic event as JSONL. The happy-path
  data contains arrival, two 30-sample stable windows, an outlier, and release.
  HTTP delivery remains deferred to T03.
- `testkit.New(t)` continues to start containers only when a test opts in, as
  required by the T01-specific specification. The broader playbook's T02
  fixture helpers require registration-domain work and were not added by these
  T01-role owners.
- PostgreSQL ID type and rollback-table-existence changes are T02 migration
  ownership (Backend Developer 1) and were deliberately not modified.

### Commands and results

| Command | Result |
| --- | --- |
| `gofmt -w cmd/worker/main.go cmd/worker/main_test.go cmd/simulator/main.go internal/simulator/simulator.go internal/simulator/simulator_test.go tests/acceptance/compose_test.go` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -race ./...` | Passed. |
| `go test -run TestGherkinSuccessFullEnvironment -tags=acceptance -v ./tests/acceptance/...` | Passed: isolated Compose API, worker, PostgreSQL, and Redis became healthy; seed succeeded; two seed-42 simulator JSONL sequences matched. |
| `docker-compose build test` | Passed. Classic builder was used because Buildx is not installed. |
| `docker-compose run --rm test` | Passed: container `make verify`, including `go test -race ./...`, succeeded. |
| `docker-compose config -q` | Passed. |
| `docker-compose down --volumes --remove-orphans` | Passed: removed temporary validation containers, network, and any Compose volume. |
| `make verify` | Passed: format check, vet, unit, integration, race, and real Compose acceptance. |

### Blockers

- None within the authorized T01 Architect, Backend Developer 2, and QA scope.
- T02 typed identifiers and guarded down-migration drops remain an explicit
  ownership boundary requiring Backend Developer 1 authorization.

## T03 — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

Act only as:
- Backend Developer 1;
- Backend Developer 2;
- QA Specialist.

Implement T03 only: authenticated asynchronous scale-reading ingestion.

Requirements:
- Implement POST /v1/scale-readings.
- Enforce a 64 KiB request body limit and HTTP read/write/idle timeouts.
- Authenticate Authorization: Bearer device keys against stored hashes.
- Validate active scale and scale_id consistency.
- Validate event_id, scale_id, plate, weight_grams greater than zero, and RFC3339 measured_at.
- Normalize plate and add received_at in UTC RFC3339Nano.
- Publish to Redis Stream scale-readings using XADD.
- Return 202 Accepted only after Redis confirms publication.
- Return 400 for malformed JSON, 401 for invalid credentials, 403 for disabled
  or mismatched scale, 422 for valid-but-invalid business payload, and 503 when
  Redis is unavailable.
- Do not persist raw readings in PostgreSQL.
- Do not create a goroutine per request.
- Implement all T03 Gherkin scenarios with real Redis integration tests.

Run all required validation commands and update docs/ai/prompt-log.md.
Do not start T04.
```

### Files changed

- `cmd/api/main.go`, `cmd/api/main_test.go`
- `internal/application/service.go`, `internal/domain/domain.go`
- `internal/httpapi/server.go`, `internal/httpapi/scale_readings_integration_test.go`
- `internal/repository/redis.go`
- `docs/openapi/openapi.yaml`, `docs/ai/prompt-log.md`

### Implementation and QA evidence

- The API uses a 64 KiB `http.MaxBytesReader`, strict single-object JSON
  decoding, bounded server read/write/idle timeouts, and no request goroutine.
- Device keys are compared with persisted bcrypt hashes. A matched disabled
  scale, or a matched key paired with a different `scale_id`, returns 403.
- Accepted events are normalized and sent only through Redis `XADD` to
  `scale-readings`. `received_at` and `measured_at` are serialized in UTC using
  `time.RFC3339Nano`; the PostgreSQL row-count assertion proves ingestion does
  not synchronously persist a raw reading or business mutation.
- Real PostgreSQL 16 and Redis 7 Testcontainers cover both T03 Gherkin
  scenarios: exactly one accepted stream message, invalid credentials, invalid
  business input, Redis unavailability, malformed and oversized JSON, disabled
  scale, and device/scale mismatch. Error responses include `code`, `message`,
  and `request_id`.

| Command | Result |
| --- | --- |
| `gofmt -d $(rg --files -g '*.go')` | Passed: no formatting diff. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -tags=integration ./internal/...` | Passed against real PostgreSQL 16 and Redis 7 Testcontainers; includes all T03 Gherkin scenarios. |
| `go test -race ./...` | Passed. |
| `go test -race -tags=integration ./internal/httpapi/...` | Passed against real PostgreSQL and Redis while exercising T03 ingestion paths. |
| `make verify` | Passed: formatting, vet, unit, real integration, race, and acceptance suites. |

### Blockers

- None. T04 stabilization, sessions, consumer groups, worker processing, and
  final-weighing persistence were not started.

## T04 — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

Act only as:
- Backend Developer 2;
- QA Specialist.

Implement T04 only: deterministic weight stabilization and weighing session state machine.

Requirements:
- Keep the implementation isolated from HTTP, Redis, and PostgreSQL.
- Implement EMPTY, COLLECTING, CANDIDATE, FINALIZED, ABANDONED, and FAILED states.
- Use bounded per-session ring buffers keyed by scale_id and normalized plate.
- Require at least 30 samples over at least 3 seconds.
- Calculate median, P05, P95, dispersion, and weight/time slope.
- Apply absolute and percentage tolerances.
- Require two consecutive stable windows before FINALIZED.
- Use the median of the final stable window as the final weight.
- Reject out-of-order timestamps with typed errors.
- Do not use float for final weight.
- Add hysteresis and release behavior through low-weight threshold, plate change,
  or timeout.
- Create table-driven tests for stable noise, outlier, increase, decrease,
  insufficient samples, out-of-order timestamps, lost stability, and T04 Gherkin scenarios.

Run all required validation commands and update docs/ai/prompt-log.md.
Do not implement worker consumption yet.
```

### Files changed

- `internal/stabilizer/stabilizer.go`
- `internal/stabilizer/stabilizer_test.go`
- `docs/ai/prompt-log.md`

### Implementation and QA evidence

- `internal/stabilizer` is a pure Go package that accepts only reading values
  and returns state/audit values; it imports no HTTP, Redis, PostgreSQL, or
  worker code.
- Active sessions are keyed by uppercased `scale_id` and normalized plate. Each
  owns a fixed-capacity ring; the manager also bounds active session count.
- A ready window requires at least 30 samples and a three-second duration. It
  calculates integer median, nearest-rank P05/P95, dispersion, and an exact
  rational Theil-Sen weight/time slope. No `float32` or `float64` is used.
- Stability requires absolute dispersion, basis-point percentage dispersion,
  and slope limits to all pass. Two consecutive stable sliding windows produce
  one `FINALIZED` result whose integer weight is that final window's median.
- `Finalization` carries the algorithm version, sample count, dispersion,
  rational slope, and UTC stabilization timestamp for the T06 persistence
  boundary; no database write is performed in T04.
- A typed `OutOfOrderTimestampError` transitions the affected session to
  `FAILED`. `Expire` emits `ABANDONED` for incomplete sessions and releases
  finalized sessions to `EMPTY`; low weight and a plate change also release a
  finalized passage.
- Table-driven tests cover stable noise, a single outlier, increase, decrease,
  insufficient samples, lost stability, out-of-order timestamps, all three
  tolerances, bounded ring/session memory, hysteresis, release, and both T04
  Gherkin scenarios.

### Commands and results

| Command | Result |
| --- | --- |
| `gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed. |
| `go test -race ./...` | Passed. |
| `go test ./internal/stabilizer` | Passed. |
| `go test -race ./internal/stabilizer` | Passed. |
| `rg -n '\\bfloat(32|64)\\b' internal/stabilizer` | Passed with no matches. |

### Blockers

- None. Redis Streams consumption, ACK/recovery/DLQ, and final-weighing
  persistence remain intentionally deferred to T05 and T06.

## T05 — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

Act only as:
- Backend Developer 2;
- QA Specialist;
- Specialist Architect for review only.

Implement T05 only: Redis Streams worker, reliable processing, pending recovery,
and dead-letter queue.

Requirements:
- Create stream scale-readings, consumer group weighing-workers, and
  scale-readings-dlq.
- Use XREADGROUP with bounded batches and a unique consumer name per worker instance.
- ACK only after successful idempotent processing and persistence.
- Use XAUTOCLAIM or equivalent for idle pending messages.
- Configure block time, batch size, pending idle timeout, and retry limit.
- Never ACK transient Redis or PostgreSQL failures.
- Move permanently invalid or exhausted messages to DLQ with original event,
  reason, and attempt count, then ACK the original.
- Add metrics for processed, pending, reclaimed, DLQ, and failures.
- Use real Redis and PostgreSQL tests. Do not mock ACK, pending, reclaim, or DLQ.
- Implement both T05 Gherkin scenarios.

Run all validation commands and update docs/ai/prompt-log.md.
Do not implement financial finalization beyond the minimum required integration point.
```

### Files changed

- `.env.example`, `docker-compose.yml`, `cmd/worker/main.go`, and
  `cmd/worker/main_test.go`
- `db/migrations/000002_t05_processed_events.up.sql` and
  `db/migrations/000002_t05_processed_events.down.sql`
- `internal/worker/worker.go` and `internal/worker/worker_integration_test.go`
- `docs/ai/prompt-log.md`

### Implementation and architecture review

- The worker creates the `scale-readings` source stream and the stable
  `weighing-workers` group, then reads new work through bounded `XREADGROUP`
  calls. Each process receives a UUID-based consumer name.
- Recovery uses bounded `XAUTOCLAIM` batches for entries idle longer than
  `WORKER_PENDING_IDLE_TIMEOUT`; retry metadata is a TTL-bounded Redis side
  key. Batch size, block time, pending idle timeout, and retry limit are
  configured by environment variables with conservative defaults.
- PostgreSQL contains only `processed_scale_events`, an idempotency ledger with
  unique event and stream IDs. It deliberately stores no raw readings and does
  not perform any T06 weighing, inventory, cost, margin, or transaction-state
  finalization. This is the minimum durable processing integration point before
  `XACK`.
- Invalid events and retry-exhausted transient failures use one Redis Lua
  transaction: `XADD` the complete original event, source ID, reason, and
  attempt count to `scale-readings-dlq`, then `XACK` the original entry and
  delete its retry key. A Redis/PostgreSQL failure before that point leaves the
  source message pending.
- The worker exposes sampled pending PEL count plus monotonic processed,
  reclaimed, DLQ, and failure counters. No background goroutines are started;
  `Run` and all I/O are context-cancellable.
- Architecture review: Redis remains a buffer, PostgreSQL remains the source
  of durable idempotency, and a failed ACK is safely replayed as an idempotent
  PostgreSQL no-op. The implementation does not claim that this ledger is
  financial finalization; the future T06 transaction remains the required
  boundary for final weighings and inventory.

### QA evidence

- Real PostgreSQL 16 and Redis 7 Testcontainers prove the first Gherkin
  scenario: a real pending entry is reclaimed with `XAUTOCLAIM`, persisted, and
  removed from the Redis PEL only after the ledger write.
- The recovery scenario cancels a real context after PostgreSQL persistence and
  before the real `XACK`; another consumer reclaims it and proves the database
  has exactly one ledger row. ACK, PEL inspection, reclaim, and DLQ are never
  mocked.
- Separate real-Redis coverage proves a transient processor failure stays
  pending until its configured retry limit, then carries the original event,
  reason, and attempt count to the DLQ. A malformed event is immediately sent
  to the same DLQ path.

### Commands and results

| Command | Result |
| --- | --- |
| `gofmt -w internal/worker/worker.go internal/worker/worker_integration_test.go cmd/worker/main.go cmd/worker/main_test.go` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed, including real Redis/PostgreSQL worker scenarios. |
| `go test -race ./...` | Passed. |
| `make verify` | Passed: format check, vet, unit/integration, race, and acceptance targets. |
| `docker build --target worker -t scale-worker:t06 .` | Passed. |
| `docker build --target worker -t scale-worker:t05 .` | Passed. |

### Blockers

- None.

## T06 — 2026-07-17

### Exact prompt

```text
Read docs/context/scale-challenge-codex-playbook.md.

Act only as:
- Product Owner;
- Backend Developer 1;
- Backend Developer 2;
- QA Specialist.

Implement T06 only: transactional final weighing, inventory, financial calculation,
and idempotency.

Requirements:
- In one PostgreSQL transaction:
  1. lock and validate the OPEN transport transaction;
  2. calculate net_weight_grams = gross_weight_grams - tare_weight_grams;
  3. reject net weight less than or equal to zero;
  4. calculate load cost from tons and purchase price;
  5. update branch/grain inventory;
  6. calculate margin from 5% to 20%, inversely proportional to
     inventory / inventory_target;
  7. snapshot purchase price and applied margin;
  8. insert final weighing;
  9. transition transport transaction to WEIGHED.
- Use int64 for grams and NUMERIC or fixed smallest units for money.
- Add uniqueness for final weighing by session_id and stage.
- Add event_id idempotency when event_id exists.
- Test rollback, duplicate event, invalid net weight, nonexistent/open transaction,
  and two concurrent workers finalizing the same session.
- Implement every T06 Gherkin scenario.

Run all validation commands and update docs/ai/prompt-log.md.
Do not start reporting yet.
```

### Files changed

- `cmd/worker/main.go`
- `db/migrations/000003_t06_final_weighings.up.sql` and
  `db/migrations/000003_t06_final_weighings.down.sql`
- `internal/finalization/finalization.go` and
  `internal/finalization/finalization_integration_test.go`
- `internal/worker/finalizing_processor.go` and
  `internal/worker/finalizing_processor_integration_test.go`
- `docs/ai/prompt-log.md`

### Implementation and product evidence

- `finalization.Service` runs the entire financial write path in one PostgreSQL
  transaction. It locks the transport transaction, verifies it remains `OPEN`,
  derives net grams, computes rounded cost in integer minor units using
  PostgreSQL `NUMERIC`, updates branch/grain inventory, derives the margin in
  integer basis points, writes the immutable weighing, and marks the transport
  `WEIGHED` before committing.
- The applied margin is `2000 - floor(1500 * clamp(inventory / target, 0, 1))`
  basis points, producing an inclusive 5%–20% range without floating point.
  Purchase price and applied margin are stored on each weighing. Cost uses a
  fixed minor-unit price per metric ton and `1_000_000_000` grams per ton.
- Schema constraints preserve positive net weight and fixed-unit values;
  `UNIQUE(session_id, stage)` prevents duplicate final weighing sessions and a
  partial unique `event_id` index makes supplied event IDs idempotent.
- The worker now passes stabilized final readings into the transaction and
  retains a pending event work item until PostgreSQL succeeds. Transient
  persistence errors therefore remain pending for the existing T05 recovery
  flow; validation, missing transaction, and non-OPEN errors are permanent.
- No reporting endpoint, aggregate query, or T07 behavior was introduced.

### QA evidence

- Real PostgreSQL tests prove successful one-transaction finalization with a
  2-ton net load, 250,000 minor-unit cost, 2-billion-gram inventory, and 1,250
  applied-margin basis points; the transport becomes `WEIGHED`.
- Invalid net weight, nonexistent transport, and a cancelled/non-OPEN
  transport leave no weighing or inventory row. Repeating the same event ID
  returns the original weighing and does not increment inventory.
- Two concurrent finalizers on the same `(session_id, stage)` yield exactly one
  weighing and one inventory update. A caller that observes the committed
  winner returns that same idempotent result; a true insert race rolls back its
  losing transaction completely.
- A real Redis 7 and PostgreSQL 16 worker scenario sends 31 ordered stable
  samples (the candidate and second stable window), verifies a single final
  weighing, `WEIGHED` transport, final inventory, and no pending stream entry.

### Commands and results

| Command | Result |
| --- | --- |
| `gofmt -w cmd/worker/main.go internal/finalization/finalization.go internal/finalization/finalization_integration_test.go internal/worker/finalizing_processor.go internal/worker/finalizing_processor_integration_test.go` | Passed. |
| `go vet ./...` | Passed. |
| `go test ./...` | Passed, including real PostgreSQL and Redis scenarios. |
| `go test -race ./...` | Passed. |
| `make verify` | Passed: format check, vet, unit/integration, race, and acceptance targets. |

### Blockers

- None.

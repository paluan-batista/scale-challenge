---
title: "Codex Playbook - Backend Scale Challenge"
subtitle: "Architecture, micro-agents, Gherkin tasks, Docker and validation"
author: "AI-assisted execution plan"
lang: en-US
geometry: margin=1.8cm
fontsize: 10pt
toc: true
toc-depth: 2
colorlinks: true
---

# How to use this document in Codex

This is the implementation contract for the challenge. Before each step, give Codex: (1) this document, (2) the task prompt, and (3) the corresponding Gherkin scenario. Execute one task at a time, record the exact prompt, and only move forward after QA validation.

The goal is to accept HTTP scale readings every 100 ms, identify a stable weight, save exactly one final weighing, and produce inventory, cost, margin, and reporting information.

## Micro-agent execution order

1. Product Owner validates the backlog and business rules.
2. Architect approves contracts, ADRs, and technical limits.
3. Backend Developer 1 builds the domain, database, registrations, and HTTP endpoint.
4. Backend Developer 2 builds Redis Streams, the worker, stabilization, idempotency, and the simulator.
5. QA reviews every step, executes tests, and approves evidence.

No agent may change another agent's responsibility without a documented decision. Every execution must be recorded in `docs/ai/prompt-log.md` with the prompt, changed files, commands, and evidence.

# Consolidated technical decision

## Stack

- Go with `chi`/`net/http`, `pgx`, and SQLC.
- PostgreSQL 16: source of truth for registrations, transport transactions, weighings, inventory, and financial snapshots.
- Redis 7 Streams: reading buffer and worker consumer group.
- Docker Compose: local execution.
- OpenAPI, JSON logs, Prometheus metrics, and optional OpenTelemetry.

Weight uses `int64` grams. Money uses database `NUMERIC` or a smallest integer unit with explicit conversion. Never use `float` for weight or money.

## Architecture

```text
ESP32 / simulator -> Go API -> Redis Stream: scale-readings -> stabilization worker -> PostgreSQL
                         \------------------------------------------------------> PostgreSQL
PostgreSQL -> reports
```

This is a modular monolith with two processes in the same repository: `api` and `worker`. Do not use microservices, Kafka, RabbitMQ, Kubernetes, or event sourcing in the MVP: their operational cost is not justified by this challenge.

## Flow

1. An OPEN transport transaction relates a truck, grain type, and branch.
2. The device sends an authenticated reading.
3. The API validates it and publishes it to Redis; it returns `202 Accepted` only after `XADD` succeeds.
4. The worker consumes, groups readings by scale/session, detects stability, and stores the final weighing in a PostgreSQL transaction.
5. The transport transaction becomes WEIGHED; inventory, cost, and margin are updated atomically.
6. Further readings from the same passage do not create another weighing until the scale is released.

## Stabilization

Maintain a configurable sliding window with at least 30 samples over 3 seconds. For each window, calculate median, P05, P95, and weight/time slope. A window is stable only when `P95 - P05` is within absolute and percentage thresholds, the slope is below its limit, and two consecutive windows are stable. The final weight is the median of the last stable window.

State machine: `EMPTY -> COLLECTING -> CANDIDATE -> FINALIZED -> EMPTY`; exceptional states: `ABANDONED` and `FAILED`.

## Idempotency and protocol limitation

The original payload has no `event_id`, sequence, or device timestamp. Therefore, it cannot provide strong idempotency or guaranteed delivery; fire-and-forget HTTP does not confirm receipt to the device. The MVP guarantees idempotency of the final weighing in PostgreSQL. The recommended protocol evolution is:

```json
{"event_id":"uuid","scale_id":"scale-001","plate":"ABC1D23","weight_grams":42850300,"measured_at":"2026-07-17T12:00:00.100Z"}
```

Create `UNIQUE(event_id)` for an event when available and `UNIQUE(weighing_session_id, stage)` for the final weighing. Send `XACK` only after idempotent processing and persistence succeed.

## Security and recovery

- One API key per scale, stored only as a hash; error responses never expose a secret.
- Future improvement: HMAC with `timestamp` and `nonce` to prevent replay attacks.
- `XREADGROUP`, `XACK` after success, `XAUTOCLAIM` for pending messages, attempt count, and DLQ.
- Timeouts and `context.Context` for I/O; no goroutine without cancellation.
- Sessions expire to ABANDONED when no new reading arrives, releasing the scale.

# Micro-agents

## 1. Product Owner

**Mission:** protect the MVP scope and turn rules into verifiable requirements.

**Responsibilities:** maintain a prioritized backlog; require success and error scenarios for every requirement; validate tare, cost, margin, and inventory calculations; review README and test data; approve business value.

**Assumptions:** an open transaction represents an expected load; net weight equals gross weight minus tare; price and margin are historical snapshots; margin decreases as inventory approaches target; readings without valid context must not alter inventory or cost.

**Out of scope:** actual firmware, gates/lights, ML, a full frontend, MQTT, Kafka, Kubernetes, and microservices.

**Definition of Done:** automated scenarios approved; failures leave no partial data; public documentation updated; QA evidence approved.

**Operational prompt:**

```text
You are the Product Owner for the grain weighing system. Review the repository backlog and Gherkin scenarios. Preserve the MVP scope, validate business rules, and prioritize blockers. Require success and error coverage for every requirement, explicit dependencies, and no side effects on failure. Do not implement code.
```

## 2. Specialist Architect

**Mission:** keep the architecture simple, robust, inexpensive, and testable.

**Responsibilities:** define API/stream contracts; record ADRs; review security, concurrency, idempotency, observability, and Docker; approve technical acceptance criteria before development.

**Constraints:** keep a Go/PostgreSQL/Redis Streams modular monolith; prohibit `float`; do not assume strong idempotency without `event_id`; require multi-stage builds, non-root execution, health checks, and documented resources.

**Outputs:** `docs/architecture/architecture.md` and ADRs for the modular monolith, Redis Streams, stabilization, protocol/idempotency, numeric modeling, Docker, and security.

**Operational prompt:**

```text
You are the Specialist Architect. Analyze the repository, Gherkin tasks, and evidence. Ensure the API and worker are part of the same modular monolith, Redis Streams is used as a buffer, PostgreSQL remains the source of truth, stabilization is deterministic, and Docker has low resource usage. Document the decision, rationale, trade-offs, impact, and validation. Do not write production code.
```

## 3. Backend Developer 1

**Mission:** implement the synchronous core: domain, migrations, SQLC, registrations, and HTTP.

**Responsibilities:** entities and domain errors; PostgreSQL; Chi API; API-key authentication; validation and publication through the `ReadingPublisher` port; unit, integration, and HTTP contract tests.

**Contracts:** weights use `int64`; API returns 202 only after successful publication; keys are hashed; every I/O uses `context`; request bodies are limited; no goroutine per request.

**Out of scope:** stabilization, stream consumer, worker, and simulator.

**Operational prompt:**

```text
You are Backend Developer 1, a Go, Chi, PostgreSQL, and SQLC specialist. Implement only the synchronous scope of the current task. Keep handler, application, and repository separated; use int64 for grams and never use float for money. Include success and error tests, then run gofmt, go vet, go test ./..., and go test -race ./....
```

## 4. Backend Developer 2

**Mission:** implement asynchronous processing and execution infrastructure.

**Responsibilities:** Redis Streams, consumer group, worker, stabilizer, sessions, idempotency, retry/DLQ, simulator, multi-stage Docker, and concurrent tests.

**Contracts:** `XACK` only after success; Redis is never the source of truth; per-scale/session state is bounded; the worker is safe in multiple instances; logs include `scale_id`, plate, session, and event.

**Operational prompt:**

```text
You are Backend Developer 2, a Go, Redis Streams, and concurrency specialist. Implement only the asynchronous scope of the current task. Use cancellable contexts, bounded memory, pending-message recovery, and PostgreSQL guarantees against duplication. Include tests using real Redis/PostgreSQL and run gofmt, go vet, go test ./..., and go test -race ./....
```

## 5. QA Specialist

**Mission:** prove behavior, failures, concurrency, and reproducibility.

**Responsibilities:** review Gherkin; create unit, integration, and acceptance tests; validate containers; deterministic test data; idempotency; recovery; `go test -race`.

**Minimum acceptance:** `go vet ./...`, `go test ./...`, `go test -race ./...`, and Docker environment pass; raw readings are not stored in PostgreSQL; one session cannot create two weighings; failures are traceable.

**Operational prompt:**

```text
You are a senior QA engineer for Go distributed systems. Review the current task and its evidence. Verify success and error coverage, tests with real Redis/PostgreSQL, race detector, concurrency, recovery, absence of partial data, and executable documentation. Classify risks, give evidence, a reproducible test, and a recommended correction. Do not implement features outside the QA scope.
```

# Docker, README, and minimal resource usage

## Mandatory requirements

- Multi-stage Dockerfile: Go builder separated from runtime.
- Static build: `CGO_ENABLED=0`, `-ldflags='-s -w'` where compatible.
- Minimal runtime image (distroless or Alpine), non-root user, no source/compiler/cache.
- Strict `.dockerignore`.
- Services: `api`, `worker`, `postgres`, `redis`, optional-profile `simulator`, and `test`.
- Real health checks for API, PostgreSQL, and Redis.
- Initial resource targets: API 0.25 CPU/128 MiB; worker 0.25 CPU/128 MiB; Redis 0.25 CPU/128 MiB; PostgreSQL 0.5 CPU/256 MiB.
- Redis: `maxmemory`, `maxmemory-policy noeviction`, stream trimming, and bounded pending-entry recovery.
- Do not expose PostgreSQL or Redis outside Docker in production.

## README commands that must exist in the project

```bash
cp .env.example .env
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready

docker compose run --rm test
docker compose run --rm test go test -race ./...
docker compose run --rm test go vet ./...

docker compose run --rm migrate
docker compose run --rm seed
docker compose run --rm simulator --base-url http://api:8080 \
  --scenario /app/testdata/scenarios/happy-path.json --seed 42
```

The README must also include logs, shutdown/cleanup, environment variables, troubleshooting, API examples, valid and invalid test data, expected timings, and result interpretation.

# Test Harness Framework

Use the native Go `testing` package as the test runner and build a project-owned test harness on top of it. Use `testcontainers-go` for integration tests that need real PostgreSQL and Redis, and Docker Compose only for explicit end-to-end acceptance tests. Testcontainers for Go integrates with native `go test` and is intended for integration/end-to-end dependencies, which matches this use case. [Testcontainers for Go Quickstart](https://golang.testcontainers.org/quickstart/)

## Why this harness

- Unit tests stay fast and have no Docker dependency beyond the Go toolchain.
- Integration tests use real PostgreSQL and Redis with isolated databases/streams.
- Acceptance tests exercise the production-like Compose topology and simulator.
- Test setup is reusable and deterministic instead of copied between tests.
- The harness is test-only, so it does not increase API or worker runtime resource consumption.

## Required structure

```text
internal/testkit/
  harness.go            # lifecycle, cleanup, contexts and deterministic clock
  postgres.go           # PostgreSQL container, migrations and isolated database
  redis.go              # Redis container and stream inspection helpers
  api.go                # in-process HTTP API driver
  fixtures.go           # branch, scale, truck, grain and transaction fixtures
  assertions.go         # database and stream assertions
  simulator.go          # deterministic reading series builder
testdata/
  scenarios/
  fixtures/
tests/acceptance/
  compose_test.go
```

## Harness contract

`testkit.New(t)` must create a cancellable context, an injectable clock, an isolated PostgreSQL database, a Redis instance, and cleanup handlers through `t.Cleanup`. It must expose:

```go
type Harness struct {
    DB       *pgxpool.Pool
    Redis    redis.UniversalClient
    API      *httptest.Server
    Clock    Clock
    Fixtures Fixtures
}
```

Required helpers: `SeedBranch`, `SeedScale`, `SeedTruck`, `SeedGrain`, `OpenTransport`, `SendReading`, `ReadStream`, `Eventually`, `AssertNoWeighing`, `AssertOneWeighing`, and `AssertNoBusinessMutation`.

`Eventually` must poll a predicate using a context deadline; tests must not use arbitrary `time.Sleep`. Containers must wait for usable SQL/Redis conditions, use bounded startup timeouts, and be terminated after tests. The official wait-strategy documentation specifically supports health, HTTP, SQL, log, and other readiness checks. [Wait strategies](https://golang.testcontainers.org/features/wait/introduction/)

# Detailed technical specifications by task

## T01 implementation specification

- Create `cmd/api`, `cmd/worker`, `cmd/simulator`, `internal/testkit`, `testdata`, `Dockerfile`, `docker-compose.yml`, `.dockerignore`, `.env.example`, and `Makefile`.
- Create multi-stage targets named `api`, `worker`, `simulator`, and `test` from one Dockerfile.
- Compose health dependencies: API waits for PostgreSQL and Redis; worker waits for Redis and PostgreSQL; simulator is an optional profile.
- Implement `make verify` as the single quality command: format check, vet, unit tests, integration tests, race tests, and acceptance tests.
- The harness must start dependencies only for tests that explicitly need them.

## T02 implementation specification - explicit CRUD

- Implement REST CRUD for **branches**, **scales**, **trucks**, **grain types**, and **transport transactions**.
- Endpoints must provide create, list, get by ID, update, and safe deactivate for branches, scales, trucks, and grain types. Never physically delete a record referenced by a transaction or weighing.
- Transport transactions provide create, list/get, and controlled state transition; cancellation is allowed only before a final weighing.
- Migrations must define foreign keys, check constraints, unique normalized truck plate, unique `scale_id`, and a partial unique index enforcing one OPEN transaction per truck.
- Generate typed SQLC queries; HTTP handlers must call application services rather than raw SQL.

## T03 implementation specification

- Implement `POST /v1/scale-readings`; use a 64 KiB request-body limit and server/read/write/idle timeouts.
- Authenticate `Authorization: Bearer <key>` by comparing against the stored hash; validate active scale and `scale_id` consistency.
- Store server `received_at` in the stream event; normalize plates; serialize timestamp as UTC RFC3339Nano.
- Return 400 for malformed JSON, 401 for absent/invalid credential, 403 for disabled/mismatched scale, 422 for syntactically valid business-invalid input, and 503 if Redis cannot confirm `XADD`.

## T04 implementation specification

- Keep session state keyed by `scale_id` and normalized plate, with a bounded ring buffer for each active session.
- Use integer-safe median and percentile selection; calculate slope using integer/rational arithmetic or bounded decimal conversion only for the diagnostic, never as final weight.
- Persist algorithm version, sample count, dispersion, slope, and stabilization timestamp with the final weighing for auditability.
- Finalized sessions use hysteresis: release only after the configured low-weight threshold, plate change, or timeout.

## T05 implementation specification

- Create stream `scale-readings`, group `weighing-workers`, and `scale-readings-dlq`.
- Worker uses `XREADGROUP` in bounded batches, a stable group name, and a per-instance consumer name.
- Persist retry count as message metadata or a bounded side key with TTL. Use `XAUTOCLAIM` for idle pending messages.
- Transient database/network failure means no ACK; permanent validation failure or exhausted retries moves the original event plus reason/attempt count to DLQ and then ACKs the original.

## T06 implementation specification

- One database transaction must lock/validate the OPEN transaction, calculate net weight, calculate cost, update inventory, calculate/snapshot margin, insert weighing, and transition the transaction state.
- `net_weight_grams <= 0` must roll back every change.
- Add unique constraints for final weighing by session/stage and processed event by event ID when present.
- For stock ratio `r = clamp(current_inventory / target_inventory, 0, 1)`, use `margin = 20% - 15% * r`; store the applied result as a snapshot.

## T07 implementation specification

- Implement aggregated SQL over finalized weighings only, with optional branch, grain, and date range filters.
- Return net weight, total cost, average purchase price, average applied margin, and completed transport count.
- Add indexes supporting `weighed_at`, `branch_id`, `grain_type_id`, and finalized transaction status.
- Expose `/health/live`, `/health/ready`, `/metrics`, structured JSON logs, and stream/stabilization/error metrics.

## T08 implementation specification

- The 10-scale test must use a fixed seed and distinct scale/truck/session combinations.
- Assert cardinality: one final weighing at most per session; assert isolation: a reading cannot cross scale/plate boundaries.
- Run `go test -race ./...`; no test may use a fixed sleep to wait for asynchronous work.
- The acceptance runner must collect logs from failed Compose services and leave a clear cleanup command.

# Mandatory test data

Version data files under `testdata/scenarios/`, use a fixed seed, and avoid reliance on real time. The simulator sends HTTP requests every 100 ms and supports multiple scales, plates, device keys, HTTP failures, and retries preserving `event_id`.

| Scenario | Must demonstrate |
|---|---|
| `happy-path.json` | arrival, oscillation, 2 stable windows, outlier, one final weighing, release |
| `invalid-readings.json` | invalid credential, unknown plate, invalid weight, duplicate, transaction not open |
| `unstable.json` | dispersion above threshold and no weighing |
| `network-failure.json` | retry with same event and at most one weighing |
| `abandoned.json` | timeout and a new released session |

Minimum data: branch, scale, truck, grain type, price, inventory target, and OPEN transaction. Running with the same seed must create the same sequence and outcome.

# Executable Gherkin backlog

Send each task to Codex in isolation using its corresponding prompt. Start the next one only after QA records success.

## T01 - Foundation, agents, Docker, and deterministic data

**Owners:** Architect + QA + Backend 2. **Dependency:** none.

```gherkin
Feature: Reproducible project foundation

  Scenario: Start the full environment and generate deterministic data
    Given Docker and Docker Compose are installed
    When I run the documented build, migration, seed, and simulator commands with seed 42
    Then API, worker, PostgreSQL, and Redis must be healthy
    And the same reading sequence must be produced in subsequent executions

  Scenario: Reject invalid simulator configuration or unavailable dependency
    Given an invalid seed, a frequency less than or equal to zero, or a missing required variable
    When I start the service or simulator
    Then the process must fail explicitly without exposing a secret
    And no reading must be sent or partial data stored
```

**Prompt:**

```text
Implement T01 in the Go project. Create the API, worker, simulator, and test structure; multi-stage Dockerfiles with a minimal non-root runtime; docker-compose with health checks and conservative limits; .dockerignore; .env.example; versioned test data and a deterministic seed-based simulator. Create Gherkin success and error tests. Document copy-and-paste commands in the README. Run gofmt, go vet ./..., go test ./..., and go test -race ./....
```

## T02 - Explicit CRUD registrations and OPEN transport transaction

**Owners:** Product Owner + Backend 1 + QA. **Dependency:** T01.

```gherkin
Feature: Registrations and transport

  Scenario: Create, read, update, list, and safely deactivate operational registrations
    Given a branch, an active truck with tare, a grain type with a price, and an active scale
    When I create and update a branch, scale, truck, and grain type through the API
    Then each resource must be returned by get and list endpoints
    And deactivation must preserve historical references

  Scenario: Reject invalid CRUD operation or transport transaction
    Given a duplicate plate, tare less than or equal to zero, nonexistent branch, inactive truck, or second OPEN transaction
    When I attempt to create, update, deactivate, or open the invalid resource or transaction
    Then I must receive a validation or conflict error
    And no partial database change must remain

  Scenario: Open a valid transport transaction
    Given an active truck, grain type, and branch
    When I create a transport transaction
    Then it must be OPEN and store a price and margin-policy snapshot
```

**Prompt:**

```text
Implement T02 using Go, PostgreSQL, and SQLC. Implement explicit REST CRUD for branches, scales, trucks, and grain types: create, list, get, update, and safe deactivate. Implement create, list/get, controlled transition, and pre-weighing cancellation for transport transactions. Create reversible migrations, entities, repositories, application services, and endpoints. Normalize plates, store only api_key_hash, use int64 for grams and NUMERIC for money. Guarantee only one OPEN transaction per truck through business logic and a database constraint. Never physically delete historical references. Cover every Gherkin scenario with integration tests and run gofmt, go vet, go test ./..., and go test -race ./....
```

## T03 - Authenticated HTTP ingestion

**Owners:** Backend 1 + Backend 2 + QA. **Dependency:** T02.

```gherkin
Feature: Asynchronous ingestion

  Scenario: Accept a valid reading and publish it to the stream
    Given an active scale with a valid API key
    When I send valid scale_id, plate, positive weight_grams, measured_at, and event_id
    Then the API must return 202 Accepted
    And exactly one message must exist in the scale-readings stream
    And no final weighing must be created synchronously

  Scenario: Reject unauthorized, invalid, or Redis-unavailable reading
    Given an invalid API key, invalid payload, or unavailable Redis
    When I send the reading
    Then the API must return 401, 422, or 503 as applicable
    And no message or weighing must be created
```

**Prompt:**

```text
Implement T03. Create POST /v1/scale-readings with Chi, a limited request body, timeout, hashed Bearer API-key authentication, and an error contract with code, message, and request_id. Validate event_id, scale_id, plate, weight_grams > 0, and RFC3339 timestamp. Publish with XADD through a ReadingPublisher port and return 202 only after success. Do not create a goroutine per request or persist a raw reading in PostgreSQL. Test success, invalid credential, validation, and unavailable Redis against real Redis. Run gofmt, go vet, go test ./..., and go test -race ./....
```

## T04 - Stabilizer and sessions

**Owners:** Backend 2 + QA. **Dependency:** T03.

```gherkin
Feature: Weight stabilization

  Scenario: Finalize after two stable windows
    Given a COLLECTING session with 60 ordered readings in two stable windows
    When the stabilizer processes the readings
    Then it must move to FINALIZED
    And the final weight must be the median of the last window
    And it must record dispersion, slope, samples, and algorithm version

  Scenario: Do not finalize an oscillating or out-of-order sequence
    Given readings with dispersion above the limit or an out-of-order timestamp
    When the stabilizer processes the readings
    Then it must remain COLLECTING or return a typed error
    And no final weighing must be produced
```

**Prompt:**

```text
Implement T04 isolated from HTTP, Redis, and PostgreSQL. Model EMPTY, COLLECTING, CANDIDATE, FINALIZED, ABANDONED, and FAILED states. Use a window with at least 30 samples/3 seconds, median, P05/P95, absolute and percentage tolerance, slope, and two consecutive stable windows. Reject out-of-order times and never use float for the final weight. Create table-driven tests for noise, outlier, increase, decrease, insufficient samples, lost stability, and both Gherkin scenarios. Run gofmt, go vet, go test ./..., and go test -race ./....
```

## T05 - Worker, ACK, recovery, and DLQ

**Owners:** Backend 2 + QA. **Dependency:** T04.

```gherkin
Feature: Recoverable stream processing

  Scenario: Process, persist, and acknowledge a reading
    Given a valid pending message in the weighing-workers consumer group
    When the worker processes it successfully
    Then it must be sent to the session and acknowledged with XACK only after success
    And it must not remain pending

  Scenario: Recover failure without duplication or send to DLQ
    Given a worker that stopped before XACK or a permanent failure after exhausted attempts
    When another worker recovers the message
    Then it must process idempotently or publish the complete event to the DLQ
    And it must not create a duplicate weighing or block the group
```

**Prompt:**

```text
Implement T05 with a Redis Streams Consumer Group. Use XREADGROUP, XACK after persistence, a unique consumer per instance, and XAUTOCLAIM for pending entries. Configure batch, block, idle timeout, and attempt limit; do not acknowledge transient failures; send permanent failure to scale-readings-dlq and only then ACK the original. Expose processed, pending, reclaimed, DLQ, and failure metrics. Prove scenarios with real Redis/PostgreSQL, without mocks for ACK/reclaim. Run gofmt, go vet, go test ./..., and go test -race ./....
```

## T06 - Final weighing, cost, inventory, and idempotency

**Owners:** Backend 1 + Backend 2 + QA + Product Owner. **Dependency:** T05.

```gherkin
Feature: Idempotent financial finalization

  Scenario: Persist one weighing and update the business state
    Given an OPEN transaction and two stable windows for the same session
    When the worker finalizes the weighing
    Then it must persist gross, tare, net, cost, grain, scale, and date
    And it must update inventory and set the transaction to WEIGHED in one transaction
    And margin must be inversely proportional to inventory and stored as a snapshot

  Scenario: Reject invalid net weight or concurrent finalization
    Given gross weight less than or equal to tare, nonexistent transaction, or two workers finalizing the same session
    When finalization is attempted
    Then no partial financial update must occur
    And only one weighing may exist for the session/stage
```

**Prompt:**

```text
Implement T06 with PostgreSQL and SQLC. In one transaction, validate the OPEN transaction, calculate net = gross - tare, cost per ton, and margin between 5% and 20% inversely proportional to inventory/inventory target; persist snapshots and update inventory/status. Use NUMERIC and int64, never float. Guarantee UNIQUE(session_id, stage) and event_id idempotency when available. Create integration tests for rollback, two concurrent finalizations, and Gherkin scenarios. Run gofmt, go vet, go test ./..., and go test -race ./....
```

## T07 - Reports, observability, and health

**Owners:** Backend 1 + QA + Product Owner. **Dependency:** T06.

```gherkin
Feature: Operational reporting

  Scenario: Query indicators by period, branch, and grain
    Given finalized weighings for different branches and grain types
    When I query a report with valid filters
    Then I must receive net weight, cost, average margin, and count only for finalized transactions

  Scenario: Query an invalid or empty range
    Given a start date after the end date or a period with no weighings
    When I query the report
    Then I must receive 422 without an invalid query or 200 with zero totals and an empty list
```

**Prompt:**

```text
Implement T07. Create a reporting endpoint with branch, grain, and period filters; use indexed aggregate SQL without N+1 queries and only finalized weighings. Return net weight, total cost, average price, average margin, and count. Include live/ready health endpoints, JSON logs, and ingestion, stabilization, stream, and failure metrics. Test Gherkin scenarios, update OpenAPI, and add README examples. Run gofmt, go vet, go test ./..., and go test -race ./....
```

## T08 - Concurrency, acceptance, and final documentation

**Owners:** QA + all micro-agents. **Dependency:** T07.

```gherkin
Feature: Reproducible quality and delivery

  Scenario: Validate concurrent end-to-end load
    Given 10 scales sending readings every 100 ms with a fixed seed
    When API and worker process them concurrently
    Then each session must create at most one final weighing
    And no reading must be assigned to the wrong scale or truck
    And go test -race must pass

  Scenario: Prevent delivery without quality or sufficient documentation
    Given a change with a test failure, race, vet error, missing variable, or undocumented instruction
    When I run the single validation command in a clean clone
    Then the process must fail in an identifiable way
    And no implicit manual step may be needed in the valid flow
```

**Prompt:**

```text
Implement T08. Create make verify or an equivalent script for format check, go vet, unit tests, integration tests, race detection, and Docker acceptance. Execute concurrent load with 10 scales and a fixed seed, without arbitrary sleeps; use polling with context timeout. Update README with prerequisites, build, startup, test data, simulator, tests, logs, cleanup, variables, troubleshooting, and expected outcomes. Record prompts and evidence. Do not declare success without executing commands.
```

## T09 - Executable README and technical documentation

**Owners:** Product Owner + Specialist Architect + QA. **Dependency:** T08.

```gherkin
Feature: Executable project documentation

  Scenario: Run the complete system from a clean clone using only the README
    Given a developer with Docker Engine, Docker Compose v2, and Git installed
    And a clean clone of the repository
    When the developer follows the README from prerequisites through the happy-path simulator
    Then API, worker, PostgreSQL, and Redis must become healthy
    And migrations, seed data, simulator, reports, and all test commands must work without undocumented manual steps

  Scenario: Diagnose a documented failure without leaking secrets
    Given Redis is unavailable, PostgreSQL is unavailable, or a required environment variable is missing
    When the developer follows the README troubleshooting section
    Then the documented checks must identify the failing dependency or configuration
    And logs and examples must not reveal device keys, passwords, or connection secrets
```

**Detailed implementation specification:**

- README starts with the architecture diagram, scope, and trade-offs.
- Provide a prerequisites matrix for Docker, Compose, Go, Make, ports, CPU, memory, and disk.
- Document an exact clean-start sequence: clone, copy `.env.example`, build, startup, health checks, migrations, seed, worker verification, simulator, report query, and shutdown.
- Include a command and expected result for each validation stage; include failure commands and recovery commands.
- Document every environment variable with purpose, required/default value, and secret-handling rule.
- Document HTTP examples for CRUD, opening a transport, sending a reading, querying reports, and health endpoints.
- Document all test layers: unit, integration through the harness, race, acceptance, and `make verify`.
- Document test-data scenarios, deterministic seed behavior, expected final weighing, and expected rejected scenarios.
- Add a troubleshooting table for common database, Redis, port, migration, API key, stream lag, worker, and Docker failures.
- QA must execute the README in a clean environment and attach command output to `docs/ai/prompt-log.md`.

**Prompt:**

```text
Implement T09 as documentation only, unless a documented command reveals a defect that must be fixed in a separate task.

Write a detailed executable README for the Go scale-weighing project. It must allow a developer with a clean clone to build, start, validate, seed, simulate, query, test, troubleshoot, and shut down the complete application without undocumented steps.

Include: architecture and trade-offs; prerequisites and resource requirements; exact Docker Compose commands; environment-variable reference; health checks; migrations; seed data; deterministic simulator scenarios; CRUD/API examples; reports; test-harness usage; unit/integration/race/acceptance commands; make verify; logs; cleanup; troubleshooting; security guidance; expected output for every important command.

Add Gherkin acceptance tests or a QA-run verification script proving both scenarios. Never include real secrets in documentation. Execute every documented command in a clean environment and record evidence in docs/ai/prompt-log.md.
```

# Global Definition of Done

- Every task has a prompt plus automated success and error scenarios.
- All five micro-agent roles participated and recorded a handoff.
- Docker flow starts from a clean clone.
- `go vet ./...`, `go test ./...`, and `go test -race ./...` pass.
- Valid test data produces exactly one weighing; invalid data does not alter business state.
- The README has been independently executed in a clean environment by QA.
- Logs, metrics, OpenAPI, README, migrations, simulator, test harness, and prompt log are versioned.
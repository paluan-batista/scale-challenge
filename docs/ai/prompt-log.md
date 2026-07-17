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

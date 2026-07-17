# Architecture baseline (T01)

## Decision

The project is a Go modular monolith with separate `api`, `worker`, and
`simulator` executables. PostgreSQL will be the system of record and Redis
Streams will be a bounded, short-lived buffer. T01 creates only the process,
container, deterministic-test, and Compose baseline; it creates no domain
model, database schema, HTTP business endpoint, stream consumer, or simulator
behavior.

## Rationale and trade-offs

Separate executables allow independent scaling and failure isolation without the
operational cost of separate deployable services. PostgreSQL gives transactional
business state; Redis is therefore not a source of truth. Integer grams and
minor-unit/`NUMERIC` money avoid floating-point loss. The cost is that stream
recovery and final-weighing idempotency must be designed explicitly in T04-T06.

## T01 contracts and operational impact

- Docker targets are `api`, `worker`, `simulator`, and `test`; runtime targets
  are static, multi-stage Alpine images that run as UID 10001.
- Compose starts API and worker only after healthy PostgreSQL and Redis. It does
  not publish database or Redis ports. The simulator is opt-in (`simulator`
  profile).
- API bootstrap probes exist only to make the T01 Compose health check real.
  They do not assert datastore connectivity; dependency-aware readiness belongs
  to T07.
- `internal/testkit.New` supplies cancellation, fixtures, an in-process
  `*httptest.Server` API driver, and a fixed UTC clock without starting dependencies. Explicit
  `WithPostgres`/`WithRedis` options start isolated real Testcontainers and
  clean them up automatically. The harness honors `DOCKER_HOST` and otherwise
  discovers the active Docker CLI context for local Colima compatibility.
- The `make verify` gate runs formatting, vet, unit, integration-tag, race, and
  acceptance-tag suites. It does not run Compose automatically.

## T02 registration contract

T02 keeps registration CRUD and transport state in the same API process and
PostgreSQL database. SQLC-generated queries are the repository boundary;
handlers call application services, never SQL directly. Registrations are
deactivated rather than deleted, preserving transaction references. Truck plates
and scale IDs are normalized before persistence, and PostgreSQL enforces their
uniqueness plus one `OPEN` transaction per truck through a partial unique index.
Weight remains `int64` grams and price is an explicit `BIGINT` minor unit.
Scale secrets are accepted only to create a bcrypt `api_key_hash`; API responses
never return the source key or hash. Redis remains unused by this task.

## Verification criteria

`make verify` must pass without Docker dependencies. `docker compose up --build`
must build the three runtime targets and report healthy PostgreSQL, Redis, and
API once a Compose v2 client is available. Future work must preserve non-root
runtime execution, contexts for I/O, bounded stream/session state, and the rule
that raw readings are never persisted in PostgreSQL.

# Scale challenge — T02 registrations and transport

T02 adds PostgreSQL registrations and transport transactions to the T01
foundation. Redis worker processing, reading ingestion, stabilization, and final
weighing remain out of scope.

## Prerequisites

- Go 1.26+
- Docker Engine and Docker Compose (`docker compose` v2 or the standalone
  `docker-compose` v5.3.1 executable)
- Make
- Port 8080 free; allow approximately 1 GiB of Docker memory for PostgreSQL,
  Redis, and test containers.

## Run the T02 environment

```bash
cp .env.example .env
docker build --target api -t scale-api:t01 .
docker build --target worker -t scale-worker:t01 .
docker build --target simulator -t scale-simulator:t01 .
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
docker compose run --rm migrate
docker compose run --rm seed
docker compose --profile simulator run --rm simulator
docker compose down --volumes --remove-orphans
```

If your installation provides the standalone command, substitute
`docker-compose` for `docker compose` in every command above.

The API rejects a missing `DATABASE_URL`; the worker remains a T01 bootstrap and
requires both `DATABASE_URL` and `REDIS_ADDR`. The simulator rejects a missing scenario/base URL, a negative seed,
and an overridden frequency less than or equal to zero before it produces an
event. It never logs device keys.

`testdata/scenarios/` is versioned and uses seed 42. The simulator emits a JSON
line for every deterministic event; compare repeated runs with the same scenario
and seed. HTTP delivery starts with the T03 ingestion contract.

## Quality checks

```bash
gofmt -w $(find . -type f -name '*.go' -not -path './vendor/*')
go vet ./...
go test ./...
go test -race ./...
go test -tags=integration ./internal/testkit/...
make verify
```

The integration-tagged harness uses real isolated PostgreSQL and Redis
Testcontainers, waits for usable connections, and cleans them up through
`t.Cleanup`. Unit tests do not start Docker dependencies.

Registrations are exposed under `/v1/branches`, `/v1/scales`, `/v1/trucks`, and
`/v1/grain-types`; transport transactions use `/v1/transport-transactions`.
All registrations are safely deactivated rather than deleted. Prices are stored
in explicit minor units and weights in integer grams.

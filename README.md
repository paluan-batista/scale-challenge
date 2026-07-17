# Scale challenge — T01 foundation

T01 establishes the Go processes, Docker topology, deterministic simulator data,
and opt-in Testcontainers harness. It does not contain registrations, database
migrations, seed records, reading ingestion, or a weighing worker; those are
owned by later tasks.

## Prerequisites

- Go 1.26+
- Docker Engine and Docker Compose v2
- Make
- Port 8080 free; allow approximately 1 GiB of Docker memory for PostgreSQL,
  Redis, and test containers.

## Run the T01 environment

```bash
cp .env.example .env
docker build --target api -t scale-api:t01 .
docker build --target worker -t scale-worker:t01 .
docker build --target simulator -t scale-simulator:t01 .
docker compose up --build -d
docker compose ps
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
docker compose --profile simulator run --rm simulator
docker compose down --volumes --remove-orphans
```

The API and worker reject missing `DATABASE_URL` or `REDIS_ADDR` before opening
any client. The simulator rejects a missing scenario/base URL, a negative seed,
and an overridden frequency less than or equal to zero before it produces an
event. It never logs device keys.

`testdata/scenarios/` is versioned and uses seed 42. The simulator currently
validates and generates deterministic events only; HTTP delivery starts with the
T03 ingestion contract. Compare repeated dry runs by running the simulator with
the same scenario and seed.

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

T01's Gherkin scenario mentions migration and seed commands, but the required
schema and registrations are T02 work. They are deliberately not represented by
misleading no-op commands in this foundation.

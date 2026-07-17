# Scale Challenge

## Prompts used in this project

The implementation contract and task prompts are in
[`docs/context/scale-challenge-codex-playbook.md`](docs/context/scale-challenge-codex-playbook.md).
The exact prompts, decisions, and validation evidence used for each completed
task are recorded in [`docs/ai/prompt-log.md`](docs/ai/prompt-log.md). Role
instructions are in [`docs/micro-agents/`](docs/micro-agents/).

## Launch with Docker

Prerequisites: Docker Engine, Docker Compose v2, and free local ports `8080`,
`3000`, and `9090`.

```bash
cp .env.example .env
docker-compose up --build -d
docker-compose ps
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
```

The first command creates only local development credentials. Do not commit the
generated `.env` file. Compose starts PostgreSQL, Redis, the migration job, API,
worker, Prometheus, and Grafana. To load the deterministic sample entities:

```bash
docker-compose --profile tools run --rm seed
```

To inspect or stop the environment:

```bash
docker-compose logs -f api worker
docker-compose down --volumes --remove-orphans
```

If your installation uses the standalone Compose client, replace `docker
compose` with `docker-compose` in each command.

## Run tests

Run the complete quality gate from the repository root:

```bash
make verify
```

It runs format checking, `go vet`, unit tests, real PostgreSQL/Redis integration
tests, the race detector, Docker acceptance, and repeats the checks in a
temporary clean Git clone. Individual targets are available when diagnosing a
failure:

```bash
make format-check
make vet
make unit
make integration
make race
make acceptance
```

## APIs: Postman and cURL

Import [`postman/Scale Challenge.postman_collection.json`](postman/Scale%20Challenge.postman_collection.json)
into Postman. Set `baseUrl` if the API is not on `http://localhost:8080`.
Run the **Create** requests for branch, scale, truck, and grain type first; the
collection saves the returned IDs and uses them in later requests. The scale API
key is the `scaleApiKey` collection variable. The collection includes every
public endpoint and requests are organized in a safe dependency order.

The following cURL commands cover the same endpoint surface. They use `jq` to
save identifiers returned by the create operations; alternatively substitute
literal IDs. Start the Docker environment first.

```bash
export BASE_URL=http://localhost:8080
export SCALE_KEY=postman-scale-key

# Operational endpoints
curl --fail "$BASE_URL/health/live"
curl --fail "$BASE_URL/health/ready"
curl --fail "$BASE_URL/metrics"

# Branches
BRANCH_ID=$(curl --fail -sS -X POST "$BASE_URL/v1/branches" -H 'Content-Type: application/json' -d '{"code":"CURLBR","name":"cURL branch"}' | jq -r .id)
curl --fail "$BASE_URL/v1/branches"
curl --fail "$BASE_URL/v1/branches/$BRANCH_ID"
curl --fail -X PUT "$BASE_URL/v1/branches/$BRANCH_ID" -H 'Content-Type: application/json' -d '{"code":"CURLBR","name":"cURL branch updated"}'

# Scales
SCALE_ID=$(curl --fail -sS -X POST "$BASE_URL/v1/scales" -H 'Content-Type: application/json' -d "{\"branch_id\":\"$BRANCH_ID\",\"scale_id\":\"curl-scale\",\"name\":\"cURL scale\",\"api_key\":\"$SCALE_KEY\"}" | jq -r .id)
curl --fail "$BASE_URL/v1/scales"
curl --fail "$BASE_URL/v1/scales/$SCALE_ID"
curl --fail -X PUT "$BASE_URL/v1/scales/$SCALE_ID" -H 'Content-Type: application/json' -d "{\"branch_id\":\"$BRANCH_ID\",\"scale_id\":\"curl-scale\",\"name\":\"cURL scale updated\",\"api_key\":\"$SCALE_KEY\"}"

# Trucks
TRUCK_ID=$(curl --fail -sS -X POST "$BASE_URL/v1/trucks" -H 'Content-Type: application/json' -d '{"plate":"CRL0001","tare_weight_grams":12000}' | jq -r .id)
curl --fail "$BASE_URL/v1/trucks"
curl --fail "$BASE_URL/v1/trucks/$TRUCK_ID"
curl --fail -X PUT "$BASE_URL/v1/trucks/$TRUCK_ID" -H 'Content-Type: application/json' -d '{"plate":"CRL0001","tare_weight_grams":12500}'

# Grain types
GRAIN_TYPE_ID=$(curl --fail -sS -X POST "$BASE_URL/v1/grain-types" -H 'Content-Type: application/json' -d '{"code":"CRLSOY","name":"cURL soy","purchase_price_minor":125000,"inventory_target_grams":100000000,"margin_policy_bps":2000}' | jq -r .id)
curl --fail "$BASE_URL/v1/grain-types"
curl --fail "$BASE_URL/v1/grain-types/$GRAIN_TYPE_ID"
curl --fail -X PUT "$BASE_URL/v1/grain-types/$GRAIN_TYPE_ID" -H 'Content-Type: application/json' -d '{"code":"CRLSOY","name":"cURL soy updated","purchase_price_minor":126000,"inventory_target_grams":100000000,"margin_policy_bps":2000}'

# Transport transactions
TRANSPORT_ID=$(curl --fail -sS -X POST "$BASE_URL/v1/transport-transactions" -H 'Content-Type: application/json' -d "{\"branch_id\":\"$BRANCH_ID\",\"truck_id\":\"$TRUCK_ID\",\"grain_type_id\":\"$GRAIN_TYPE_ID\"}" | jq -r .id)
curl --fail "$BASE_URL/v1/transport-transactions"
curl --fail "$BASE_URL/v1/transport-transactions/$TRANSPORT_ID"

# Authenticated Redis Streams ingestion and finalized-only report
curl --fail -X POST "$BASE_URL/v1/scale-readings" -H 'Content-Type: application/json' -H "Authorization: Bearer $SCALE_KEY" -d '{"event_id":"4a24a562-a243-4cc0-a04c-4234a438c793","scale_id":"curl-scale","plate":"CRL0001","weight_grams":2012000,"measured_at":"2026-07-17T12:00:00Z"}'
curl --fail "$BASE_URL/v1/reports/weighings?branch_id=$BRANCH_ID&grain_type_id=$GRAIN_TYPE_ID&start=2026-01-01T00:00:00Z&end=2026-12-31T23:59:59Z"

# State/deactivation endpoints (run after the preceding requests)
curl --fail -X PATCH "$BASE_URL/v1/transport-transactions/$TRANSPORT_ID/status" -H 'Content-Type: application/json' -d '{"status":"CANCELLED"}'
curl --fail -X POST "$BASE_URL/v1/scales/$SCALE_ID/deactivate"
curl --fail -X POST "$BASE_URL/v1/trucks/$TRUCK_ID/deactivate"
curl --fail -X POST "$BASE_URL/v1/grain-types/$GRAIN_TYPE_ID/deactivate"
curl --fail -X POST "$BASE_URL/v1/branches/$BRANCH_ID/deactivate"
```

The post above is one reading, not a final weighing: the worker finalizes only
after its stable-window criteria are satisfied. Use the Postman collection to
repeat readings with unique event IDs, or run the deterministic simulator:

```bash
docker compose --profile simulator run --rm simulator
```

## View metrics in Grafana

After `docker compose up`, open [Grafana](http://localhost:3000). Sign in with
`GRAFANA_ADMIN_USER` and `GRAFANA_ADMIN_PASSWORD` from `.env` (the development
defaults are `admin` and `development-grafana-only`). Open **Dashboards → Scale
operations → Scale operations**.

The dashboard is provisioned automatically and shows ingestion/processing rates,
Redis stream lag, pending and dead-letter messages, and finalized weighings.
Prometheus is available at [http://localhost:9090](http://localhost:9090) for
ad-hoc metric queries; the API’s raw Prometheus exposition is
[http://localhost:8080/metrics](http://localhost:8080/metrics).

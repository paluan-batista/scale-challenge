# ADR 002: Redis Streams buffer

**Decision:** use Redis Streams as the reading buffer; PostgreSQL remains the
business source of truth.

**Rationale:** consumer groups and pending recovery fit short-lived readings.
**Trade-off:** Redis is an additional runtime dependency, never durable business
state. **Impact:** later workers ACK only after PostgreSQL persistence.
**Validation:** integration tests will inspect pending, acknowledged, reclaimed,
and DLQ messages against real Redis.

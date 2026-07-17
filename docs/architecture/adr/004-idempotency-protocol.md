# ADR 004: Protocol and idempotency

**Decision:** evolve device events to include `event_id` and `measured_at`; until
then only final-weighing idempotency is guaranteed.

**Rationale:** payloads without a unique event identifier cannot prove exactly-once
delivery. **Trade-off:** old devices retain a known duplication limit. **Impact:**
later persistence requires unique event and session/stage protections.
**Validation:** retry/restart tests must prove at most one final weighing.

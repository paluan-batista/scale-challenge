# ADR 001: Modular monolith

**Decision:** keep API and worker as executables in one Go repository.

**Rationale:** this gives isolated process lifecycles without distributed-service
deployment cost. **Trade-off:** module boundaries must remain explicit. **Impact:**
all shared contracts stay versioned in-repository. **Validation:** build `api` and
`worker` targets independently and exercise them together in Compose.

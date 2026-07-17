# ADR 006: Container and resource policy

**Decision:** use one multi-stage Dockerfile, static Go binaries, Alpine runtime,
non-root application users, health checks, and Compose resource limits.

**Rationale:** this minimizes image/runtime overhead. **Trade-off:** Alpine is
slightly larger than distroless but permits a simple user setup. **Impact:** API
and worker target 0.25 CPU/128 MiB, Redis 0.25 CPU/128 MiB, and PostgreSQL 0.5
CPU/256 MiB. **Validation:** Compose build and health checks.

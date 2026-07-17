# ADR 003: Deterministic stabilization

**Decision:** T04 will use bounded per-session windows, integer/rational
calculations, and two consecutive stable windows.

**Rationale:** stable results must be reproducible and auditable. **Trade-off:**
the algorithm needs explicit limits and test fixtures. **Impact:** no float is
allowed for final weight. **Validation:** fixed-clock, fixed-seed unit and
concurrency scenarios will assert the selected median and single finalization.

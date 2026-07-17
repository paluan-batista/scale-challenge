# ADR 005: Numeric modeling

**Decision:** weight is `int64` grams; money is PostgreSQL `NUMERIC` or a named
smallest integer unit.

**Rationale:** both values need exact arithmetic. **Trade-off:** conversion rules
must be explicit at boundaries. **Impact:** float is prohibited for weight and
money. **Validation:** domain and persistence tests will use boundary values.

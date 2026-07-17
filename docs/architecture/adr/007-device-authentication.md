# ADR 007: Device authentication and replay protection

**Decision:** T03 will authenticate one hashed API key per scale; a later protocol
revision should add HMAC, timestamp, and nonce replay protection.

**Rationale:** API keys are simple for the MVP but do not prevent replay alone.
**Trade-off:** replay resistance requires device protocol support. **Impact:**
secrets must never appear in logs or error responses. **Validation:** HTTP tests
will cover absent, invalid, disabled, and mismatched credentials.

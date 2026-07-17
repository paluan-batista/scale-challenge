-- T05 stores only the idempotency ledger for a consumed stream event. Raw
-- readings remain in Redis Streams; final weighings and financial state are
-- intentionally deferred to T06.
CREATE TABLE processed_scale_events (
    event_id TEXT PRIMARY KEY,
    stream_message_id TEXT NOT NULL UNIQUE,
    scale_id TEXT NOT NULL,
    plate TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT processed_scale_events_event_id_not_blank CHECK (btrim(event_id) <> ''),
    CONSTRAINT processed_scale_events_stream_message_id_not_blank CHECK (btrim(stream_message_id) <> ''),
    CONSTRAINT processed_scale_events_scale_id_not_blank CHECK (btrim(scale_id) <> ''),
    CONSTRAINT processed_scale_events_plate_not_blank CHECK (btrim(plate) <> '')
);

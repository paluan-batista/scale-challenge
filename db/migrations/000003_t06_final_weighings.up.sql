CREATE TABLE branch_grain_inventory (
    branch_id TEXT NOT NULL REFERENCES branches(id),
    grain_type_id TEXT NOT NULL REFERENCES grain_types(id),
    current_inventory_grams BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (branch_id, grain_type_id),
    CONSTRAINT branch_grain_inventory_non_negative CHECK (current_inventory_grams >= 0)
);

CREATE TABLE weighings (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    stage TEXT NOT NULL,
    event_id TEXT,
    transport_transaction_id TEXT NOT NULL REFERENCES transport_transactions(id),
    branch_id TEXT NOT NULL REFERENCES branches(id),
    grain_type_id TEXT NOT NULL REFERENCES grain_types(id),
    scale_id TEXT NOT NULL REFERENCES scales(id),
    gross_weight_grams BIGINT NOT NULL,
    tare_weight_grams BIGINT NOT NULL,
    net_weight_grams BIGINT NOT NULL,
    load_cost_minor BIGINT NOT NULL,
    purchase_price_minor_snapshot BIGINT NOT NULL,
    applied_margin_bps INTEGER NOT NULL,
    algorithm_version TEXT NOT NULL,
    sample_count INTEGER NOT NULL,
    dispersion_grams BIGINT NOT NULL,
    slope TEXT NOT NULL,
    weighed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT weighings_session_not_blank CHECK (btrim(session_id) <> ''),
    CONSTRAINT weighings_stage_not_blank CHECK (btrim(stage) <> ''),
    CONSTRAINT weighings_weights_valid CHECK (gross_weight_grams > 0 AND tare_weight_grams >= 0 AND net_weight_grams > 0),
    CONSTRAINT weighings_net_weight_matches CHECK (net_weight_grams = gross_weight_grams - tare_weight_grams),
    CONSTRAINT weighings_cost_non_negative CHECK (load_cost_minor >= 0),
    CONSTRAINT weighings_purchase_price_non_negative CHECK (purchase_price_minor_snapshot >= 0),
    CONSTRAINT weighings_margin_range CHECK (applied_margin_bps BETWEEN 500 AND 2000),
    CONSTRAINT weighings_sample_count_positive CHECK (sample_count > 0),
    CONSTRAINT weighings_dispersion_non_negative CHECK (dispersion_grams >= 0)
);

CREATE UNIQUE INDEX weighings_session_stage_unique ON weighings (session_id, stage);
CREATE UNIQUE INDEX weighings_event_id_unique ON weighings (event_id) WHERE event_id IS NOT NULL;
CREATE INDEX weighings_finalized_lookup_idx ON weighings (branch_id, grain_type_id, weighed_at);

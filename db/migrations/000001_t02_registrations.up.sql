CREATE TABLE branches (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT branches_code_not_blank CHECK (btrim(code) <> ''),
    CONSTRAINT branches_name_not_blank CHECK (btrim(name) <> '')
);

CREATE TABLE scales (
    id TEXT PRIMARY KEY,
    branch_id TEXT NOT NULL REFERENCES branches(id),
    scale_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT scales_scale_id_not_blank CHECK (btrim(scale_id) <> ''),
    CONSTRAINT scales_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT scales_api_key_hash_not_blank CHECK (btrim(api_key_hash) <> '')
);

CREATE TABLE trucks (
    id TEXT PRIMARY KEY,
    plate TEXT NOT NULL UNIQUE,
    tare_weight_grams BIGINT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT trucks_plate_not_blank CHECK (btrim(plate) <> ''),
    CONSTRAINT trucks_tare_positive CHECK (tare_weight_grams > 0)
);

CREATE TABLE grain_types (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    purchase_price_minor BIGINT NOT NULL,
    inventory_target_grams BIGINT NOT NULL,
    margin_policy_bps INTEGER NOT NULL DEFAULT 2000,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT grain_types_code_not_blank CHECK (btrim(code) <> ''),
    CONSTRAINT grain_types_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT grain_types_price_non_negative CHECK (purchase_price_minor >= 0),
    CONSTRAINT grain_types_inventory_target_positive CHECK (inventory_target_grams > 0),
    CONSTRAINT grain_types_margin_policy_valid CHECK (margin_policy_bps BETWEEN 0 AND 10000)
);

CREATE TABLE transport_transactions (
    id TEXT PRIMARY KEY,
    branch_id TEXT NOT NULL REFERENCES branches(id),
    truck_id TEXT NOT NULL REFERENCES trucks(id),
    grain_type_id TEXT NOT NULL REFERENCES grain_types(id),
    status TEXT NOT NULL,
    purchase_price_minor_snapshot BIGINT NOT NULL,
    margin_policy_bps_snapshot INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT transport_transactions_status_valid CHECK (status IN ('OPEN', 'CANCELLED', 'WEIGHED')),
    CONSTRAINT transport_transactions_price_non_negative CHECK (purchase_price_minor_snapshot >= 0),
    CONSTRAINT transport_transactions_margin_valid CHECK (margin_policy_bps_snapshot BETWEEN 0 AND 10000)
);

CREATE UNIQUE INDEX transport_transactions_one_open_per_truck
    ON transport_transactions (truck_id)
    WHERE status = 'OPEN';

CREATE INDEX transport_transactions_branch_id_idx ON transport_transactions (branch_id);
CREATE INDEX transport_transactions_grain_type_id_idx ON transport_transactions (grain_type_id);

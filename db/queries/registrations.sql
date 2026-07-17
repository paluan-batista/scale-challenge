-- name: CreateBranch :one
INSERT INTO branches (id, code, name)
VALUES (sqlc.arg(id), sqlc.arg(code), sqlc.arg(name))
RETURNING *;

-- name: GetBranch :one
SELECT * FROM branches WHERE id = sqlc.arg(id);

-- name: GetActiveBranch :one
SELECT * FROM branches WHERE id = sqlc.arg(id) AND active = TRUE;

-- name: ListBranches :many
SELECT * FROM branches ORDER BY code;

-- name: UpdateBranch :one
UPDATE branches
SET code = sqlc.arg(code), name = sqlc.arg(name), updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: DeactivateBranch :one
UPDATE branches SET active = FALSE, updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: CreateScale :one
INSERT INTO scales (id, branch_id, scale_id, name, api_key_hash)
VALUES (sqlc.arg(id), sqlc.arg(branch_id), sqlc.arg(scale_id), sqlc.arg(name), sqlc.arg(api_key_hash))
RETURNING *;

-- name: GetScale :one
SELECT * FROM scales WHERE id = sqlc.arg(id);

-- name: GetActiveScale :one
SELECT * FROM scales WHERE id = sqlc.arg(id) AND active = TRUE;

-- name: ListScales :many
SELECT * FROM scales ORDER BY scale_id;

-- name: UpdateScale :one
UPDATE scales
SET branch_id = sqlc.arg(branch_id), scale_id = sqlc.arg(scale_id), name = sqlc.arg(name),
    api_key_hash = sqlc.arg(api_key_hash), updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: DeactivateScale :one
UPDATE scales SET active = FALSE, updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: CreateTruck :one
INSERT INTO trucks (id, plate, tare_weight_grams)
VALUES (sqlc.arg(id), sqlc.arg(plate), sqlc.arg(tare_weight_grams))
RETURNING *;

-- name: GetTruck :one
SELECT * FROM trucks WHERE id = sqlc.arg(id);

-- name: GetActiveTruck :one
SELECT * FROM trucks WHERE id = sqlc.arg(id) AND active = TRUE;

-- name: ListTrucks :many
SELECT * FROM trucks ORDER BY plate;

-- name: UpdateTruck :one
UPDATE trucks
SET plate = sqlc.arg(plate), tare_weight_grams = sqlc.arg(tare_weight_grams), updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: DeactivateTruck :one
UPDATE trucks SET active = FALSE, updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: CreateGrainType :one
INSERT INTO grain_types (id, code, name, purchase_price_minor, inventory_target_grams, margin_policy_bps)
VALUES (sqlc.arg(id), sqlc.arg(code), sqlc.arg(name), sqlc.arg(purchase_price_minor),
        sqlc.arg(inventory_target_grams), sqlc.arg(margin_policy_bps))
RETURNING *;

-- name: GetGrainType :one
SELECT * FROM grain_types WHERE id = sqlc.arg(id);

-- name: GetActiveGrainType :one
SELECT * FROM grain_types WHERE id = sqlc.arg(id) AND active = TRUE;

-- name: ListGrainTypes :many
SELECT * FROM grain_types ORDER BY code;

-- name: UpdateGrainType :one
UPDATE grain_types
SET code = sqlc.arg(code), name = sqlc.arg(name), purchase_price_minor = sqlc.arg(purchase_price_minor),
    inventory_target_grams = sqlc.arg(inventory_target_grams), margin_policy_bps = sqlc.arg(margin_policy_bps),
    updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: DeactivateGrainType :one
UPDATE grain_types SET active = FALSE, updated_at = now()
WHERE id = sqlc.arg(id) AND active = TRUE
RETURNING *;

-- name: CreateTransportTransaction :one
INSERT INTO transport_transactions (
    id, branch_id, truck_id, grain_type_id, status, purchase_price_minor_snapshot, margin_policy_bps_snapshot
)
VALUES (
    sqlc.arg(id), sqlc.arg(branch_id), sqlc.arg(truck_id), sqlc.arg(grain_type_id), 'OPEN',
    sqlc.arg(purchase_price_minor_snapshot), sqlc.arg(margin_policy_bps_snapshot)
)
RETURNING *;

-- name: GetTransportTransaction :one
SELECT * FROM transport_transactions WHERE id = sqlc.arg(id);

-- name: ListTransportTransactions :many
SELECT * FROM transport_transactions ORDER BY created_at DESC, id;

-- name: CancelTransportTransaction :one
UPDATE transport_transactions
SET status = 'CANCELLED', updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'OPEN'
RETURNING *;

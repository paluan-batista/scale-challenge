// Package finalization owns the single PostgreSQL transaction that converts a
// stable gross weight into an immutable final weighing and financial snapshot.
package finalization

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"scale-challenge/internal/domain"
)

const gramsPerMetricTon int64 = 1_000_000_000

type Input struct {
	SessionID              string
	Stage                  string
	EventID                string
	TransportTransactionID string
	ScaleID                string // external, stable scales.scale_id
	Plate                  string // used only when no transaction ID is supplied
	GrossWeightGrams       int64
	AlgorithmVersion       string
	SampleCount            int
	DispersionGrams        int64
	Slope                  string
	WeighedAt              time.Time
}

type Weighing struct {
	ID                     string
	SessionID              string
	Stage                  string
	EventID                string
	TransportTransactionID string
	BranchID               string
	GrainTypeID            string
	ScaleID                string
	GrossWeightGrams       int64
	TareWeightGrams        int64
	NetWeightGrams         int64
	LoadCostMinor          int64
	PurchasePriceMinor     int64
	AppliedMarginBPS       int32
	WeighedAt              time.Time
}

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Finalize is idempotent by supplied event ID and by (session, stage). Its
// transaction locks the OPEN transport before any inventory mutation.
func (s *Service) Finalize(ctx context.Context, input Input) (Weighing, error) {
	if err := validate(input); err != nil {
		return Weighing{}, err
	}
	transaction, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Weighing{}, fmt.Errorf("begin finalization: %w", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if input.EventID != "" {
		if result, found, err := getByEvent(ctx, transaction, input.EventID); err != nil {
			return Weighing{}, err
		} else if found {
			if err := transaction.Commit(ctx); err != nil {
				return Weighing{}, fmt.Errorf("commit idempotent event: %w", err)
			}
			return result, nil
		}
	}
	if result, found, err := getBySession(ctx, transaction, input.SessionID, input.Stage); err != nil {
		return Weighing{}, err
	} else if found {
		if err := transaction.Commit(ctx); err != nil {
			return Weighing{}, fmt.Errorf("commit idempotent session: %w", err)
		}
		return result, nil
	}

	transport, err := lockTransport(ctx, transaction, input)
	if err != nil {
		return Weighing{}, err
	}
	if transport.status != "OPEN" {
		return Weighing{}, fmt.Errorf("%w: transport transaction is %s", domain.ErrConflict, transport.status)
	}
	if input.GrossWeightGrams <= transport.tareWeightGrams {
		return Weighing{}, fmt.Errorf("%w: net weight must be greater than zero", domain.ErrValidation)
	}
	netWeight := input.GrossWeightGrams - transport.tareWeightGrams

	var cost int64
	if err := transaction.QueryRow(ctx, `SELECT ROUND(($1::NUMERIC * $2::NUMERIC) / $3::NUMERIC)::BIGINT`, netWeight, transport.purchasePriceMinor, gramsPerMetricTon).Scan(&cost); err != nil {
		return Weighing{}, fmt.Errorf("calculate load cost: %w", err)
	}
	var inventory int64
	if err := transaction.QueryRow(ctx, `
		INSERT INTO branch_grain_inventory (branch_id, grain_type_id, current_inventory_grams)
		VALUES ($1, $2, $3)
		ON CONFLICT (branch_id, grain_type_id) DO UPDATE
		SET current_inventory_grams = branch_grain_inventory.current_inventory_grams + EXCLUDED.current_inventory_grams,
		    updated_at = now()
		RETURNING current_inventory_grams`, transport.branchID, transport.grainTypeID, netWeight).Scan(&inventory); err != nil {
		return Weighing{}, fmt.Errorf("update branch grain inventory: %w", err)
	}
	var margin int32
	if err := transaction.QueryRow(ctx, `
		SELECT GREATEST(500, 2000 - FLOOR(1500::NUMERIC * LEAST($1::BIGINT, $2::BIGINT)::NUMERIC / $2::NUMERIC)::INTEGER)`, inventory, transport.inventoryTargetGrams).Scan(&margin); err != nil {
		return Weighing{}, fmt.Errorf("calculate applied margin: %w", err)
	}

	result := Weighing{
		ID: uuid.NewString(), SessionID: input.SessionID, Stage: input.Stage, EventID: input.EventID,
		TransportTransactionID: transport.id, BranchID: transport.branchID, GrainTypeID: transport.grainTypeID, ScaleID: transport.scaleID,
		GrossWeightGrams: input.GrossWeightGrams, TareWeightGrams: transport.tareWeightGrams, NetWeightGrams: netWeight,
		LoadCostMinor: cost, PurchasePriceMinor: transport.purchasePriceMinor, AppliedMarginBPS: margin, WeighedAt: input.WeighedAt.UTC(),
	}
	if err := insertWeighing(ctx, transaction, result, input); err != nil {
		return Weighing{}, classifyConstraint(err)
	}
	command, err := transaction.Exec(ctx, `UPDATE transport_transactions SET status = 'WEIGHED', updated_at = now() WHERE id = $1 AND status = 'OPEN'`, transport.id)
	if err != nil {
		return Weighing{}, fmt.Errorf("transition transport transaction: %w", err)
	}
	if command.RowsAffected() != 1 {
		return Weighing{}, fmt.Errorf("%w: transport transaction changed during finalization", domain.ErrConflict)
	}
	if err := transaction.Commit(ctx); err != nil {
		return Weighing{}, classifyConstraint(fmt.Errorf("commit finalization: %w", err))
	}
	return result, nil
}

type lockedTransport struct {
	id, status, branchID, grainTypeID, scaleID                string
	tareWeightGrams, purchasePriceMinor, inventoryTargetGrams int64
}

func lockTransport(ctx context.Context, transaction pgx.Tx, input Input) (lockedTransport, error) {
	var result lockedTransport
	var err error
	if input.TransportTransactionID != "" {
		err = transaction.QueryRow(ctx, `
			SELECT t.id, t.status, t.branch_id, t.grain_type_id, sc.id, tr.tare_weight_grams,
			       t.purchase_price_minor_snapshot, g.inventory_target_grams
			FROM transport_transactions t
			JOIN trucks tr ON tr.id = t.truck_id
			JOIN grain_types g ON g.id = t.grain_type_id
			JOIN scales sc ON sc.scale_id = $2 AND sc.branch_id = t.branch_id
			WHERE t.id = $1
			FOR UPDATE OF t`, input.TransportTransactionID, input.ScaleID).Scan(
			&result.id, &result.status, &result.branchID, &result.grainTypeID, &result.scaleID, &result.tareWeightGrams, &result.purchasePriceMinor, &result.inventoryTargetGrams)
	} else {
		err = transaction.QueryRow(ctx, `
			SELECT t.id, t.status, t.branch_id, t.grain_type_id, sc.id, tr.tare_weight_grams,
			       t.purchase_price_minor_snapshot, g.inventory_target_grams
			FROM transport_transactions t
			JOIN trucks tr ON tr.id = t.truck_id
			JOIN grain_types g ON g.id = t.grain_type_id
			JOIN scales sc ON sc.scale_id = $1 AND sc.branch_id = t.branch_id
			WHERE tr.plate = $2 AND t.status = 'OPEN'
			FOR UPDATE OF t`, input.ScaleID, input.Plate).Scan(
			&result.id, &result.status, &result.branchID, &result.grainTypeID, &result.scaleID, &result.tareWeightGrams, &result.purchasePriceMinor, &result.inventoryTargetGrams)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedTransport{}, fmt.Errorf("%w: OPEN transport transaction", domain.ErrNotFound)
	}
	if err != nil {
		return lockedTransport{}, fmt.Errorf("lock transport transaction: %w", err)
	}
	return result, nil
}

func insertWeighing(ctx context.Context, transaction pgx.Tx, value Weighing, input Input) error {
	_, err := transaction.Exec(ctx, `
		INSERT INTO weighings (
			id, session_id, stage, event_id, transport_transaction_id, branch_id, grain_type_id, scale_id,
			gross_weight_grams, tare_weight_grams, net_weight_grams, load_cost_minor,
			purchase_price_minor_snapshot, applied_margin_bps, algorithm_version, sample_count,
			dispersion_grams, slope, weighed_at
		) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		value.ID, value.SessionID, value.Stage, value.EventID, value.TransportTransactionID, value.BranchID, value.GrainTypeID, value.ScaleID,
		value.GrossWeightGrams, value.TareWeightGrams, value.NetWeightGrams, value.LoadCostMinor, value.PurchasePriceMinor,
		value.AppliedMarginBPS, input.AlgorithmVersion, input.SampleCount, input.DispersionGrams, input.Slope, value.WeighedAt)
	return err
}

func getByEvent(ctx context.Context, transaction pgx.Tx, eventID string) (Weighing, bool, error) {
	return getOne(ctx, transaction, `SELECT id, session_id, stage, COALESCE(event_id, ''), transport_transaction_id, branch_id, grain_type_id, scale_id, gross_weight_grams, tare_weight_grams, net_weight_grams, load_cost_minor, purchase_price_minor_snapshot, applied_margin_bps, weighed_at FROM weighings WHERE event_id = $1`, eventID)
}
func getBySession(ctx context.Context, transaction pgx.Tx, sessionID, stage string) (Weighing, bool, error) {
	return getOne(ctx, transaction, `SELECT id, session_id, stage, COALESCE(event_id, ''), transport_transaction_id, branch_id, grain_type_id, scale_id, gross_weight_grams, tare_weight_grams, net_weight_grams, load_cost_minor, purchase_price_minor_snapshot, applied_margin_bps, weighed_at FROM weighings WHERE session_id = $1 AND stage = $2`, sessionID, stage)
}
func getOne(ctx context.Context, transaction pgx.Tx, query string, args ...any) (Weighing, bool, error) {
	var value Weighing
	err := transaction.QueryRow(ctx, query, args...).Scan(&value.ID, &value.SessionID, &value.Stage, &value.EventID, &value.TransportTransactionID, &value.BranchID, &value.GrainTypeID, &value.ScaleID, &value.GrossWeightGrams, &value.TareWeightGrams, &value.NetWeightGrams, &value.LoadCostMinor, &value.PurchasePriceMinor, &value.AppliedMarginBPS, &value.WeighedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Weighing{}, false, nil
	}
	if err != nil {
		return Weighing{}, false, fmt.Errorf("read idempotent weighing: %w", err)
	}
	return value, true, nil
}

func validate(input Input) error {
	if strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Stage) == "" || strings.TrimSpace(input.ScaleID) == "" || input.GrossWeightGrams <= 0 || input.SampleCount <= 0 || input.DispersionGrams < 0 || strings.TrimSpace(input.AlgorithmVersion) == "" || strings.TrimSpace(input.Slope) == "" || input.WeighedAt.IsZero() {
		return fmt.Errorf("%w: incomplete finalization input", domain.ErrValidation)
	}
	if input.EventID != "" {
		if _, err := uuid.Parse(input.EventID); err != nil {
			return fmt.Errorf("%w: event_id must be a UUID", domain.ErrValidation)
		}
	}
	if input.TransportTransactionID == "" && strings.TrimSpace(input.Plate) == "" {
		return fmt.Errorf("%w: transaction ID or plate is required", domain.ErrValidation)
	}
	return nil
}

func classifyConstraint(err error) error {
	// The transaction rollback ensures a uniqueness conflict has no inventory or
	// status side effect. A concurrent winner is subsequently observable by a
	// retry through the idempotency queries above.
	return err
}

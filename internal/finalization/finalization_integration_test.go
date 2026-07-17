package finalization_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"scale-challenge/internal/domain"
	"scale-challenge/internal/finalization"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/testkit"
)

func TestGherkinPersistOneWeighingAndUpdateBusinessState(t *testing.T) {
	h, fixture := seededFinalization(t)
	service := finalization.New(h.DB)
	result, err := service.Finalize(h.Context, input(fixture, "session-001", uuid.NewString(), 2_012_000_000))
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if result.NetWeightGrams != 2_000_000_000 || result.LoadCostMinor != 250_000 || result.AppliedMarginBPS != 1_250 {
		t.Fatalf("financial result = %+v, want net=2t cost=250000 margin=1250", result)
	}
	var inventory int64
	if err := h.DB.QueryRow(h.Context, "SELECT current_inventory_grams FROM branch_grain_inventory WHERE branch_id = $1 AND grain_type_id = $2", fixture.branchID, fixture.grainID).Scan(&inventory); err != nil {
		t.Fatal(err)
	}
	if inventory != 2_000_000_000 {
		t.Fatalf("inventory = %d, want 2000000000", inventory)
	}
	var status string
	if err := h.DB.QueryRow(h.Context, "SELECT status FROM transport_transactions WHERE id = $1", fixture.transactionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "WEIGHED" {
		t.Fatalf("transaction status = %s, want WEIGHED", status)
	}
	var purchase, margin int64
	if err := h.DB.QueryRow(h.Context, "SELECT purchase_price_minor_snapshot, applied_margin_bps FROM weighings WHERE id = $1", result.ID).Scan(&purchase, &margin); err != nil {
		t.Fatal(err)
	}
	if purchase != 125_000 || margin != 1_250 {
		t.Fatalf("snapshots price=%d margin=%d, want 125000/1250", purchase, margin)
	}
}

func TestFinalizationRejectsInvalidNetAndLeavesNoPartialData(t *testing.T) {
	h, fixture := seededFinalization(t)
	_, err := finalization.New(h.DB).Finalize(h.Context, input(fixture, "invalid-net", uuid.NewString(), 12_000_000))
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, want validation", err)
	}
	assertNoFinancialMutation(t, h, fixture.transactionID)
}

func TestFinalizationIsIdempotentByEventID(t *testing.T) {
	h, fixture := seededFinalization(t)
	service := finalization.New(h.DB)
	request := input(fixture, "session-event", uuid.NewString(), 2_012_000_000)
	first, err := service.Finalize(h.Context, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Finalize(h.Context, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate event weighing IDs = %q and %q, want one", first.ID, second.ID)
	}
	var count, inventory int64
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM weighings").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.QueryRow(h.Context, "SELECT current_inventory_grams FROM branch_grain_inventory").Scan(&inventory); err != nil {
		t.Fatal(err)
	}
	if count != 1 || inventory != 2_000_000_000 {
		t.Fatalf("duplicate event count=%d inventory=%d, want 1/2000000000", count, inventory)
	}
}

func TestFinalizationRejectsMissingAndNonOpenTransactionsWithoutPartialData(t *testing.T) {
	h, fixture := seededFinalization(t)
	service := finalization.New(h.DB)
	missing := input(fixture, "missing", uuid.NewString(), 2_012_000_000)
	missing.TransportTransactionID = "does-not-exist"
	if _, err := service.Finalize(h.Context, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing error = %v, want not found", err)
	}
	assertNoFinancialMutation(t, h, fixture.transactionID)
	if _, err := h.DB.Exec(h.Context, "UPDATE transport_transactions SET status = 'CANCELLED' WHERE id = $1", fixture.transactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Finalize(h.Context, input(fixture, "not-open", uuid.NewString(), 2_012_000_000)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("non-open error = %v, want conflict", err)
	}
	assertNoFinancialMutation(t, h, fixture.transactionID)
}

func TestGherkinConcurrentWorkersCreateOnlyOneSessionWeighing(t *testing.T) {
	h, fixture := seededFinalization(t)
	secondTransaction := "transaction-002"
	if _, err := h.DB.Exec(h.Context, `INSERT INTO trucks (id, plate, tare_weight_grams) VALUES ('truck-002', 'XYZ9A99', 12000000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.DB.Exec(h.Context, `INSERT INTO transport_transactions (id, branch_id, truck_id, grain_type_id, status, purchase_price_minor_snapshot, margin_policy_bps_snapshot) VALUES ($1, $2, 'truck-002', $3, 'OPEN', 125000, 2000)`, secondTransaction, fixture.branchID, fixture.grainID); err != nil {
		t.Fatal(err)
	}
	service := finalization.New(h.DB)
	requests := []finalization.Input{
		input(fixture, "same-session", uuid.NewString(), 2_012_000_000),
		func() finalization.Input {
			value := input(fixture, "same-session", uuid.NewString(), 2_012_000_000)
			value.TransportTransactionID = secondTransaction
			return value
		}(),
	}
	results := make(chan error, len(requests))
	var group sync.WaitGroup
	for _, request := range requests {
		request := request
		group.Go(func() { _, err := service.Finalize(context.Background(), request); results <- err })
	}
	group.Wait()
	close(results)
	var succeeded, failed int
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			failed++
		}
	}
	if succeeded+failed != 2 || succeeded == 0 {
		t.Fatalf("concurrent finalizations success=%d failed=%d, want at least one completed result", succeeded, failed)
	}
	var weighings, inventory int64
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM weighings WHERE session_id = 'same-session' AND stage = 'FINAL'").Scan(&weighings); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.QueryRow(h.Context, "SELECT current_inventory_grams FROM branch_grain_inventory").Scan(&inventory); err != nil {
		t.Fatal(err)
	}
	if weighings != 1 || inventory != 2_000_000_000 {
		t.Fatalf("session weighings=%d inventory=%d, want 1/2000000000", weighings, inventory)
	}
}

type fixture struct{ branchID, scaleID, grainID, transactionID string }

func seededFinalization(t *testing.T) (*testkit.Harness, fixture) {
	t.Helper()
	h := testkit.New(t, testkit.WithPostgres())
	if err := migrations.Apply(h.Context, h.DB); err != nil {
		t.Fatal(err)
	}
	if _, err := h.DB.Exec(h.Context, `
		INSERT INTO branches (id, code, name) VALUES ('branch-001', 'BR', 'Branch');
		INSERT INTO scales (id, branch_id, scale_id, name, api_key_hash) VALUES ('scale-001-id', 'branch-001', 'SCALE-001', 'Scale', 'hash');
		INSERT INTO trucks (id, plate, tare_weight_grams) VALUES ('truck-001', 'ABC1D23', 12000000);
		INSERT INTO grain_types (id, code, name, purchase_price_minor, inventory_target_grams, margin_policy_bps) VALUES ('grain-001', 'SOY', 'Soy', 125000, 4000000000, 2000);
		INSERT INTO transport_transactions (id, branch_id, truck_id, grain_type_id, status, purchase_price_minor_snapshot, margin_policy_bps_snapshot) VALUES ('transaction-001', 'branch-001', 'truck-001', 'grain-001', 'OPEN', 125000, 2000);`); err != nil {
		t.Fatalf("seed finalization fixture: %v", err)
	}
	return h, fixture{branchID: "branch-001", scaleID: "SCALE-001", grainID: "grain-001", transactionID: "transaction-001"}
}

func input(f fixture, session, event string, gross int64) finalization.Input {
	return finalization.Input{SessionID: session, Stage: "FINAL", EventID: event, TransportTransactionID: f.transactionID, ScaleID: f.scaleID, GrossWeightGrams: gross, AlgorithmVersion: "t04-v1", SampleCount: 31, DispersionGrams: 60, Slope: "0/1", WeighedAt: time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)}
}

func assertNoFinancialMutation(t *testing.T, h *testkit.Harness, transactionID string) {
	t.Helper()
	var weighings, inventories int64
	var status string
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM weighings").Scan(&weighings); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM branch_grain_inventory").Scan(&inventories); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.QueryRow(h.Context, "SELECT status FROM transport_transactions WHERE id = $1", transactionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if weighings != 0 || inventories != 0 || status != "OPEN" && status != "CANCELLED" {
		t.Fatalf("partial mutation weighings=%d inventories=%d status=%s", weighings, inventories, status)
	}
}

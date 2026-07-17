package reports_test

import (
	"errors"
	"testing"
	"time"

	"scale-challenge/internal/domain"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/reports"
	"scale-challenge/internal/testkit"
)

func TestGherkinQueryFinalizedIndicatorsByFilters(t *testing.T) {
	h := seededReports(t)
	service := reports.New(h.DB)
	result, err := service.Query(h.Context, reports.Filter{BranchID: "branch-001", GrainTypeID: "grain-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.NetWeightGrams != 300 || item.TotalCostMinor != 4_000 || item.AveragePurchasePrice != 20 || item.AverageAppliedMarginBPS != 1_500 || item.CompletedTransportCount != 2 {
		t.Fatalf("item = %+v, want aggregated finalized branch/grain values", item)
	}
	start := time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC)
	result, err = service.Query(h.Context, reports.Filter{Start: &start})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Totals.CompletedTransportCount != 2 || result.Totals.NetWeightGrams != 500 {
		t.Fatalf("date filtered result = %+v, want two completed weighings totaling 500 grams", result)
	}
}

func TestGherkinRejectInvalidOrReturnEmptyRange(t *testing.T) {
	h := seededReports(t)
	service := reports.New(h.DB)
	start := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.Query(h.Context, reports.Filter{Start: &start, End: &end}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid range error = %v, want validation", err)
	}
	start = time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	result, err := service.Query(h.Context, reports.Filter{Start: &start})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.Totals.NetWeightGrams != 0 || result.Totals.TotalCostMinor != 0 || result.Totals.CompletedTransportCount != 0 {
		t.Fatalf("empty result = %+v, want zero totals and non-nil empty items", result)
	}
}

func seededReports(t *testing.T) *testkit.Harness {
	t.Helper()
	h := testkit.New(t, testkit.WithPostgres())
	if err := migrations.Apply(h.Context, h.DB); err != nil {
		t.Fatal(err)
	}
	_, err := h.DB.Exec(h.Context, `
		INSERT INTO branches (id, code, name) VALUES ('branch-001', 'B1', 'Branch 1'), ('branch-002', 'B2', 'Branch 2');
		INSERT INTO scales (id, branch_id, scale_id, name, api_key_hash) VALUES ('scale-001', 'branch-001', 'SCALE-001', 'Scale 1', 'hash'), ('scale-002', 'branch-002', 'SCALE-002', 'Scale 2', 'hash');
		INSERT INTO trucks (id, plate, tare_weight_grams) VALUES ('truck-001', 'AAA1A11', 1), ('truck-002', 'BBB2B22', 1), ('truck-003', 'CCC3C33', 1);
		INSERT INTO grain_types (id, code, name, purchase_price_minor, inventory_target_grams, margin_policy_bps) VALUES ('grain-001', 'SOY', 'Soy', 10, 1000, 2000), ('grain-002', 'CORN', 'Corn', 20, 1000, 2000);
		INSERT INTO transport_transactions (id, branch_id, truck_id, grain_type_id, status, purchase_price_minor_snapshot, margin_policy_bps_snapshot) VALUES ('tx-001', 'branch-001', 'truck-001', 'grain-001', 'WEIGHED', 10, 2000), ('tx-002', 'branch-001', 'truck-002', 'grain-001', 'WEIGHED', 30, 1000), ('tx-003', 'branch-002', 'truck-003', 'grain-002', 'WEIGHED', 30, 500);
		INSERT INTO weighings (id, session_id, stage, transport_transaction_id, branch_id, grain_type_id, scale_id, gross_weight_grams, tare_weight_grams, net_weight_grams, load_cost_minor, purchase_price_minor_snapshot, applied_margin_bps, algorithm_version, sample_count, dispersion_grams, slope, weighed_at) VALUES
		('w-001', 's-001', 'FINAL', 'tx-001', 'branch-001', 'grain-001', 'scale-001', 101, 1, 100, 1000, 10, 2000, 't04-v1', 31, 1, '0/1', '2026-07-01T12:00:00Z'),
		('w-002', 's-002', 'FINAL', 'tx-002', 'branch-001', 'grain-001', 'scale-001', 201, 1, 200, 3000, 30, 1000, 't04-v1', 31, 1, '0/1', '2026-07-02T12:00:00Z'),
		('w-003', 's-003', 'FINAL', 'tx-003', 'branch-002', 'grain-002', 'scale-002', 301, 1, 300, 9000, 30, 500, 't04-v1', 31, 1, '0/1', '2026-07-03T12:00:00Z');`)
	if err != nil {
		t.Fatalf("seed reporting fixture: %v", err)
	}
	return h
}

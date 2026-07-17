package worker_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/finalization"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/stabilizer"
	"scale-challenge/internal/testkit"
	"scale-challenge/internal/worker"
)

func TestGherkinWorkerFinalizesTwoStableWindowsInOneTransaction(t *testing.T) {
	h := testkit.New(t, testkit.WithPostgres(), testkit.WithRedis())
	if err := migrations.Apply(h.Context, h.DB); err != nil {
		t.Fatal(err)
	}
	seedWorkerFinalization(t, h)
	manager, err := stabilizer.New(stabilizer.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	processor, err := worker.NewFinalizingProcessor(manager, worker.NewPostgresLedger(h.DB), finalization.New(h.DB))
	if err != nil {
		t.Fatal(err)
	}
	config := worker.DefaultConfig()
	config.ConsumerName = "finalization-worker"
	config.BatchSize = 64
	config.BlockTimeout = 50 * time.Millisecond
	instance, err := worker.New(h.Redis, processor, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Ensure(h.Context); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	for index := range 31 {
		weight := int64(2_012_000_000)
		if index%3 == 1 {
			weight += 20
		}
		if index%3 == 2 {
			weight -= 20
		}
		if _, err := h.Redis.XAdd(h.Context, &redis.XAddArgs{Stream: "scale-readings", Values: map[string]any{
			"event_id": uuid.NewString(), "scale_id": "SCALE-001", "plate": "ABC1D23", "weight_grams": weight,
			"measured_at": base.Add(time.Duration(index) * 200 * time.Millisecond).Format(time.RFC3339Nano), "received_at": base.Add(time.Duration(index) * 200 * time.Millisecond).Format(time.RFC3339Nano),
		}}).Result(); err != nil {
			t.Fatalf("publish reading %d: %v", index, err)
		}
	}
	processed, err := instance.ProcessNew(h.Context)
	if err != nil {
		t.Fatalf("process stable reading series: %v", err)
	}
	if processed != 31 {
		t.Fatalf("processed = %d, want 31", processed)
	}
	if pending, err := instance.Pending(h.Context); err != nil || pending != 0 {
		t.Fatalf("pending = %d, %v; want 0, nil", pending, err)
	}
	var weighings, inventory int64
	var status string
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM weighings WHERE session_id = 'SCALE-001:ABC1D23' AND stage = 'FINAL'").Scan(&weighings); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.QueryRow(h.Context, "SELECT current_inventory_grams FROM branch_grain_inventory").Scan(&inventory); err != nil {
		t.Fatal(err)
	}
	if err := h.DB.QueryRow(h.Context, "SELECT status FROM transport_transactions WHERE id = 'transaction-001'").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if weighings != 1 || inventory != 2_000_000_000 || status != "WEIGHED" {
		t.Fatalf("weighings=%d inventory=%d status=%s, want 1/2000000000/WEIGHED", weighings, inventory, status)
	}
}

func seedWorkerFinalization(t *testing.T, h *testkit.Harness) {
	t.Helper()
	_, err := h.DB.Exec(h.Context, `
		INSERT INTO branches (id, code, name) VALUES ('branch-001', 'BR', 'Branch');
		INSERT INTO scales (id, branch_id, scale_id, name, api_key_hash) VALUES ('scale-001-id', 'branch-001', 'SCALE-001', 'Scale', 'hash');
		INSERT INTO trucks (id, plate, tare_weight_grams) VALUES ('truck-001', 'ABC1D23', 12000000);
		INSERT INTO grain_types (id, code, name, purchase_price_minor, inventory_target_grams, margin_policy_bps) VALUES ('grain-001', 'SOY', 'Soy', 125000, 4000000000, 2000);
		INSERT INTO transport_transactions (id, branch_id, truck_id, grain_type_id, status, purchase_price_minor_snapshot, margin_policy_bps_snapshot) VALUES ('transaction-001', 'branch-001', 'truck-001', 'grain-001', 'OPEN', 125000, 2000);`)
	if err != nil {
		t.Fatalf("seed worker finalization fixture: %v", err)
	}
}

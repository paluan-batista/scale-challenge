package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/migrations"
	"scale-challenge/internal/testkit"
	"scale-challenge/internal/worker"
)

func TestGherkinProcessPersistAndAcknowledgePendingReading(t *testing.T) {
	h := integrationHarness(t)
	w := newWorker(t, h, worker.NewPostgresLedger(h.DB), "recovery-worker")
	if err := w.Ensure(h.Context); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	id := addValidEvent(t, h)
	claimPending(t, h, "stopped-worker")
	if pending := pendingCount(t, h); pending != 1 {
		t.Fatalf("pending before recovery = %d, want 1", pending)
	}

	processed, err := w.Recover(h.Context)
	if err != nil {
		t.Fatalf("recover pending message: %v", err)
	}
	if processed != 1 {
		t.Fatalf("recovered messages = %d, want 1", processed)
	}
	if pending := pendingCount(t, h); pending != 0 {
		t.Fatalf("pending after successful persistence and XACK = %d, want 0", pending)
	}
	var rows int
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM processed_scale_events WHERE stream_message_id = $1", id).Scan(&rows); err != nil {
		t.Fatalf("count persisted event: %v", err)
	}
	if rows != 1 {
		t.Fatalf("persisted events = %d, want 1", rows)
	}
	metrics := w.Metrics()
	if metrics.Processed != 1 || metrics.Reclaimed != 1 {
		t.Fatalf("metrics = %+v, want one processed and reclaimed event", metrics)
	}
	if got, err := w.Pending(h.Context); err != nil || got != 0 {
		t.Fatalf("sample pending metric = %d, %v; want 0, nil", got, err)
	}
}

func TestGherkinRecoverAfterPersistenceBeforeXACKWithoutDuplicate(t *testing.T) {
	h := integrationHarness(t)
	base := worker.NewPostgresLedger(h.DB)
	attemptCtx, cancel := context.WithCancel(h.Context)
	first := newWorker(t, h, cancelAfterPersist{next: base, cancel: cancel}, "crashed-worker")
	if err := first.Ensure(h.Context); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	id := addValidEvent(t, h)
	if _, err := first.ProcessNew(attemptCtx); err == nil {
		t.Fatal("ProcessNew() error = nil, want failed real XACK after cancellation")
	}
	if pending := pendingCount(t, h); pending != 1 {
		t.Fatalf("pending after interrupted XACK = %d, want 1", pending)
	}

	second := newWorker(t, h, base, "recovery-worker")
	if _, err := second.Recover(h.Context); err != nil {
		t.Fatalf("recover after interrupted XACK: %v", err)
	}
	if pending := pendingCount(t, h); pending != 0 {
		t.Fatalf("pending after recovery = %d, want 0", pending)
	}
	var rows int
	if err := h.DB.QueryRow(h.Context, "SELECT count(*) FROM processed_scale_events WHERE stream_message_id = $1", id).Scan(&rows); err != nil {
		t.Fatalf("count idempotent event: %v", err)
	}
	if rows != 1 {
		t.Fatalf("idempotent event rows = %d, want 1", rows)
	}
}

func TestGherkinPermanentFailureAndExhaustedRetryGoToDLQ(t *testing.T) {
	h := integrationHarness(t)
	w := newWorker(t, h, failingProcessor{}, "failure-worker")
	if err := w.Ensure(h.Context); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	id := addValidEvent(t, h)
	if _, err := w.ProcessNew(h.Context); err == nil {
		t.Fatal("transient processing failure was unexpectedly acknowledged")
	}
	if pending := pendingCount(t, h); pending != 1 {
		t.Fatalf("pending after first transient failure = %d, want 1", pending)
	}
	if _, err := w.Recover(h.Context); err != nil {
		t.Fatalf("recover exhausted event: %v", err)
	}
	assertDLQ(t, h, id, 2)
	if pending := pendingCount(t, h); pending != 0 {
		t.Fatalf("pending after DLQ = %d, want 0", pending)
	}
	metrics := w.Metrics()
	if metrics.DLQ != 1 || metrics.Failures < 2 || metrics.Reclaimed != 1 {
		t.Fatalf("metrics = %+v, want DLQ/failure/reclaim evidence", metrics)
	}
}

func TestPermanentInvalidReadingMovesOriginalEventToDLQ(t *testing.T) {
	h := integrationHarness(t)
	w := newWorker(t, h, worker.NewPostgresLedger(h.DB), "invalid-worker")
	if err := w.Ensure(h.Context); err != nil {
		t.Fatalf("ensure group: %v", err)
	}
	id, err := h.Redis.XAdd(h.Context, &redis.XAddArgs{Stream: "scale-readings", Values: map[string]any{
		"event_id": uuid.NewString(), "scale_id": "scale-001", "plate": "ABC1D23", "weight_grams": "0",
		"measured_at": time.Now().UTC().Format(time.RFC3339Nano), "received_at": time.Now().UTC().Format(time.RFC3339Nano),
	}}).Result()
	if err != nil {
		t.Fatalf("add invalid event: %v", err)
	}
	if _, err := w.ProcessNew(h.Context); err != nil {
		t.Fatalf("process invalid event: %v", err)
	}
	assertDLQ(t, h, id, 1)
	if pending := pendingCount(t, h); pending != 0 {
		t.Fatalf("pending after permanent DLQ = %d, want 0", pending)
	}
}

func integrationHarness(t *testing.T) *testkit.Harness {
	t.Helper()
	h := testkit.New(t, testkit.WithPostgres(), testkit.WithRedis())
	if err := migrations.Apply(h.Context, h.DB); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return h
}

func newWorker(t *testing.T, h *testkit.Harness, processor worker.Processor, consumer string) *worker.Worker {
	t.Helper()
	config := worker.DefaultConfig()
	config.ConsumerName = consumer
	config.BatchSize = 8
	config.BlockTimeout = 50 * time.Millisecond
	config.PendingIdleTimeout = 0
	config.RetryLimit = 2
	instance, err := worker.New(h.Redis, processor, config)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	return instance
}

func addValidEvent(t *testing.T, h *testkit.Harness) string {
	t.Helper()
	id, err := h.Redis.XAdd(h.Context, &redis.XAddArgs{Stream: "scale-readings", Values: map[string]any{
		"event_id": uuid.NewString(), "scale_id": "scale-001", "plate": "ABC1D23", "weight_grams": "42850300",
		"measured_at": time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), "received_at": time.Now().UTC().Format(time.RFC3339Nano),
	}}).Result()
	if err != nil {
		t.Fatalf("add valid event: %v", err)
	}
	return id
}

func claimPending(t *testing.T, h *testkit.Harness, consumer string) {
	t.Helper()
	streams, err := h.Redis.XReadGroup(h.Context, &redis.XReadGroupArgs{Group: worker.ConsumerGroup, Consumer: consumer, Streams: []string{"scale-readings", ">"}, Count: 8}).Result()
	if err != nil || len(streams) != 1 || len(streams[0].Messages) != 1 {
		t.Fatalf("create real pending message: streams=%+v err=%v", streams, err)
	}
}

func pendingCount(t *testing.T, h *testkit.Harness) int64 {
	t.Helper()
	pending, err := h.Redis.XPending(h.Context, "scale-readings", worker.ConsumerGroup).Result()
	if err != nil {
		t.Fatalf("inspect real pending entries: %v", err)
	}
	return pending.Count
}

func assertDLQ(t *testing.T, h *testkit.Harness, sourceID string, attempts int64) {
	t.Helper()
	messages, err := h.Redis.XRangeN(h.Context, worker.DeadLetterStream, "-", "+", 10).Result()
	if err != nil {
		t.Fatalf("inspect real DLQ: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("DLQ messages = %d, want 1", len(messages))
	}
	values := messages[0].Values
	if got := fmt.Sprint(values["source_message_id"]); got != sourceID {
		t.Fatalf("DLQ source ID = %q, want %q", got, sourceID)
	}
	if got := fmt.Sprint(values["attempt_count"]); got != fmt.Sprint(attempts) {
		t.Fatalf("DLQ attempt_count = %q, want %d", got, attempts)
	}
	if fmt.Sprint(values["reason"]) == "" {
		t.Fatal("DLQ reason is empty")
	}
	var original map[string]any
	if err := json.Unmarshal([]byte(fmt.Sprint(values["event"])), &original); err != nil {
		t.Fatalf("decode DLQ original event: %v", err)
	}
	if fmt.Sprint(original["event_id"]) == "" {
		t.Fatalf("DLQ original event missing event_id: %+v", original)
	}
}

type cancelAfterPersist struct {
	next   worker.Processor
	cancel context.CancelFunc
}

func (p cancelAfterPersist) Process(ctx context.Context, event worker.Event) error {
	err := p.next.Process(ctx, event)
	p.cancel()
	return err
}

type failingProcessor struct{}

func (failingProcessor) Process(context.Context, worker.Event) error {
	return errors.New("simulated PostgreSQL network timeout")
}

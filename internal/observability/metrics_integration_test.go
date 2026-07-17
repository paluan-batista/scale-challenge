package observability_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/observability"
	"scale-challenge/internal/testkit"
)

func TestMetricsExposeRealRedisCountersAndStreamState(t *testing.T) {
	h := testkit.New(t, testkit.WithRedis())
	if err := h.Redis.XGroupCreateMkStream(h.Context, "scale-readings", "weighing-workers", "0").Err(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := h.Redis.XAdd(h.Context, &redis.XAddArgs{Stream: "scale-readings", Values: map[string]any{"event": "reading"}}).Result(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.Redis.XReadGroup(h.Context, &redis.XReadGroupArgs{Group: "weighing-workers", Consumer: "metrics-test", Streams: []string{"scale-readings", ">"}, Count: 1}).Result(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Redis.XAdd(h.Context, &redis.XAddArgs{Stream: "scale-readings-dlq", Values: map[string]any{"event": "dead-letter"}}).Result(); err != nil {
		t.Fatal(err)
	}
	counters := observability.NewRedisCounters(h.Redis)
	for _, name := range []string{observability.IngestionAccepted, observability.IngestionFailures, observability.StabilizationFinalized, observability.ProcessingFailures} {
		counters.Inc(h.Context, name)
	}

	response := httptest.NewRecorder()
	counters.Handler("scale-readings", "weighing-workers", "scale-readings-dlq").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := response.Body.String()
	for _, metric := range []string{"scale_ingestion_accepted_total 1", "scale_ingestion_failures_total 1", "scale_stabilization_finalized_total 1", "scale_processing_failures_total 1", "scale_stream_lag", "scale_pending_messages 1", "scale_dead_letter_messages 1"} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics body missing %q:\n%s", metric, body)
		}
	}
	if !observability.IsPrometheus(response.Header().Get("Content-Type")) {
		t.Fatalf("metrics content type = %q", response.Header().Get("Content-Type"))
	}
}

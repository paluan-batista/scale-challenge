// Package observability supplies small Prometheus-text operational metrics
// without adding a runtime framework dependency.
package observability

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	IngestionAccepted      = "ingestion_accepted"
	IngestionFailures      = "ingestion_failures"
	StabilizationFinalized = "stabilization_finalized"
	ProcessingFailures     = "processing_failures"
)

const metricKeyPrefix = "scale:metrics:"

type Counter interface{ Inc(context.Context, string) }

type RedisCounters struct{ client redis.UniversalClient }

func NewRedisCounters(client redis.UniversalClient) *RedisCounters {
	return &RedisCounters{client: client}
}
func (c *RedisCounters) Inc(ctx context.Context, name string) {
	_ = c.client.Incr(ctx, metricKeyPrefix+name).Err()
}

type Snapshot struct {
	IngestionAccepted      int64
	IngestionFailures      int64
	StabilizationFinalized int64
	ProcessingFailures     int64
	StreamLag              int64
	PendingMessages        int64
	DeadLetterMessages     int64
}

func (c *RedisCounters) Snapshot(ctx context.Context, stream, group, dlq string) Snapshot {
	result := Snapshot{}
	values, err := c.client.MGet(ctx, metricKeyPrefix+IngestionAccepted, metricKeyPrefix+IngestionFailures, metricKeyPrefix+StabilizationFinalized, metricKeyPrefix+ProcessingFailures).Result()
	if err == nil {
		fields := []*int64{&result.IngestionAccepted, &result.IngestionFailures, &result.StabilizationFinalized, &result.ProcessingFailures}
		for index, value := range values {
			if value != nil {
				*fields[index], _ = strconv.ParseInt(fmt.Sprint(value), 10, 64)
			}
		}
	}
	if groups, err := c.client.XInfoGroups(ctx, stream).Result(); err == nil {
		for _, candidate := range groups {
			if candidate.Name == group {
				result.StreamLag, result.PendingMessages = candidate.Lag, candidate.Pending
				break
			}
		}
	}
	if length, err := c.client.XLen(ctx, dlq).Result(); err == nil {
		result.DeadLetterMessages = length
	}
	return result
}

func (c *RedisCounters) Handler(stream, group, dlq string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := c.Snapshot(r.Context(), stream, group, dlq)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		for _, metric := range []struct {
			name  string
			value int64
			kind  string
		}{
			{"scale_ingestion_accepted_total", value.IngestionAccepted, "counter"}, {"scale_ingestion_failures_total", value.IngestionFailures, "counter"},
			{"scale_stabilization_finalized_total", value.StabilizationFinalized, "counter"}, {"scale_processing_failures_total", value.ProcessingFailures, "counter"},
			{"scale_stream_lag", value.StreamLag, "gauge"}, {"scale_pending_messages", value.PendingMessages, "gauge"}, {"scale_dead_letter_messages", value.DeadLetterMessages, "gauge"},
		} {
			_, _ = fmt.Fprintf(w, "# TYPE %s %s\n%s %d\n", metric.name, metric.kind, metric.name, metric.value)
		}
	}
}

func IsPrometheus(contentType string) bool {
	return strings.HasPrefix(contentType, "text/plain; version=0.0.4")
}

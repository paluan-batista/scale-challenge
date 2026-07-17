// Package worker reliably consumes scale-reading Redis Stream events. It keeps
// Redis as a transport buffer and records only an idempotency ledger in
// PostgreSQL; financial finalization deliberately belongs to T06.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/domain"
	"scale-challenge/internal/repository"
)

const (
	ConsumerGroup       = "weighing-workers"
	DeadLetterStream    = "scale-readings-dlq"
	retryKeyPrefix      = "scale-readings:attempt:"
	defaultBatchSize    = 32
	defaultBlockTimeout = time.Second
	defaultPendingIdle  = 30 * time.Second
	defaultRetryLimit   = 3
	defaultRetryKeyTTL  = 24 * time.Hour
)

// Config bounds Redis reads and recovery work. ConsumerName must be unique to
// a worker process; NewConsumerName provides a safe default for callers.
type Config struct {
	Stream             string
	Group              string
	ConsumerName       string
	BatchSize          int64
	BlockTimeout       time.Duration
	PendingIdleTimeout time.Duration
	RetryLimit         int64
	RetryKeyTTL        time.Duration
}

func DefaultConfig() Config {
	return Config{
		Stream:             repository.ScaleReadingsStream,
		Group:              ConsumerGroup,
		ConsumerName:       NewConsumerName(),
		BatchSize:          defaultBatchSize,
		BlockTimeout:       defaultBlockTimeout,
		PendingIdleTimeout: defaultPendingIdle,
		RetryLimit:         defaultRetryLimit,
		RetryKeyTTL:        defaultRetryKeyTTL,
	}
}

func NewConsumerName() string { return "worker-" + uuid.NewString() }

func (c Config) validate() error {
	if strings.TrimSpace(c.Stream) == "" || strings.TrimSpace(c.Group) == "" || strings.TrimSpace(c.ConsumerName) == "" {
		return errors.New("stream, group, and consumer name are required")
	}
	if c.BatchSize < 1 || c.BlockTimeout <= 0 || c.PendingIdleTimeout < 0 || c.RetryLimit < 1 || c.RetryKeyTTL <= 0 {
		return errors.New("invalid worker stream configuration")
	}
	return nil
}

// Event is the validated form of a raw Stream message. Its raw values are
// retained only for an eventual DLQ envelope.
type Event struct {
	StreamID    string
	EventID     string
	ScaleID     string
	Plate       string
	WeightGrams int64
	MeasuredAt  time.Time
	ReceivedAt  time.Time
	Original    map[string]any
}

// PermanentError marks an invalid event. All other processor errors are
// treated as transient to avoid losing work during Redis/PostgreSQL outages.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Processor is the persistence/session integration point. Implementations
// must be idempotent by EventID because an ACK can fail after persistence.
type Processor interface {
	Process(context.Context, Event) error
}

// PostgresLedger records completed events without storing their raw readings.
// The two unique keys make recovery after an ACK failure a no-op in PostgreSQL.
type PostgresLedger struct{ pool *pgxpool.Pool }

func NewPostgresLedger(pool *pgxpool.Pool) *PostgresLedger { return &PostgresLedger{pool: pool} }

func (p *PostgresLedger) Process(ctx context.Context, event Event) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO processed_scale_events (event_id, stream_message_id, scale_id, plate)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) DO NOTHING`, event.EventID, event.StreamID, event.ScaleID, event.Plate)
	return err
}

// Metrics contains monotonic counters. Pending is a sampled Redis PEL count
// refreshed by Pending, rather than an in-process approximation.
type Metrics struct {
	processed atomic.Uint64
	pending   atomic.Uint64
	reclaimed atomic.Uint64
	dlq       atomic.Uint64
	failures  atomic.Uint64
}

type MetricSnapshot struct {
	Processed uint64
	Pending   uint64
	Reclaimed uint64
	DLQ       uint64
	Failures  uint64
}

func (m *Metrics) Snapshot() MetricSnapshot {
	return MetricSnapshot{Processed: m.processed.Load(), Pending: m.pending.Load(), Reclaimed: m.reclaimed.Load(), DLQ: m.dlq.Load(), Failures: m.failures.Load()}
}

// Worker has no background goroutines; Run is cancellable and all Redis/SQL
// calls use the context given by its caller.
type Worker struct {
	redis     redis.UniversalClient
	processor Processor
	config    Config
	metrics   Metrics
	recoverMu sync.Mutex
	claimFrom string
}

func New(client redis.UniversalClient, processor Processor, config Config) (*Worker, error) {
	if client == nil || processor == nil {
		return nil, errors.New("redis client and processor are required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Worker{redis: client, processor: processor, config: config, claimFrom: "0-0"}, nil
}

func (w *Worker) Metrics() MetricSnapshot { return w.metrics.Snapshot() }

// Ensure creates the source stream and its stable consumer group. Starting at
// zero means an outage during deployment cannot silently strand prior events.
func (w *Worker) Ensure(ctx context.Context) error {
	err := w.redis.XGroupCreateMkStream(ctx, w.config.Stream, w.config.Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create stream group: %w", err)
	}
	return nil
}

// Run continuously reclaims idle work before reading bounded batches of new
// messages. Transient Redis/PostgreSQL errors leave PEL entries untouched and
// are retried after the configured block interval rather than stopping a live
// worker process.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.Ensure(ctx); err != nil {
		return err
	}
	for ctx.Err() == nil {
		if _, err := w.Recover(ctx); err != nil && ctx.Err() == nil {
			w.metrics.failures.Add(1)
			if !w.retryPause(ctx) {
				break
			}
			continue
		}
		if _, err := w.ProcessNew(ctx); err != nil && !errors.Is(err, redis.Nil) && ctx.Err() == nil {
			// processMessage already counts a processing failure. An XREADGROUP
			// transport failure is still retried and cannot ACK anything.
			if !w.retryPause(ctx) {
				break
			}
			continue
		}
		if _, err := w.Pending(ctx); err != nil && ctx.Err() == nil {
			w.metrics.failures.Add(1)
		}
	}
	return ctx.Err()
}

func (w *Worker) retryPause(ctx context.Context) bool {
	timer := time.NewTimer(w.config.BlockTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// ProcessNew reads only messages never delivered to this consumer group.
func (w *Worker) ProcessNew(ctx context.Context) (int, error) {
	streams, err := w.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: w.config.Group, Consumer: w.config.ConsumerName, Streams: []string{w.config.Stream, ">"}, Count: w.config.BatchSize, Block: w.config.BlockTimeout,
	}).Result()
	if err != nil {
		return 0, err
	}
	return w.processStreams(ctx, streams)
}

// Recover claims at most one bounded batch of idle pending messages. Repeated
// calls continue from Redis's cursor so no PEL scan or recovery cycle is
// unbounded.
func (w *Worker) Recover(ctx context.Context) (int, error) {
	w.recoverMu.Lock()
	defer w.recoverMu.Unlock()
	messages, next, err := w.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: w.config.Stream, Group: w.config.Group, Consumer: w.config.ConsumerName,
		MinIdle: w.config.PendingIdleTimeout, Start: w.claimFrom, Count: w.config.BatchSize,
	}).Result()
	if err != nil {
		return 0, err
	}
	if next == "0-0" {
		w.claimFrom = "0-0"
	} else {
		w.claimFrom = next
	}
	if len(messages) == 0 {
		return 0, nil
	}
	w.metrics.reclaimed.Add(uint64(len(messages)))
	return w.processMessages(ctx, messages)
}

func (w *Worker) processStreams(ctx context.Context, streams []redis.XStream) (int, error) {
	processed := 0
	for _, stream := range streams {
		count, err := w.processMessages(ctx, stream.Messages)
		processed += count
		if err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (w *Worker) processMessages(ctx context.Context, messages []redis.XMessage) (int, error) {
	processed := 0
	for _, message := range messages {
		if err := w.processMessage(ctx, message); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (w *Worker) processMessage(ctx context.Context, message redis.XMessage) error {
	attempts, err := w.incrementAttempt(ctx, message.ID)
	if err != nil {
		w.metrics.failures.Add(1)
		return fmt.Errorf("increment retry attempt: %w", err)
	}

	event, parseErr := parseEvent(message)
	if parseErr != nil {
		w.metrics.failures.Add(1)
		return w.deadLetter(ctx, message, attempts, parseErr.Error())
	}
	if err := w.processor.Process(ctx, event); err != nil {
		w.metrics.failures.Add(1)
		var permanent *PermanentError
		if errors.As(err, &permanent) {
			return w.deadLetter(ctx, message, attempts, permanent.Error())
		}
		if attempts >= w.config.RetryLimit {
			return w.deadLetter(ctx, message, attempts, "retry limit exhausted: "+err.Error())
		}
		// A transient PostgreSQL failure intentionally leaves this PEL entry
		// unacknowledged for XAUTOCLAIM recovery.
		return fmt.Errorf("process stream message %s: %w", message.ID, err)
	}

	if err := w.redis.XAck(ctx, w.config.Stream, w.config.Group, message.ID).Err(); err != nil {
		w.metrics.failures.Add(1)
		return fmt.Errorf("ack processed message %s: %w", message.ID, err)
	}
	if err := w.redis.Del(ctx, retryKey(message.ID)).Err(); err != nil {
		// The durable event is already safe and acknowledged. TTL bounds this
		// cleanup failure; returning it prevents pretending Redis was healthy.
		w.metrics.failures.Add(1)
		return fmt.Errorf("delete retry metadata for %s: %w", message.ID, err)
	}
	w.metrics.processed.Add(1)
	return nil
}

func (w *Worker) incrementAttempt(ctx context.Context, messageID string) (int64, error) {
	key := retryKey(messageID)
	attempts, err := w.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if err := w.redis.Expire(ctx, key, w.config.RetryKeyTTL).Err(); err != nil {
		return 0, err
	}
	return attempts, nil
}

// deadLetter uses one Redis Lua transaction. The original event is added to
// the DLQ before its source PEL entry is acknowledged, so a Redis failure can
// never ACK and discard the event between those two operations.
func (w *Worker) deadLetter(ctx context.Context, message redis.XMessage, attempts int64, reason string) error {
	original, err := json.Marshal(message.Values)
	if err != nil {
		return fmt.Errorf("encode original event for DLQ: %w", err)
	}
	_, err = w.redis.Eval(ctx, `
local id = redis.call('XADD', KEYS[1], '*', 'source_stream', ARGV[1], 'source_message_id', ARGV[2], 'event', ARGV[3], 'reason', ARGV[4], 'attempt_count', ARGV[5])
redis.call('XACK', KEYS[2], ARGV[6], ARGV[2])
redis.call('DEL', KEYS[3])
return id`, []string{DeadLetterStream, w.config.Stream, retryKey(message.ID)}, w.config.Stream, message.ID, string(original), reason, strconv.FormatInt(attempts, 10), w.config.Group).Result()
	if err != nil {
		return fmt.Errorf("move message %s to DLQ: %w", message.ID, err)
	}
	w.metrics.dlq.Add(1)
	return nil
}

// Pending refreshes the observable PEL size from real Redis.
func (w *Worker) Pending(ctx context.Context) (uint64, error) {
	pending, err := w.redis.XPending(ctx, w.config.Stream, w.config.Group).Result()
	if err != nil {
		return 0, err
	}
	w.metrics.pending.Store(uint64(pending.Count))
	return uint64(pending.Count), nil
}

func retryKey(messageID string) string { return retryKeyPrefix + messageID }

func parseEvent(message redis.XMessage) (Event, error) {
	value := func(name string) string { return strings.TrimSpace(fmt.Sprint(message.Values[name])) }
	event := Event{StreamID: message.ID, EventID: value("event_id"), ScaleID: domain.NormalizeCode(value("scale_id")), Plate: domain.NormalizePlate(value("plate")), Original: message.Values}
	if event.EventID == "" {
		return Event{}, &PermanentError{Err: errors.New("event_id is required")}
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return Event{}, &PermanentError{Err: fmt.Errorf("event_id must be a UUID: %w", err)}
	}
	if event.ScaleID == "" {
		return Event{}, &PermanentError{Err: errors.New("scale_id is required")}
	}
	if err := domain.ValidatePlate(event.Plate); err != nil {
		return Event{}, &PermanentError{Err: err}
	}
	weight, err := strconv.ParseInt(value("weight_grams"), 10, 64)
	if err != nil || weight <= 0 {
		return Event{}, &PermanentError{Err: errors.New("weight_grams must be a positive integer")}
	}
	event.WeightGrams = weight
	measured, err := time.Parse(time.RFC3339Nano, value("measured_at"))
	if err != nil {
		return Event{}, &PermanentError{Err: errors.New("measured_at must be RFC3339Nano")}
	}
	received, err := time.Parse(time.RFC3339Nano, value("received_at"))
	if err != nil {
		return Event{}, &PermanentError{Err: errors.New("received_at must be RFC3339Nano")}
	}
	event.MeasuredAt, event.ReceivedAt = measured.UTC(), received.UTC()
	return event, nil
}

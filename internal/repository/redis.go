// Package repository contains infrastructure adapters for the application
// ports. This file owns the Redis Streams producer used by T03.
package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/domain"
)

const ScaleReadingsStream = "scale-readings"

type RedisReadingPublisher struct{ client redis.UniversalClient }

func NewRedisReadingPublisher(client redis.UniversalClient) *RedisReadingPublisher {
	return &RedisReadingPublisher{client: client}
}

// Publish blocks until Redis acknowledges XADD. Redis is the only persistence
// destination for raw readings at this stage.
func (p *RedisReadingPublisher) Publish(ctx context.Context, reading domain.ScaleReading) error {
	_, err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: ScaleReadingsStream,
		Values: map[string]any{
			"event_id":     reading.EventID,
			"scale_id":     reading.ScaleID,
			"plate":        reading.Plate,
			"weight_grams": strconv.FormatInt(reading.WeightGrams, 10),
			"measured_at":  reading.MeasuredAt.UTC().Format(time.RFC3339Nano),
			"received_at":  reading.ReceivedAt.UTC().Format(time.RFC3339Nano),
		},
	}).Result()
	return err
}

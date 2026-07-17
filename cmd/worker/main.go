package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/bootstrap"
	"scale-challenge/internal/finalization"
	"scale-challenge/internal/stabilizer"
	streamworker "scale-challenge/internal/worker"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := checkHealth(); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	if err := bootstrap.RequiredEnvironment("DATABASE_URL", "REDIS_ADDR"); err != nil {
		log.Print(err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Printf("open PostgreSQL: %v", err)
		return
	}
	defer database.Close()
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	defer client.Close()

	config, err := workerConfigFromEnvironment()
	if err != nil {
		log.Printf("invalid worker configuration: %v", err)
		return
	}
	manager, err := stabilizer.New(stabilizer.DefaultConfig())
	if err != nil {
		log.Printf("initialize stabilizer: %v", err)
		return
	}
	processor, err := streamworker.NewFinalizingProcessor(manager, streamworker.NewPostgresLedger(database), finalization.New(database))
	if err != nil {
		log.Printf("initialize finalization processor: %v", err)
		return
	}
	worker, err := streamworker.New(client, processor, config)
	if err != nil {
		log.Printf("initialize stream worker: %v", err)
		return
	}
	log.Printf("worker started consumer=%s stream=%s group=%s", config.ConsumerName, config.Stream, config.Group)
	if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
		log.Printf("worker stopped with error: %v", err)
		return
	}
	log.Print("worker stopped")
}

func workerConfigFromEnvironment() (streamworker.Config, error) {
	config := streamworker.DefaultConfig()
	if value := strings.TrimSpace(os.Getenv("WORKER_BATCH_SIZE")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return config, fmt.Errorf("WORKER_BATCH_SIZE: %w", err)
		}
		config.BatchSize = parsed
	}
	if value := strings.TrimSpace(os.Getenv("WORKER_BLOCK_TIMEOUT")); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config, fmt.Errorf("WORKER_BLOCK_TIMEOUT: %w", err)
		}
		config.BlockTimeout = parsed
	}
	if value := strings.TrimSpace(os.Getenv("WORKER_PENDING_IDLE_TIMEOUT")); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return config, fmt.Errorf("WORKER_PENDING_IDLE_TIMEOUT: %w", err)
		}
		config.PendingIdleTimeout = parsed
	}
	if value := strings.TrimSpace(os.Getenv("WORKER_RETRY_LIMIT")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return config, fmt.Errorf("WORKER_RETRY_LIMIT: %w", err)
		}
		config.RetryLimit = parsed
	}
	return config, nil
}

// checkHealth verifies that the worker's two required backing services are
// reachable. Stream processing itself deliberately remains outside T01.
func checkHealth() error {
	if err := bootstrap.RequiredEnvironment("DATABASE_URL", "REDIS_ADDR"); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	database, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("open PostgreSQL health connection: %w", err)
	}
	defer database.Close()
	if err := database.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL for worker health: %w", err)
	}

	client := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis for worker health: %w", err)
	}
	return nil
}

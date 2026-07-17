package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/bootstrap"
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

	log.Print("worker bootstrap started; stream processing is scheduled for T04-T06")
	<-ctx.Done()
	log.Print("worker bootstrap stopped")
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/application"
	"scale-challenge/internal/bootstrap"
	"scale-challenge/internal/httpapi"
	"scale-challenge/internal/observability"
	"scale-challenge/internal/reports"
	"scale-challenge/internal/repository"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := checkHealth(); err != nil {
			slog.Error("api health check failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := bootstrap.RequiredEnvironment("DATABASE_URL", "REDIS_ADDR"); err != nil {
		slog.Error("api configuration invalid", "error", err)
		os.Exit(2)
	}
	databaseContext, cancelDatabase := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := pgxpool.New(databaseContext, os.Getenv("DATABASE_URL"))
	if err == nil {
		err = database.Ping(databaseContext)
	}
	cancelDatabase()
	if err != nil {
		if database != nil {
			database.Close()
		}
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	defer redisClient.Close()

	address := os.Getenv("API_ADDR")
	if address == "" {
		address = ":8080"
	}

	service := application.New(repository.NewPostgres(database), repository.NewRedisReadingPublisher(redisClient))
	counters := observability.NewRedisCounters(redisClient)
	handler := httpapi.New(service, httpapi.WithReports(reports.New(database)), httpapi.WithMetrics(counters)).Router()
	server := newServer(address, handler, serverOptions{
		metrics: counters.Handler(repository.ScaleReadingsStream, "weighing-workers", "scale-readings-dlq"),
		readiness: func(ctx context.Context) error {
			if err := database.Ping(ctx); err != nil {
				return err
			}
			return redisClient.Ping(ctx).Err()
		},
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("api shutdown", "error", err)
		}
	}()

	slog.Info("api listening", "address", address)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("api serve", "error", err)
		os.Exit(1)
	}
}

type serverOptions struct {
	readiness func(context.Context) error
	metrics   http.Handler
}

func newServer(address string, applicationHandler http.Handler, options ...serverOptions) *http.Server {
	option := serverOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", liveHandler)
	mux.HandleFunc("GET /health/ready", readyHandler(option.readiness))
	if option.metrics != nil {
		mux.Handle("GET /metrics", option.metrics)
	}
	mux.Handle("/v1/", applicationHandler)

	return &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func liveHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func readyHandler(readiness func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if readiness != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			if err := readiness(ctx); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
				return
			}
		}
		liveHandler(w, r)
	}
}

func checkHealth() error {
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:8080/health/live")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("api liveness probe did not return 200")
	}
	return nil
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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
	"scale-challenge/internal/repository"
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
		log.Printf("connect PostgreSQL: %v", err)
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
	server := newServer(address, httpapi.New(service).Router())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("api shutdown: %v", err)
		}
	}()

	log.Printf("api bootstrap listening on %s", address)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func newServer(address string, applicationHandler http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler)
	mux.HandleFunc("GET /health/ready", healthHandler)
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

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

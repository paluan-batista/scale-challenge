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

	address := os.Getenv("API_ADDR")
	if address == "" {
		address = ":8080"
	}

	server := newServer(address)
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

func newServer(address string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler)
	mux.HandleFunc("GET /health/ready", healthHandler)

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

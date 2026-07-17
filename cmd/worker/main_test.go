package main

import (
	"strings"
	"testing"
)

func TestWorkerHealthCheckRejectsMissingConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_ADDR", "")
	if err := checkHealth(); err == nil {
		t.Fatal("checkHealth() error = nil, want missing configuration error")
	}
}

func TestWorkerHealthCheckDoesNotReadUnrelatedSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("QA_TEST_SECRET", "must-not-appear")
	err := checkHealth()
	if err == nil {
		t.Fatal("checkHealth() error = nil, want missing configuration error")
	}
	if got := err.Error(); strings.Contains(got, "must-not-appear") {
		t.Fatalf("health error leaked secret: %q", got)
	}
}

func TestWorkerConfigReadsBoundedStreamSettings(t *testing.T) {
	t.Setenv("WORKER_BATCH_SIZE", "7")
	t.Setenv("WORKER_BLOCK_TIMEOUT", "250ms")
	t.Setenv("WORKER_PENDING_IDLE_TIMEOUT", "2s")
	t.Setenv("WORKER_RETRY_LIMIT", "4")
	config, err := workerConfigFromEnvironment()
	if err != nil {
		t.Fatalf("workerConfigFromEnvironment() error = %v", err)
	}
	if config.BatchSize != 7 || config.BlockTimeout.String() != "250ms" || config.PendingIdleTimeout.String() != "2s" || config.RetryLimit != 4 {
		t.Fatalf("config = %+v, want configured bounded values", config)
	}
}

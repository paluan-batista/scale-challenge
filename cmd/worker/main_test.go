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

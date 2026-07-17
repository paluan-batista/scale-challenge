package testkit

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewProvidesDeterministicClockWithoutDependencies(t *testing.T) {
	harness := New(t)
	if harness.DB != nil || harness.Redis != nil {
		t.Fatal("New() started a dependency without an explicit option")
	}
	want := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	if got := harness.Clock.Now(); !got.Equal(want) {
		t.Fatalf("clock = %s, want %s", got, want)
	}
}

func TestAPIDriverUsesInProcessServer(t *testing.T) {
	harness := New(t, WithAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	response, err := harness.Request(harness.Context, http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestEventuallyRejectsInvalidInterval(t *testing.T) {
	err := Eventually(context.Background(), 0, func(context.Context) (bool, error) { return true, nil })
	if err == nil {
		t.Fatal("Eventually() error = nil, want invalid interval error")
	}
}

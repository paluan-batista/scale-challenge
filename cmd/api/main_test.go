package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthEndpoints(t *testing.T) {
	server := newServer(":0", http.NotFoundHandler())
	for _, path := range []string{"/health/live", "/health/ready"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()

		server.Handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}
}

func TestReadinessCanReportUnavailableAndMetricsAreExposed(t *testing.T) {
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte("scale_stream_lag 0\n"))
	})
	server := newServer(":0", http.NotFoundHandler(), serverOptions{readiness: func(context.Context) error { return errors.New("postgres unavailable") }, metrics: metrics})
	ready := httptest.NewRecorder()
	server.Handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("unready status = %d, want 503", ready.Code)
	}
	metricResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(metricResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricResponse.Code != http.StatusOK || !strings.Contains(metricResponse.Body.String(), "scale_stream_lag 0") {
		t.Fatalf("metrics response = %d %q", metricResponse.Code, metricResponse.Body.String())
	}
}

func TestServerUsesBoundedHTTPTimeouts(t *testing.T) {
	server := newServer(":0", http.NotFoundHandler())
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 10*time.Second || server.WriteTimeout != 10*time.Second || server.IdleTimeout != 60*time.Second {
		t.Fatalf("timeouts = header:%s read:%s write:%s idle:%s", server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}

func TestHealthEndpointsRejectUnsupportedMethods(t *testing.T) {
	server := newServer(":0", http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPost, "/health/live", nil)
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

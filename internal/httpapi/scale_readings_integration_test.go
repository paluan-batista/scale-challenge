//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/application"
	"scale-challenge/internal/domain"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/repository"
	"scale-challenge/internal/testkit"
)

func TestGherkinAcceptValidReadingAndPublishToRedisStream(t *testing.T) {
	harness, service, server, scale := newIngestionAPI(t, nil)
	before := databaseRowCounts(t, harness)
	response := sendReading(t, server, scale.APIKey, validReading())
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
	response.Body.Close()

	messages, err := harness.Redis.XRange(harness.Context, repository.ScaleReadingsStream, "-", "+").Result()
	if err != nil {
		t.Fatalf("read scale-readings stream: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("stream messages = %d, want 1", len(messages))
	}
	fields := messages[0].Values
	if fields["event_id"] != validReading().EventID || fields["scale_id"] != "SCALE-001" || fields["plate"] != "ABC1D23" || fields["weight_grams"] != "42850300" || fields["measured_at"] != "2026-07-17T09:00:00.1Z" {
		t.Fatalf("stream fields = %#v, want normalized reading", fields)
	}
	receivedAt, ok := fields["received_at"].(string)
	if !ok {
		t.Fatalf("received_at = %#v, want RFC3339Nano string", fields["received_at"])
	}
	if parsed, err := time.Parse(time.RFC3339Nano, receivedAt); err != nil || parsed.Location() != time.UTC {
		t.Fatalf("received_at = %q, want UTC RFC3339Nano: %v", receivedAt, err)
	}
	if after := databaseRowCounts(t, harness); !sameRowCounts(before, after) {
		t.Fatalf("ingestion changed PostgreSQL rows: before=%v after=%v", before, after)
	}
	_ = service // preserves the service fixture as the explicit no-sync-write subject.
}

func TestGherkinRejectUnauthorizedInvalidAndRedisUnavailableReadings(t *testing.T) {
	harness, service, server, scale := newIngestionAPI(t, nil)
	assertReadingStatus(t, sendReading(t, server, "not-a-device-key", validReading()), http.StatusUnauthorized)
	invalid := validReading()
	invalid.WeightGrams = 0
	assertReadingStatus(t, sendReading(t, server, scale.APIKey, invalid), http.StatusUnprocessableEntity)

	unavailableClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond, MaxRetries: 0})
	t.Cleanup(func() { _ = unavailableClient.Close() })
	unavailable := httptest.NewServer(New(application.New(repository.NewPostgres(harness.DB), repository.NewRedisReadingPublisher(unavailableClient))).Router())
	t.Cleanup(unavailable.Close)
	assertReadingStatus(t, sendReading(t, unavailable, scale.APIKey, validReading()), http.StatusServiceUnavailable)

	length, err := harness.Redis.XLen(harness.Context, repository.ScaleReadingsStream).Result()
	if err != nil {
		t.Fatalf("count stream messages: %v", err)
	}
	if length != 0 {
		t.Fatalf("stream messages = %d, want 0 after rejected readings", length)
	}
	_ = service
}

func TestScaleReadingRejectsMalformedOversizedDisabledAndMismatchedRequests(t *testing.T) {
	harness, service, server, scale := newIngestionAPI(t, nil)

	malformedRequest, err := http.NewRequest(http.MethodPost, server.URL+"/v1/scale-readings", strings.NewReader(`{"event_id":`))
	if err != nil {
		t.Fatal(err)
	}
	malformedRequest.Header.Set("Content-Type", "application/json")
	malformedRequest.Header.Set("Authorization", "Bearer "+scale.APIKey)
	malformedResponse, err := server.Client().Do(malformedRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertReadingStatus(t, malformedResponse, http.StatusBadRequest)

	oversized := `{"event_id":"550e8400-e29b-41d4-a716-446655440000","scale_id":"scale-001","plate":"` + strings.Repeat("A", 65<<10) + `","weight_grams":1,"measured_at":"2026-07-17T12:00:00Z"}`
	overRequest, err := http.NewRequest(http.MethodPost, server.URL+"/v1/scale-readings", strings.NewReader(oversized))
	if err != nil {
		t.Fatal(err)
	}
	overRequest.Header.Set("Content-Type", "application/json")
	overRequest.Header.Set("Authorization", "Bearer "+scale.APIKey)
	overResponse, err := server.Client().Do(overRequest)
	if err != nil {
		t.Fatal(err)
	}
	assertReadingStatus(t, overResponse, http.StatusBadRequest)

	second, err := service.CreateScale(harness.Context, application.ScaleInput{BranchID: scale.BranchID, ScaleID: "scale-002", Name: "Second", APIKey: "second-device-key"})
	if err != nil {
		t.Fatal(err)
	}
	mismatch := validReading()
	mismatch.ScaleID = second.ScaleID
	assertReadingStatus(t, sendReading(t, server, scale.APIKey, mismatch), http.StatusForbidden)
	if _, err := service.DeactivateScale(harness.Context, scale.ID); err != nil {
		t.Fatal(err)
	}
	assertReadingStatus(t, sendReading(t, server, scale.APIKey, validReading()), http.StatusForbidden)

	length, err := harness.Redis.XLen(harness.Context, repository.ScaleReadingsStream).Result()
	if err != nil {
		t.Fatal(err)
	}
	if length != 0 {
		t.Fatalf("stream messages = %d, want 0", length)
	}
}

func newIngestionAPI(t *testing.T, publisher application.ReadingPublisher) (*testkit.Harness, *application.Service, *httptest.Server, scaleFixture) {
	t.Helper()
	harness := testkit.New(t, testkit.WithPostgres(), testkit.WithRedis())
	if err := migrations.Apply(harness.Context, harness.DB); err != nil {
		t.Fatal(err)
	}
	if publisher == nil {
		publisher = repository.NewRedisReadingPublisher(harness.Redis)
	}
	service := application.New(repository.NewPostgres(harness.DB), publisher)
	branch, err := service.CreateBranch(harness.Context, application.BranchInput{Code: "BR-001", Name: "North"})
	if err != nil {
		t.Fatal(err)
	}
	scale, err := service.CreateScale(harness.Context, application.ScaleInput{BranchID: branch.ID, ScaleID: "scale-001", Name: "Inbound", APIKey: "valid-device-key"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(service).Router())
	t.Cleanup(server.Close)
	return harness, service, server, scaleFixture{Scale: scale, APIKey: "valid-device-key"}
}

type scaleFixture struct {
	domain.Scale
	APIKey string
}

func validReading() application.ScaleReadingInput {
	return application.ScaleReadingInput{EventID: "550e8400-e29b-41d4-a716-446655440000", ScaleID: " scale-001 ", Plate: "abc-1d23", WeightGrams: 42_850_300, MeasuredAt: "2026-07-17T12:00:00.100+03:00"}
}

func sendReading(t *testing.T, server *httptest.Server, key string, reading application.ScaleReadingInput) *http.Response {
	t.Helper()
	body, err := json.Marshal(reading)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/scale-readings", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertReadingStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != want {
		var value map[string]any
		_ = json.NewDecoder(response.Body).Decode(&value)
		t.Fatalf("status = %d, want %d body=%v", response.StatusCode, want, value)
	}
	if want >= http.StatusBadRequest {
		var value map[string]string
		if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if value["code"] == "" || value["message"] == "" || value["request_id"] == "" {
			t.Fatalf("error contract = %v, want code, message, request_id", value)
		}
	}
}

func databaseRowCounts(t *testing.T, harness *testkit.Harness) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	for _, table := range []string{"branches", "scales", "trucks", "grain_types", "transport_transactions"} {
		var count int64
		if err := harness.DB.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}

func sameRowCounts(left, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for table, count := range left {
		if right[table] != count {
			return false
		}
	}
	return true
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"scale-challenge/internal/application"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/reports"
	"scale-challenge/internal/repository"
	"scale-challenge/internal/testkit"
)

func TestGherkinReportHTTPInvalidAndEmptyRanges(t *testing.T) {
	h := testkit.New(t, testkit.WithPostgres())
	if err := migrations.Apply(h.Context, h.DB); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(application.New(repository.NewPostgres(h.DB)), WithReports(reports.New(h.DB))).Router())
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/v1/reports/weighings?start=2026-08-02&end=2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid range status = %d, want 422", response.StatusCode)
	}
	response.Body.Close()

	response, err = server.Client().Get(server.URL + "/v1/reports/weighings?start=2027-01-01")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("empty range status = %d, want 200", response.StatusCode)
	}
	var result reports.Result
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.Totals.NetWeightGrams != 0 || result.Totals.CompletedTransportCount != 0 {
		t.Fatalf("empty response = %+v, want zero totals and empty items", result)
	}
}

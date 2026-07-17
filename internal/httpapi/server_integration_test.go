//go:build integration

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"scale-challenge/internal/application"
	"scale-challenge/internal/domain"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/repository"
	"scale-challenge/internal/testkit"
)

func TestGherkinCreateReadUpdateListAndSafelyDeactivateRegistrations(t *testing.T) {
	api := newIntegrationAPI(t)
	branch := create[domain.Branch](t, api, "/v1/branches", application.BranchInput{Code: "br-001", Name: "North"})
	scale := create[domain.Scale](t, api, "/v1/scales", application.ScaleInput{BranchID: branch.ID, ScaleID: "scale-001", Name: "Inbound", APIKey: "never-return-this"})
	truck := create[domain.Truck](t, api, "/v1/trucks", application.TruckInput{Plate: "abc-1d23", TareWeightGrams: 12_000})
	grain := create[domain.GrainType](t, api, "/v1/grain-types", application.GrainTypeInput{Code: "soy", Name: "Soy", PurchasePriceMinor: 125_000, InventoryTargetGrams: 100_000_000, MarginPolicyBPS: 2_000})

	branch = put[domain.Branch](t, api, "/v1/branches/"+branch.ID, application.BranchInput{Code: "br-002", Name: "North updated"})
	scale = put[domain.Scale](t, api, "/v1/scales/"+scale.ID, application.ScaleInput{BranchID: branch.ID, ScaleID: "scale-002", Name: "Inbound updated"})
	truck = put[domain.Truck](t, api, "/v1/trucks/"+truck.ID, application.TruckInput{Plate: "abc1d23", TareWeightGrams: 13_000})
	grain = put[domain.GrainType](t, api, "/v1/grain-types/"+grain.ID, application.GrainTypeInput{Code: "soy", Name: "Soy updated", PurchasePriceMinor: 130_000, InventoryTargetGrams: 110_000_000, MarginPolicyBPS: 1_900})

	assertGetAndList(t, api, "/v1/branches", branch.ID)
	assertGetAndList(t, api, "/v1/scales", scale.ID)
	assertGetAndList(t, api, "/v1/trucks", truck.ID)
	assertGetAndList(t, api, "/v1/grain-types", grain.ID)

	transaction := create[domain.TransportTransaction](t, api, "/v1/transport-transactions", application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID})
	if transaction.Status != "OPEN" || transaction.PurchasePriceMinorSnapshot != grain.PurchasePriceMinor || transaction.MarginPolicyBPSSnapshot != grain.MarginPolicyBPS {
		t.Fatalf("transport snapshot = %+v, want OPEN with grain snapshots", transaction)
	}

	branch = post[domain.Branch](t, api, "/v1/branches/"+branch.ID+"/deactivate", nil, http.StatusOK)
	scale = post[domain.Scale](t, api, "/v1/scales/"+scale.ID+"/deactivate", nil, http.StatusOK)
	truck = post[domain.Truck](t, api, "/v1/trucks/"+truck.ID+"/deactivate", nil, http.StatusOK)
	grain = post[domain.GrainType](t, api, "/v1/grain-types/"+grain.ID+"/deactivate", nil, http.StatusOK)
	if branch.Active || scale.Active || truck.Active || grain.Active {
		t.Fatal("safe deactivation did not mark every registration inactive")
	}
	get[domain.TransportTransaction](t, api, "/v1/transport-transactions/"+transaction.ID, http.StatusOK)
	assertStatus(t, api, http.MethodDelete, "/v1/trucks/"+truck.ID, nil, http.StatusMethodNotAllowed)
}

func TestGherkinRejectInvalidCRUDAndTransportWithoutPartialData(t *testing.T) {
	api := newIntegrationAPI(t)
	branch := create[domain.Branch](t, api, "/v1/branches", application.BranchInput{Code: "br-001", Name: "North"})
	truck := create[domain.Truck](t, api, "/v1/trucks", application.TruckInput{Plate: "ABC1D23", TareWeightGrams: 12_000})
	grain := create[domain.GrainType](t, api, "/v1/grain-types", application.GrainTypeInput{Code: "SOY", Name: "Soy", PurchasePriceMinor: 125_000, InventoryTargetGrams: 100_000_000, MarginPolicyBPS: 2_000})
	_ = create[domain.Scale](t, api, "/v1/scales", application.ScaleInput{BranchID: branch.ID, ScaleID: "scale-001", Name: "Inbound", APIKey: "private-key"})

	assertStatus(t, api, http.MethodPost, "/v1/trucks", application.TruckInput{Plate: "abc-1d23", TareWeightGrams: 10_000}, http.StatusConflict)
	assertStatus(t, api, http.MethodPost, "/v1/trucks", application.TruckInput{Plate: "XYZ9A99", TareWeightGrams: 0}, http.StatusUnprocessableEntity)
	assertStatus(t, api, http.MethodPost, "/v1/transport-transactions", application.TransportInput{BranchID: "missing", TruckID: truck.ID, GrainTypeID: grain.ID}, http.StatusUnprocessableEntity)

	first := create[domain.TransportTransaction](t, api, "/v1/transport-transactions", application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID})
	assertStatus(t, api, http.MethodPost, "/v1/transport-transactions", application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID}, http.StatusConflict)
	transactions := get[[]domain.TransportTransaction](t, api, "/v1/transport-transactions", http.StatusOK)
	if len(transactions) != 1 || transactions[0].ID != first.ID {
		t.Fatalf("transactions = %+v, want exactly the first OPEN transaction", transactions)
	}

	post[domain.Truck](t, api, "/v1/trucks/"+truck.ID+"/deactivate", nil, http.StatusOK)
	assertStatus(t, api, http.MethodPost, "/v1/transport-transactions", application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID}, http.StatusUnprocessableEntity)
}

func TestGherkinOpenTransportAndControlledCancellation(t *testing.T) {
	api := newIntegrationAPI(t)
	branch := create[domain.Branch](t, api, "/v1/branches", application.BranchInput{Code: "br-001", Name: "North"})
	truck := create[domain.Truck](t, api, "/v1/trucks", application.TruckInput{Plate: "ABC1D23", TareWeightGrams: 12_000})
	grain := create[domain.GrainType](t, api, "/v1/grain-types", application.GrainTypeInput{Code: "SOY", Name: "Soy", PurchasePriceMinor: 125_000, InventoryTargetGrams: 100_000_000, MarginPolicyBPS: 2_000})
	transaction := create[domain.TransportTransaction](t, api, "/v1/transport-transactions", application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID})

	cancelled := patch[domain.TransportTransaction](t, api, "/v1/transport-transactions/"+transaction.ID+"/status", map[string]string{"status": "CANCELLED"}, http.StatusOK)
	if cancelled.Status != "CANCELLED" {
		t.Fatalf("status = %s, want CANCELLED", cancelled.Status)
	}
	assertStatus(t, api, http.MethodPatch, "/v1/transport-transactions/"+transaction.ID+"/status", map[string]string{"status": "CANCELLED"}, http.StatusConflict)
	assertStatus(t, api, http.MethodPatch, "/v1/transport-transactions/"+transaction.ID+"/status", map[string]string{"status": "WEIGHED"}, http.StatusUnprocessableEntity)
}

func TestDatabaseConstraintAllowsOnlyOneConcurrentOpenTransactionPerTruck(t *testing.T) {
	api := newIntegrationAPI(t)
	branch := create[domain.Branch](t, api, "/v1/branches", application.BranchInput{Code: "br-001", Name: "North"})
	truck := create[domain.Truck](t, api, "/v1/trucks", application.TruckInput{Plate: "ABC1D23", TareWeightGrams: 12_000})
	grain := create[domain.GrainType](t, api, "/v1/grain-types", application.GrainTypeInput{Code: "SOY", Name: "Soy", PurchasePriceMinor: 125_000, InventoryTargetGrams: 100_000_000, MarginPolicyBPS: 2_000})
	body := application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID}

	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Error(err)
				return
			}
			response, err := api.Client().Post(api.URL+"/v1/transport-transactions/", "application/json", bytes.NewReader(encoded))
			if err != nil {
				t.Error(err)
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		})
	}
	group.Wait()
	close(statuses)
	var created, conflicted int
	for status := range statuses {
		if status == http.StatusCreated {
			created++
		}
		if status == http.StatusConflict {
			conflicted++
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("concurrent statuses created=%d conflict=%d, want 1 each", created, conflicted)
	}
}

func newIntegrationAPI(t *testing.T) *httptest.Server {
	t.Helper()
	harness := testkit.New(t, testkit.WithPostgres())
	if err := migrations.Apply(harness.Context, harness.DB); err != nil {
		t.Fatal(err)
	}
	service := application.New(repository.NewPostgres(harness.DB))
	server := httptest.NewServer(New(service).Router())
	t.Cleanup(server.Close)
	return server
}

func create[T any](t *testing.T, api *httptest.Server, path string, body any) T {
	return post[T](t, api, path, body, http.StatusCreated)
}
func put[T any](t *testing.T, api *httptest.Server, path string, body any) T {
	return request[T](t, api, http.MethodPut, path, body, http.StatusOK)
}
func patch[T any](t *testing.T, api *httptest.Server, path string, body any, status int) T {
	return request[T](t, api, http.MethodPatch, path, body, status)
}
func post[T any](t *testing.T, api *httptest.Server, path string, body any, status int) T {
	return request[T](t, api, http.MethodPost, path, body, status)
}
func get[T any](t *testing.T, api *httptest.Server, path string, status int) T {
	return request[T](t, api, http.MethodGet, path, nil, status)
}
func request[T any](t *testing.T, api *httptest.Server, method, path string, body any, status int) T {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	}
	response, err := api.Client().Do(mustRequest(t, method, api.URL+path, payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		var actual map[string]any
		_ = json.NewDecoder(response.Body).Decode(&actual)
		t.Fatalf("%s %s status=%d want=%d body=%v", method, path, response.StatusCode, status, actual)
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func assertStatus(t *testing.T, api *httptest.Server, method, path string, body any, status int) {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	}
	response, err := api.Client().Do(mustRequest(t, method, api.URL+path, payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("%s %s status=%d want=%d", method, path, response.StatusCode, status)
	}
}
func mustRequest(t *testing.T, method, url string, body *bytes.Reader) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request
}
func assertGetAndList(t *testing.T, api *httptest.Server, collection, id string) {
	t.Helper()
	get[map[string]any](t, api, collection+"/"+id, http.StatusOK)
	list := get[[]map[string]any](t, api, collection+"/", http.StatusOK)
	if len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("%s list=%v, want %s", collection, list, id)
	}
}

func TestScaleAPIKeyIsNeverReturned(t *testing.T) {
	api := newIntegrationAPI(t)
	branch := create[domain.Branch](t, api, "/v1/branches", application.BranchInput{Code: "BR", Name: "Branch"})
	request := application.ScaleInput{BranchID: branch.ID, ScaleID: "SCALE", Name: "Scale", APIKey: "secret-scale-key"}
	assertStatus(t, api, http.MethodPost, "/v1/scales", request, http.StatusCreated)
	response, err := api.Client().Get(api.URL + "/v1/scales/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var raw any
	_ = json.NewDecoder(response.Body).Decode(&raw)
	if strings.Contains(stringify(raw), "secret-scale-key") {
		t.Fatal("scale API key was returned")
	}
}
func stringify(value any) string { encoded, _ := json.Marshal(value); return string(encoded) }

//go:build acceptance

package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"scale-challenge/internal/application"
	"scale-challenge/internal/finalization"
	"scale-challenge/internal/httpapi"
	"scale-challenge/internal/migrations"
	"scale-challenge/internal/repository"
	"scale-challenge/internal/stabilizer"
	"scale-challenge/internal/testkit"
	"scale-challenge/internal/worker"
)

const (
	concurrentScaleCount   = 10
	concurrentSamples      = 32
	concurrentLoadSeed     = "t08-fixed-seed-42"
	concurrentSamplePeriod = 100 * time.Millisecond
)

func TestGherkinConcurrentTenScaleLoadHasNoCrossAssociation(t *testing.T) {
	h := testkit.New(t, testkit.WithPostgres(), testkit.WithRedis())
	if err := migrations.Apply(h.Context, h.DB); err != nil {
		t.Fatal(err)
	}
	service := application.New(repository.NewPostgres(h.DB), repository.NewRedisReadingPublisher(h.Redis))
	server := httptest.NewServer(httpapi.New(service).Router())
	t.Cleanup(server.Close)
	fixtures := seedConcurrentScales(t, h.Context, service)

	start := make(chan struct{})
	errors := make(chan error, concurrentScaleCount)
	var producers sync.WaitGroup
	for _, fixture := range fixtures {
		fixture := fixture
		producers.Go(func() {
			<-start
			errors <- sendFixedSeries(server.Client(), server.URL, fixture)
		})
	}
	close(start)
	producers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	manager, err := stabilizer.New(stabilizer.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	processor, err := worker.NewFinalizingProcessor(manager, worker.NewPostgresLedger(h.DB), finalization.New(h.DB))
	if err != nil {
		t.Fatal(err)
	}
	config := worker.DefaultConfig()
	config.ConsumerName = "t08-concurrent-worker"
	config.BatchSize = 64
	config.BlockTimeout = 20 * time.Millisecond
	instance, err := worker.New(h.Redis, processor, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Ensure(h.Context); err != nil {
		t.Fatal(err)
	}

	deadline, cancel := context.WithTimeout(h.Context, 20*time.Second)
	defer cancel()
	if err := testkit.Eventually(deadline, 10*time.Millisecond, func(ctx context.Context) (bool, error) {
		_, err := instance.ProcessNew(ctx)
		if err != nil && err != redis.Nil {
			return false, err
		}
		var finalized int
		if err := h.DB.QueryRow(ctx, "SELECT count(*) FROM weighings WHERE stage = 'FINAL'").Scan(&finalized); err != nil {
			return false, err
		}
		return finalized == concurrentScaleCount, nil
	}); err != nil {
		t.Fatalf("process deterministic concurrent load: %v", err)
	}

	assertConcurrentIsolation(t, h, fixtures)
}

type concurrentFixture struct{ scaleID, plate, key, session string }

func seedConcurrentScales(t *testing.T, ctx context.Context, service *application.Service) []concurrentFixture {
	t.Helper()
	branch, err := service.CreateBranch(ctx, application.BranchInput{Code: "T08", Name: "Concurrent branch"})
	if err != nil {
		t.Fatal(err)
	}
	grain, err := service.CreateGrainType(ctx, application.GrainTypeInput{Code: "T08-SOY", Name: "Concurrent soy", PurchasePriceMinor: 125_000, InventoryTargetGrams: 100_000_000_000, MarginPolicyBPS: 2_000})
	if err != nil {
		t.Fatal(err)
	}
	fixtures := make([]concurrentFixture, 0, concurrentScaleCount)
	for index := 0; index < concurrentScaleCount; index++ {
		scaleID, key := fmt.Sprintf("T08-SCALE-%02d", index), fmt.Sprintf("t08-key-%02d", index)
		plate := fmt.Sprintf("T%06d", index+1)
		scale, err := service.CreateScale(ctx, application.ScaleInput{BranchID: branch.ID, ScaleID: scaleID, Name: "Concurrent scale", APIKey: key})
		if err != nil {
			t.Fatal(err)
		}
		truck, err := service.CreateTruck(ctx, application.TruckInput{Plate: plate, TareWeightGrams: 12_000_000})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateTransportTransaction(ctx, application.TransportInput{BranchID: branch.ID, TruckID: truck.ID, GrainTypeID: grain.ID}); err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, concurrentFixture{scaleID: scale.ScaleID, plate: truck.Plate, key: key, session: scale.ScaleID + ":" + truck.Plate})
	}
	return fixtures
}

func sendFixedSeries(client *http.Client, baseURL string, fixture concurrentFixture) error {
	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	for sample := 0; sample < concurrentSamples; sample++ {
		weight := int64(2_012_000_000)
		if sample%3 == 1 {
			weight += 20
		}
		if sample%3 == 2 {
			weight -= 20
		}
		payload, err := json.Marshal(application.ScaleReadingInput{EventID: deterministicEventID(fixture, sample), ScaleID: fixture.scaleID, Plate: fixture.plate, WeightGrams: weight, MeasuredAt: base.Add(time.Duration(sample) * concurrentSamplePeriod).Format(time.RFC3339Nano)})
		if err != nil {
			return err
		}
		request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/scale-readings", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+fixture.key)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			return fmt.Errorf("scale %s sample %d status = %d, want 202", fixture.scaleID, sample, response.StatusCode)
		}
	}
	return nil
}

func deterministicEventID(fixture concurrentFixture, sample int) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s:%s:%s:%d", concurrentLoadSeed, fixture.scaleID, fixture.plate, sample))).String()
}

func assertConcurrentIsolation(t *testing.T, h *testkit.Harness, fixtures []concurrentFixture) {
	t.Helper()
	rows, err := h.DB.Query(h.Context, `
		SELECT w.session_id, s.scale_id, tr.plate
		FROM weighings w
		JOIN scales s ON s.id = w.scale_id
		JOIN transport_transactions tx ON tx.id = w.transport_transaction_id
		JOIN trucks tr ON tr.id = tx.truck_id
		WHERE w.stage = 'FINAL'
		ORDER BY w.session_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actual := make(map[string]concurrentFixture)
	for rows.Next() {
		var session, scaleID, plate string
		if err := rows.Scan(&session, &scaleID, &plate); err != nil {
			t.Fatal(err)
		}
		if _, duplicate := actual[session]; duplicate {
			t.Fatalf("more than one final weighing for session %s", session)
		}
		actual[session] = concurrentFixture{scaleID: scaleID, plate: plate, session: session}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(actual) != concurrentScaleCount {
		t.Fatalf("final sessions = %d, want %d", len(actual), concurrentScaleCount)
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].session < fixtures[j].session })
	for _, expected := range fixtures {
		got, found := actual[expected.session]
		if !found || got.scaleID != expected.scaleID || got.plate != expected.plate {
			t.Fatalf("cross-scale or cross-plate association for %s: got %+v, want scale=%s plate=%s", expected.session, got, expected.scaleID, expected.plate)
		}
	}
}

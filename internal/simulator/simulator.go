// Package simulator loads versioned fixtures and produces a deterministic event
// sequence. HTTP transport is intentionally deferred to T03, when the reading
// ingestion contract exists.
package simulator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Scenario is a versioned, deterministic input to the simulator.
type Scenario struct {
	Version     int               `json:"version"`
	Name        string            `json:"name"`
	Seed        int64             `json:"seed"`
	FrequencyMS int               `json:"frequency_ms"`
	Context     ScenarioContext   `json:"context"`
	Readings    []ScenarioReading `json:"readings"`
}

// ScenarioContext holds future registration fixture references without creating
// registration records (which is explicitly reserved for T02).
type ScenarioContext struct {
	BranchCode           string `json:"branch_code"`
	GrainCode            string `json:"grain_code"`
	TruckPlate           string `json:"truck_plate"`
	ScaleID              string `json:"scale_id"`
	PurchasePriceMinor   int64  `json:"purchase_price_minor"`
	InventoryTargetGrams int64  `json:"inventory_target_grams"`
	TransportState       string `json:"transport_state"`
	DeviceKeyEnvironment string `json:"device_key_environment"`
}

// ScenarioReading contains no secret and uses integer grams exclusively.
type ScenarioReading struct {
	ScaleID     string `json:"scale_id"`
	Plate       string `json:"plate"`
	WeightGrams int64  `json:"weight_grams"`
	MeasuredAt  string `json:"measured_at"`
	EventID     string `json:"event_id,omitempty"`
}

// Event is the deterministic output used by a future HTTP transport adapter.
type Event struct {
	EventID     string
	ScaleID     string
	Plate       string
	WeightGrams int64
	MeasuredAt  time.Time
}

// Load decodes and validates a scenario without contacting a service.
func Load(path string) (Scenario, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}

	var scenario Scenario
	if err := json.Unmarshal(contents, &scenario); err != nil {
		return Scenario{}, fmt.Errorf("decode scenario: %w", err)
	}
	if err := Validate(scenario); err != nil {
		return Scenario{}, err
	}
	return scenario, nil
}

// Validate rejects configuration that could make a simulator run ambiguous.
func Validate(s Scenario) error {
	if s.Version != 1 {
		return errors.New("scenario version must be 1")
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.New("scenario name is required")
	}
	if s.Seed < 0 {
		return errors.New("scenario seed must be non-negative")
	}
	if s.FrequencyMS <= 0 {
		return errors.New("scenario frequency_ms must be greater than zero")
	}
	if strings.TrimSpace(s.Context.ScaleID) == "" || strings.TrimSpace(s.Context.TruckPlate) == "" {
		return errors.New("scenario scale_id and truck_plate are required")
	}
	if len(s.Readings) == 0 {
		return errors.New("scenario must contain at least one reading")
	}
	for index, reading := range s.Readings {
		if reading.ScaleID != s.Context.ScaleID {
			return fmt.Errorf("reading %d scale_id does not match scenario context", index)
		}
		if _, err := time.Parse(time.RFC3339Nano, reading.MeasuredAt); err != nil {
			return fmt.Errorf("reading %d measured_at: %w", index, err)
		}
	}
	return nil
}

// Sequence returns a stable order and derives missing event IDs from scenario
// name, the caller's seed, and reading position. It never relies on wall clock.
func Sequence(s Scenario, seed int64) ([]Event, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(s.Readings))
	for index, reading := range s.Readings {
		measuredAt, err := time.Parse(time.RFC3339Nano, reading.MeasuredAt)
		if err != nil {
			return nil, err
		}
		eventID := reading.EventID
		if eventID == "" {
			eventID = deterministicEventID(s.Name, seed, index)
		}
		events = append(events, Event{
			EventID: eventID, ScaleID: reading.ScaleID, Plate: reading.Plate,
			WeightGrams: reading.WeightGrams, MeasuredAt: measuredAt.UTC(),
		})
	}

	sort.SliceStable(events, func(i, j int) bool { return events[i].MeasuredAt.Before(events[j].MeasuredAt) })
	return events, nil
}

func deterministicEventID(name string, seed int64, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", name, seed, index)))
	return hex.EncodeToString(sum[:16])
}

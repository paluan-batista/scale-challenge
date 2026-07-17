package simulator

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestHappyPathSequenceIsDeterministic(t *testing.T) {
	scenario, err := Load(filepath.Join("..", "..", "testdata", "scenarios", "happy-path.json"))
	if err != nil {
		t.Fatal(err)
	}

	first, err := Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Sequence(scenario, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same scenario and seed produced different sequences")
	}
	if len(first) == 0 || first[0].EventID == "" {
		t.Fatal("sequence did not produce a deterministic event ID")
	}
	differentSeed, err := Sequence(scenario, 43)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].EventID == differentSeed[0].EventID {
		t.Fatal("different seed did not change a generated event ID")
	}
}

func TestInvalidScenarioConfigurationFailsBeforeAnyEventIsProduced(t *testing.T) {
	scenario := Scenario{
		Version: 1, Name: "invalid", Seed: 42, FrequencyMS: 0,
		Context:  ScenarioContext{ScaleID: "scale-001", TruckPlate: "ABC1D23"},
		Readings: []ScenarioReading{{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: 1, MeasuredAt: "2026-07-17T00:00:00Z"}},
	}
	if _, err := Sequence(scenario, 42); err == nil {
		t.Fatal("Sequence() error = nil, want invalid configuration error")
	}
}

func TestVersionedScenariosLoadWithTheirFixedSeeds(t *testing.T) {
	for _, name := range []string{"happy-path", "invalid-readings", "unstable", "network-failure", "abandoned"} {
		t.Run(name, func(t *testing.T) {
			scenario, err := Load(filepath.Join("..", "..", "testdata", "scenarios", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Sequence(scenario, scenario.Seed); err != nil {
				t.Fatal(err)
			}
		})
	}
}

package stabilizer

import (
	"errors"
	"testing"
	"time"
)

func TestStabilizationSequences(t *testing.T) {
	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		weights       []int64
		wantState     State
		wantFinal     bool
		wantFinalMass int64
	}{
		{
			name:          "stable noise finalizes with last stable median",
			weights:       noisyWeights(60, 42_850_300),
			wantState:     StateFinalized,
			wantFinal:     true,
			wantFinalMass: 42_850_300,
		},
		{
			name:          "single outlier is rejected by percentile window without changing final mass",
			weights:       withOutlier(noisyWeights(60, 42_850_300), 15, 43_750_000),
			wantState:     StateFinalized,
			wantFinal:     true,
			wantFinalMass: 42_850_300,
		},
		{
			name:      "increasing weight is not stable",
			weights:   rampWeights(60, 42_850_000, 200),
			wantState: StateCollecting,
		},
		{
			name:      "decreasing weight is not stable",
			weights:   rampWeights(60, 42_862_000, -200),
			wantState: StateCollecting,
		},
		{
			name:      "insufficient samples remain collecting",
			weights:   noisyWeights(29, 42_850_300),
			wantState: StateCollecting,
		},
		{
			name:      "lost stability resets candidate to collecting",
			weights:   append(withOutlier(noisyWeights(30, 42_850_300), 15, 43_750_000), 43_000_000),
			wantState: StateCollecting,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newTestManager(t)
			var result Result
			var finalized *Result
			for index, weight := range test.weights {
				var err error
				result, err = manager.Add(Reading{ScaleID: " scale-001 ", Plate: "abc-1d23", WeightGrams: weight, MeasuredAt: base.Add(time.Duration(index) * 200 * time.Millisecond)})
				if err != nil {
					t.Fatalf("Add(%d): %v", index, err)
				}
				if result.Finalization != nil {
					captured := result
					finalized = &captured
				}
			}
			if result.State != test.wantState {
				t.Fatalf("state = %s, want %s", result.State, test.wantState)
			}
			if (finalized != nil) != test.wantFinal {
				t.Fatalf("finalization = %#v, want present=%t", finalized, test.wantFinal)
			}
			if test.wantFinal {
				if finalized.Finalization.WeightGrams != test.wantFinalMass {
					t.Fatalf("final weight = %d, want %d", finalized.Finalization.WeightGrams, test.wantFinalMass)
				}
				if finalized.Finalization.WeightGrams != finalized.Window.MedianGrams {
					t.Fatalf("final weight = %d, window median = %d", finalized.Finalization.WeightGrams, finalized.Window.MedianGrams)
				}
				if finalized.Finalization.SampleCount < 30 || finalized.Finalization.DispersionGrams < 0 || finalized.Finalization.Slope.Denominator.Sign() <= 0 || finalized.Finalization.AlgorithmVersion == "" {
					t.Fatalf("incomplete finalization audit data: %#v", finalized.Finalization)
				}
			}
		})
	}
}

func TestT04GherkinScenarios(t *testing.T) {
	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		run  func(*testing.T, *Manager)
	}{
		{
			name: "Finalize after two stable windows",
			run: func(t *testing.T, manager *Manager) {
				var result Result
				var finalized *Result
				for index, weight := range noisyWeights(60, 42_850_300) {
					var err error
					result, err = manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: weight, MeasuredAt: base.Add(time.Duration(index) * 200 * time.Millisecond)})
					if err != nil {
						t.Fatal(err)
					}
					if result.Finalization != nil {
						captured := result
						finalized = &captured
					}
				}
				if result.State != StateFinalized || finalized == nil {
					t.Fatalf("result = %#v, want FINALIZED with audit data", result)
				}
				if finalized.Finalization.WeightGrams != finalized.Window.MedianGrams {
					t.Fatalf("final = %d, median = %d", finalized.Finalization.WeightGrams, finalized.Window.MedianGrams)
				}
			},
		},
		{
			name: "Do not finalize an oscillating or out-of-order sequence",
			run: func(t *testing.T, manager *Manager) {
				for index, weight := range rampWeights(31, 42_850_000, 500) {
					result, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: weight, MeasuredAt: base.Add(time.Duration(index) * 200 * time.Millisecond)})
					if err != nil {
						t.Fatal(err)
					}
					if result.Finalization != nil || result.State == StateFinalized {
						t.Fatalf("oscillating sequence finalized: %#v", result)
					}
				}
				_, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: 42_850_000, MeasuredAt: base})
				var ordered *OutOfOrderTimestampError
				if !errors.As(err, &ordered) {
					t.Fatalf("error = %v, want OutOfOrderTimestampError", err)
				}
				if manager.State(Key{ScaleID: "scale-001", Plate: "abc-1d23"}) != StateFailed {
					t.Fatal("out-of-order session must transition to FAILED")
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { test.run(t, newTestManager(t)) })
	}
}

func TestHysteresisAndRelease(t *testing.T) {
	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	manager := newTestManager(t)
	finalize(t, manager, base)

	heavy, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: 42_850_300, MeasuredAt: base.Add(20 * time.Second)})
	if err != nil || heavy.State != StateFinalized || heavy.Finalization != nil {
		t.Fatalf("heavy post-final reading = %#v, %v; want retained FINALIZED without duplicate", heavy, err)
	}
	low, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: 10_000, MeasuredAt: base.Add(21 * time.Second)})
	if err != nil || low.State != StateEmpty {
		t.Fatalf("low-weight release = %#v, %v; want EMPTY", low, err)
	}

	finalize(t, manager, base.Add(40*time.Second))
	changedPlate, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "XYZ9A99", WeightGrams: 42_850_300, MeasuredAt: base.Add(60 * time.Second)})
	if err != nil || changedPlate.State != StateCollecting {
		t.Fatalf("plate change = %#v, %v; want new COLLECTING session", changedPlate, err)
	}
	if got := manager.State(Key{ScaleID: "scale-001", Plate: "ABC1D23"}); got != StateEmpty {
		t.Fatalf("replaced plate state = %s, want EMPTY", got)
	}

	changes := manager.Expire(base.Add(2*time.Minute + time.Second))
	if len(changes) != 1 || changes[0].State != StateAbandoned {
		t.Fatalf("timeout changes = %#v, want one ABANDONED session", changes)
	}
}

func TestBoundedRingAndSessionCount(t *testing.T) {
	config := DefaultConfig()
	config.WindowCapacity = 32
	config.MaxSessions = 2
	manager, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 40; index++ {
		_, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: 42_850_300 + int64(index)*100, MeasuredAt: base.Add(time.Duration(index) * 200 * time.Millisecond)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := manager.sessions[Key{ScaleID: "SCALE-001", Plate: "ABC1D23"}].readings.count; got != 32 {
		t.Fatalf("ring count = %d, want 32", got)
	}
	for _, plate := range []string{"BBB1B11", "CCC1C11"} {
		_, err := manager.Add(Reading{ScaleID: "scale-002", Plate: plate, WeightGrams: 1, MeasuredAt: base.Add(20 * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(manager.sessions) != 2 {
		t.Fatalf("active sessions = %d, want bounded maximum 2", len(manager.sessions))
	}
}

func TestAbsolutePercentageAndSlopeTolerances(t *testing.T) {
	base := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		config  func(*Config)
		weights []int64
	}{
		{
			name: "absolute dispersion limit",
			config: func(config *Config) {
				config.AbsoluteToleranceGrams = 100
			},
			weights: alternatingWeights(31, 42_850_300, 200),
		},
		{
			name: "percentage dispersion limit",
			config: func(config *Config) {
				config.AbsoluteToleranceGrams = 10_000
				config.PercentToleranceBPS = 0
			},
			weights: alternatingWeights(31, 42_850_300, 20),
		},
		{
			name: "weight time slope limit",
			config: func(config *Config) {
				config.AbsoluteToleranceGrams = 500
				config.PercentToleranceBPS = 100
				config.MaxSlopeGramsPerSecond = 25
			},
			weights: rampWeights(31, 42_850_300, 10),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig()
			config.WindowCapacity = 60
			config.InactivityTimeout = time.Minute
			test.config(&config)
			manager, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			var result Result
			for index, weight := range test.weights {
				result, err = manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: weight, MeasuredAt: base.Add(time.Duration(index) * 200 * time.Millisecond)})
				if err != nil {
					t.Fatal(err)
				}
			}
			if result.State != StateCollecting || result.Window == nil || result.Window.Stable {
				t.Fatalf("result = %#v, want unstable COLLECTING window", result)
			}
		})
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	config := DefaultConfig()
	config.WindowCapacity = 60
	config.AbsoluteToleranceGrams = 500
	config.PercentToleranceBPS = 100
	config.MaxSlopeGramsPerSecond = 250
	config.InactivityTimeout = time.Minute
	manager, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func finalize(t *testing.T, manager *Manager, base time.Time) {
	t.Helper()
	for index, weight := range noisyWeights(31, 42_850_300) {
		result, err := manager.Add(Reading{ScaleID: "scale-001", Plate: "ABC1D23", WeightGrams: weight, MeasuredAt: base.Add(time.Duration(index) * 200 * time.Millisecond)})
		if err != nil {
			t.Fatal(err)
		}
		if result.Finalization != nil {
			return
		}
	}
	t.Fatal("session did not finalize")
}

func noisyWeights(count int, center int64) []int64 {
	noise := []int64{0, 20, -20, 10, -10, 30, -30, 5, -5, 0}
	weights := make([]int64, count)
	for index := range weights {
		weights[index] = center + noise[index%len(noise)]
	}
	return weights
}

func rampWeights(count int, start, delta int64) []int64 {
	weights := make([]int64, count)
	for index := range weights {
		weights[index] = start + int64(index)*delta
	}
	return weights
}

func alternatingWeights(count int, center, offset int64) []int64 {
	weights := make([]int64, count)
	for index := range weights {
		weights[index] = center - offset
		if index%2 == 1 {
			weights[index] = center + offset
		}
	}
	return weights
}

func withOutlier(weights []int64, index int, value int64) []int64 {
	copy := append([]int64(nil), weights...)
	copy[index] = value
	return copy
}

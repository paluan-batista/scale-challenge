// Package stabilizer deterministically turns ordered scale readings into one
// finalized weight per released passage. It has no transport, Redis, or
// PostgreSQL dependency so a worker can use it without making the algorithm
// dependent on infrastructure.
package stabilizer

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"scale-challenge/internal/domain"
)

const defaultAlgorithmVersion = "t04-v1"

var ErrInvalidReading = errors.New("invalid scale reading")

// State is the lifecycle of one scale/plate passage.
type State string

const (
	StateEmpty      State = "EMPTY"
	StateCollecting State = "COLLECTING"
	StateCandidate  State = "CANDIDATE"
	StateFinalized  State = "FINALIZED"
	StateAbandoned  State = "ABANDONED"
	StateFailed     State = "FAILED"
)

// Key identifies a passage. Plate is always normalized before it is used.
type Key struct {
	ScaleID string
	Plate   string
}

// Reading is deliberately a small domain-independent input to the algorithm.
type Reading struct {
	ScaleID     string
	Plate       string
	WeightGrams int64
	MeasuredAt  time.Time
}

// Config constrains both the stabilization decision and retained memory.
// PercentToleranceBPS is basis points (100 = 1%).
type Config struct {
	WindowCapacity          int
	MaxSessions             int
	MinSamples              int
	MinWindowDuration       time.Duration
	AbsoluteToleranceGrams  int64
	PercentToleranceBPS     int64
	MaxSlopeGramsPerSecond  int64
	LowWeightThresholdGrams int64
	InactivityTimeout       time.Duration
	AlgorithmVersion        string
}

func DefaultConfig() Config {
	return Config{
		WindowCapacity:          64,
		MaxSessions:             1_024,
		MinSamples:              30,
		MinWindowDuration:       3 * time.Second,
		AbsoluteToleranceGrams:  250,
		PercentToleranceBPS:     10,
		MaxSlopeGramsPerSecond:  250,
		LowWeightThresholdGrams: 10_000,
		InactivityTimeout:       30 * time.Second,
		AlgorithmVersion:        defaultAlgorithmVersion,
	}
}

func (c Config) validate() error {
	if c.WindowCapacity < 30 || c.MinSamples < 30 || c.WindowCapacity < c.MinSamples {
		return fmt.Errorf("window capacity and minimum samples must be at least 30: %w", ErrInvalidReading)
	}
	if c.MinWindowDuration < 3*time.Second {
		return fmt.Errorf("minimum window duration must be at least 3 seconds: %w", ErrInvalidReading)
	}
	if c.MaxSessions < 1 || c.AbsoluteToleranceGrams < 0 || c.PercentToleranceBPS < 0 || c.MaxSlopeGramsPerSecond < 0 || c.LowWeightThresholdGrams < 0 || c.InactivityTimeout <= 0 {
		return fmt.Errorf("invalid stabilizer configuration: %w", ErrInvalidReading)
	}
	return nil
}

// OutOfOrderTimestampError is returned when a reading is not strictly newer
// than the previous reading in the same scale/plate session.
type OutOfOrderTimestampError struct {
	Key      Key
	Previous time.Time
	Current  time.Time
}

func (e *OutOfOrderTimestampError) Error() string {
	return fmt.Sprintf("out-of-order timestamp for scale %q plate %q: %s is not after %s", e.Key.ScaleID, e.Key.Plate, e.Current.UTC().Format(time.RFC3339Nano), e.Previous.UTC().Format(time.RFC3339Nano))
}

// FailedSessionError prevents silently reusing a session after a data-ordering
// failure. It is released through the regular timeout or plate-change paths.
type FailedSessionError struct{ Key Key }

func (e *FailedSessionError) Error() string {
	return fmt.Sprintf("session for scale %q plate %q is failed", e.Key.ScaleID, e.Key.Plate)
}

// Rational represents the signed exact slope in grams per second. The values
// are reduced and copied on construction; it never relies on floating point.
type Rational struct {
	Numerator   *big.Int
	Denominator *big.Int
}

func newRational(numerator, denominator *big.Int) Rational {
	n := new(big.Int).Set(numerator)
	d := new(big.Int).Set(denominator)
	if d.Sign() < 0 {
		n.Neg(n)
		d.Neg(d)
	}
	if n.Sign() == 0 {
		return Rational{Numerator: big.NewInt(0), Denominator: big.NewInt(1)}
	}
	gcd := new(big.Int).GCD(nil, nil, new(big.Int).Abs(n), d)
	return Rational{Numerator: n.Quo(n, gcd), Denominator: d.Quo(d, gcd)}
}

func (r Rational) String() string {
	return r.Numerator.String() + "/" + r.Denominator.String()
}

func (r Rational) absLessOrEqual(limit int64) bool {
	left := new(big.Int).Abs(new(big.Int).Set(r.Numerator))
	right := new(big.Int).Mul(r.Denominator, big.NewInt(limit))
	return left.Cmp(right) <= 0
}

// WindowStats contains the values needed later for final-weighing audit data.
type WindowStats struct {
	SampleCount     int
	Duration        time.Duration
	MedianGrams     int64
	P05Grams        int64
	P95Grams        int64
	DispersionGrams int64
	Slope           Rational
	Stable          bool
}

// Finalization is emitted exactly once until its session is released.
type Finalization struct {
	WeightGrams      int64
	AlgorithmVersion string
	SampleCount      int
	DispersionGrams  int64
	Slope            Rational
	StabilizedAt     time.Time
}

type Result struct {
	Key          Key
	State        State
	Window       *WindowStats
	Finalization *Finalization
}

// Manager owns all currently active in-memory passages. It is intentionally
// single-threaded; a future worker owns the synchronization policy.
type Manager struct {
	config   Config
	sessions map[Key]*session
}

type session struct {
	state    State
	readings ring
	lastSeen time.Time
}

// New validates the invariant-bearing configuration before accepting readings.
func New(config Config) (*Manager, error) {
	if config.AlgorithmVersion == "" {
		config.AlgorithmVersion = defaultAlgorithmVersion
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Manager{config: config, sessions: make(map[Key]*session)}, nil
}

func (m *Manager) State(key Key) State {
	key = normalizeKey(key)
	if current, ok := m.sessions[key]; ok {
		return current.state
	}
	return StateEmpty
}

// Add processes exactly one reading. A finalized session intentionally ignores
// further heavy readings, which provides the one-finalization hysteresis.
func (m *Manager) Add(reading Reading) (Result, error) {
	key, err := readingKey(reading)
	if err != nil {
		return Result{}, err
	}
	m.releaseFinalizedForPlateChange(key)

	current := m.sessions[key]
	if current != nil && reading.MeasuredAt.Sub(current.lastSeen) > m.config.InactivityTimeout {
		current.state = StateAbandoned
		delete(m.sessions, key)
		current = nil
	}
	if current == nil {
		m.makeRoom()
		current = &session{state: StateEmpty, readings: newRing(m.config.WindowCapacity)}
		m.sessions[key] = current
	}
	if current.state == StateFailed {
		return Result{Key: key, State: StateFailed}, &FailedSessionError{Key: key}
	}
	if !current.lastSeen.IsZero() && !reading.MeasuredAt.After(current.lastSeen) {
		current.state = StateFailed
		return Result{Key: key, State: StateFailed}, &OutOfOrderTimestampError{Key: key, Previous: current.lastSeen, Current: reading.MeasuredAt}
	}
	if current.state == StateFinalized {
		if reading.WeightGrams <= m.config.LowWeightThresholdGrams {
			delete(m.sessions, key)
			return Result{Key: key, State: StateEmpty}, nil
		}
		current.lastSeen = reading.MeasuredAt
		return Result{Key: key, State: StateFinalized}, nil
	}

	previousState := current.state
	current.readings.add(reading)
	current.lastSeen = reading.MeasuredAt
	stats, ready := m.evaluate(current.readings.values())
	if !ready {
		current.state = StateCollecting
		return Result{Key: key, State: current.state}, nil
	}
	return m.applyWindow(key, current, previousState, stats), nil
}

func (m *Manager) applyWindow(key Key, current *session, previous State, stats WindowStats) Result {
	if stats.Stable {
		if previous == StateCandidate {
			current.state = StateFinalized
			finalization := &Finalization{
				WeightGrams:      stats.MedianGrams,
				AlgorithmVersion: m.config.AlgorithmVersion,
				SampleCount:      stats.SampleCount,
				DispersionGrams:  stats.DispersionGrams,
				Slope:            stats.Slope,
				StabilizedAt:     current.lastSeen.UTC(),
			}
			return Result{Key: key, State: StateFinalized, Window: &stats, Finalization: finalization}
		}
		current.state = StateCandidate
		return Result{Key: key, State: StateCandidate, Window: &stats}
	}
	current.state = StateCollecting
	return Result{Key: key, State: StateCollecting, Window: &stats}
}

// Expire abandons incomplete passages and releases finalized ones after their
// timeout. The returned states make both effects observable without retaining
// unbounded completed sessions.
func (m *Manager) Expire(now time.Time) []Result {
	now = now.UTC()
	var results []Result
	for key, current := range m.sessions {
		if now.Sub(current.lastSeen) <= m.config.InactivityTimeout {
			continue
		}
		state := StateAbandoned
		if current.state == StateFinalized {
			state = StateEmpty
		}
		delete(m.sessions, key)
		results = append(results, Result{Key: key, State: state})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Key.ScaleID == results[j].Key.ScaleID {
			return results[i].Key.Plate < results[j].Key.Plate
		}
		return results[i].Key.ScaleID < results[j].Key.ScaleID
	})
	return results
}

func (m *Manager) releaseFinalizedForPlateChange(key Key) {
	for existingKey, current := range m.sessions {
		if existingKey.ScaleID == key.ScaleID && existingKey.Plate != key.Plate && current.state == StateFinalized {
			delete(m.sessions, existingKey)
		}
	}
}

func (m *Manager) makeRoom() {
	if len(m.sessions) < m.config.MaxSessions {
		return
	}
	var oldestKey Key
	var oldest time.Time
	for key, current := range m.sessions {
		if oldest.IsZero() || current.lastSeen.Before(oldest) {
			oldestKey, oldest = key, current.lastSeen
		}
	}
	delete(m.sessions, oldestKey)
}

func (m *Manager) evaluate(readings []Reading) (WindowStats, bool) {
	if len(readings) < m.config.MinSamples {
		return WindowStats{}, false
	}
	duration := readings[len(readings)-1].MeasuredAt.Sub(readings[0].MeasuredAt)
	if duration < m.config.MinWindowDuration {
		return WindowStats{}, false
	}
	weights := make([]int64, len(readings))
	for index, reading := range readings {
		weights[index] = reading.WeightGrams
	}
	sort.Slice(weights, func(i, j int) bool { return weights[i] < weights[j] })
	median := weights[len(weights)/2]
	if len(weights)%2 == 0 {
		median = weights[len(weights)/2-1] + (weights[len(weights)/2]-weights[len(weights)/2-1])/2
	}
	p05 := weights[percentileIndex(len(weights), 5)]
	p95 := weights[percentileIndex(len(weights), 95)]
	dispersion := p95 - p05
	slope := theilSenSlope(readings)
	stable := dispersion <= m.config.AbsoluteToleranceGrams && percentWithin(dispersion, median, m.config.PercentToleranceBPS) && slope.absLessOrEqual(m.config.MaxSlopeGramsPerSecond)
	return WindowStats{SampleCount: len(readings), Duration: duration, MedianGrams: median, P05Grams: p05, P95Grams: p95, DispersionGrams: dispersion, Slope: slope, Stable: stable}, true
}

func percentileIndex(length, percentile int) int {
	return (percentile*(length-1) + 99) / 100 // nearest-rank ceiling, integer-only
}

func percentWithin(dispersion, median, basisPoints int64) bool {
	left := new(big.Int).Mul(big.NewInt(dispersion), big.NewInt(10_000))
	right := new(big.Int).Mul(big.NewInt(median), big.NewInt(basisPoints))
	return left.Cmp(right) <= 0
}

// theilSenSlope is the median of all pairwise weight/time slopes. It is exact
// rational arithmetic and, unlike ordinary regression, does not let one bad
// sample override the percentile-based spread check.
func theilSenSlope(readings []Reading) Rational {
	slopes := make([]Rational, 0, len(readings)*(len(readings)-1)/2)
	for left := 0; left < len(readings)-1; left++ {
		for right := left + 1; right < len(readings); right++ {
			numerator := big.NewInt(readings[right].WeightGrams - readings[left].WeightGrams)
			numerator.Mul(numerator, big.NewInt(int64(time.Second)))
			denominator := big.NewInt(readings[right].MeasuredAt.Sub(readings[left].MeasuredAt).Nanoseconds())
			slopes = append(slopes, newRational(numerator, denominator))
		}
	}
	sort.Slice(slopes, func(i, j int) bool {
		left := new(big.Int).Mul(slopes[i].Numerator, slopes[j].Denominator)
		right := new(big.Int).Mul(slopes[j].Numerator, slopes[i].Denominator)
		return left.Cmp(right) < 0
	})
	middle := len(slopes) / 2
	if len(slopes)%2 == 1 {
		return slopes[middle]
	}
	left, right := slopes[middle-1], slopes[middle]
	numerator := new(big.Int).Add(
		new(big.Int).Mul(left.Numerator, right.Denominator),
		new(big.Int).Mul(right.Numerator, left.Denominator),
	)
	denominator := new(big.Int).Mul(left.Denominator, right.Denominator)
	denominator.Mul(denominator, big.NewInt(2))
	return newRational(numerator, denominator)
}

func readingKey(reading Reading) (Key, error) {
	key := normalizeKey(Key{ScaleID: reading.ScaleID, Plate: reading.Plate})
	if key.ScaleID == "" || key.Plate == "" || reading.WeightGrams <= 0 || reading.MeasuredAt.IsZero() {
		return Key{}, fmt.Errorf("scale_id, plate, positive weight, and measured_at are required: %w", ErrInvalidReading)
	}
	return key, nil
}

func normalizeKey(key Key) Key {
	return Key{ScaleID: strings.ToUpper(strings.TrimSpace(key.ScaleID)), Plate: domain.NormalizePlate(key.Plate)}
}

type ring struct {
	elements []Reading
	start    int
	count    int
}

func newRing(capacity int) ring { return ring{elements: make([]Reading, capacity)} }

func (r *ring) add(value Reading) {
	index := (r.start + r.count) % len(r.elements)
	if r.count == len(r.elements) {
		index = r.start
		r.start = (r.start + 1) % len(r.elements)
	} else {
		r.count++
	}
	r.elements[index] = value
}

func (r ring) values() []Reading {
	result := make([]Reading, r.count)
	for index := range result {
		result[index] = r.elements[(r.start+index)%len(r.elements)]
	}
	return result
}

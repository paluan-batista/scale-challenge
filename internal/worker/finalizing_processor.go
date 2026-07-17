package worker

import (
	"context"
	"errors"
	"sync"

	"scale-challenge/internal/domain"
	"scale-challenge/internal/finalization"
	"scale-challenge/internal/observability"
	"scale-challenge/internal/stabilizer"
)

const finalWeighingStage = "FINAL"

// FinalizingProcessor joins the T04 deterministic stabilizer with the T06
// transaction. It serializes a worker instance's session state and keeps the
// exact event work in memory until its durable write succeeds, so a transient
// PostgreSQL error cannot make a redelivery appear out of order.
type FinalizingProcessor struct {
	manager   *stabilizer.Manager
	ledger    Processor
	finalizer *finalization.Service
	counter   observability.Counter

	mu      sync.Mutex
	pending map[string]pendingWork
}

type pendingWork struct {
	event Event
	input *finalization.Input
}

func NewFinalizingProcessor(manager *stabilizer.Manager, ledger Processor, finalizer *finalization.Service, counters ...observability.Counter) (*FinalizingProcessor, error) {
	if manager == nil || ledger == nil || finalizer == nil {
		return nil, errors.New("stabilizer manager, ledger, and finalizer are required")
	}
	var counter observability.Counter
	if len(counters) > 0 {
		counter = counters[0]
	}
	return &FinalizingProcessor{manager: manager, ledger: ledger, finalizer: finalizer, counter: counter, pending: make(map[string]pendingWork)}, nil
}

func (p *FinalizingProcessor) Process(ctx context.Context, event Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	work, exists := p.pending[event.EventID]
	if !exists {
		result, err := p.manager.Add(stabilizer.Reading{ScaleID: event.ScaleID, Plate: event.Plate, WeightGrams: event.WeightGrams, MeasuredAt: event.MeasuredAt})
		if err != nil {
			return permanent(err)
		}
		work = pendingWork{event: event}
		if result.Finalization != nil {
			work.input = &finalization.Input{
				SessionID:        result.Key.ScaleID + ":" + result.Key.Plate,
				Stage:            finalWeighingStage,
				EventID:          event.EventID,
				ScaleID:          event.ScaleID,
				Plate:            event.Plate,
				GrossWeightGrams: result.Finalization.WeightGrams,
				AlgorithmVersion: result.Finalization.AlgorithmVersion,
				SampleCount:      result.Finalization.SampleCount,
				DispersionGrams:  result.Finalization.DispersionGrams,
				Slope:            result.Finalization.Slope.String(),
				WeighedAt:        result.Finalization.StabilizedAt,
			}
		}
		p.pending[event.EventID] = work
	}

	if work.input == nil {
		if err := p.ledger.Process(ctx, work.event); err != nil {
			return err
		}
		delete(p.pending, event.EventID)
		return nil
	}
	if _, err := p.finalizer.Finalize(ctx, *work.input); err != nil {
		if isBusinessError(err) {
			delete(p.pending, event.EventID)
			return permanent(err)
		}
		return err
	}
	if p.counter != nil {
		p.counter.Inc(ctx, observability.StabilizationFinalized)
	}
	delete(p.pending, event.EventID)
	return nil
}

func permanent(err error) error {
	if _, ok := err.(*PermanentError); ok {
		return err
	}
	return &PermanentError{Err: err}
}

func isBusinessError(err error) bool {
	return errors.Is(err, domain.ErrValidation) || errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict)
}

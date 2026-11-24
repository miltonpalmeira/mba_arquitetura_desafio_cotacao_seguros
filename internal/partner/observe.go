package partner

import (
	"context"
	"log"
	"sync"
	"sync/atomic"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Instrument names are the ones the SAD and the process README repeat.
const (
	metricBreakerState       = "partner.breaker.state"
	metricBreakerTransitions = "partner.breaker.transitions"
	eventShortCircuit        = "circuit.short_circuit"
)

const (
	attrPartner      = "partner"
	attrFromState    = "from_state"
	attrToState      = "to_state"
	attrBreakerState = "breaker.state"
)

var (
	instrumentsOnce sync.Once
	transitions     metric.Int64Counter
	stateGauge      metric.Int64ObservableGauge
	breakerStates   sync.Map // partner name -> *atomic.Int64
)

func ensureBreakerInstruments() {
	instrumentsOnce.Do(func() {
		meter := otel.Meter("github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner")
		var err error
		transitions, err = meter.Int64Counter(
			metricBreakerTransitions,
			metric.WithDescription("Breaker transitions for one partner. from_state and to_state are closed, half-open or open."),
		)
		if err != nil {
			log.Printf("quotation-api: breaker transition counter: %v", err)
		}
		stateGauge, err = meter.Int64ObservableGauge(
			metricBreakerState,
			metric.WithDescription("Breaker state for one partner: 0 closed, 1 half-open, 2 open."),
		)
		if err != nil {
			log.Printf("quotation-api: breaker state gauge: %v", err)
			return
		}
		_, err = meter.RegisterCallback(observeBreakerStates, stateGauge)
		if err != nil {
			log.Printf("quotation-api: breaker state callback: %v", err)
		}
	})
}

func observeBreakerStates(_ context.Context, observer metric.Observer) error {
	if stateGauge == nil {
		return nil
	}
	breakerStates.Range(func(key, value any) bool {
		observer.ObserveInt64(stateGauge, value.(*atomic.Int64).Load(), metric.WithAttributes(
			attribute.String(attrPartner, key.(string)),
		))
		return true
	})
	return nil
}

func stateSlot(name string) *atomic.Int64 {
	if existing, ok := breakerStates.Load(name); ok {
		return existing.(*atomic.Int64)
	}
	slot := &atomic.Int64{}
	actual, _ := breakerStates.LoadOrStore(name, slot)
	return actual.(*atomic.Int64)
}

// noteBreakerCreated publishes the closed state so the gauge exists before the first trip.
func noteBreakerCreated(name string) {
	ensureBreakerInstruments()
	stateSlot(name).Store(int64(gobreaker.StateClosed))
}

// noteBreakerState records the transition and the current state.
// The gauge values are the gobreaker constants: 0 closed, 1 half-open, 2 open.
// Half-open lasts one probe, shorter than the 10s metric export, so the counter is what keeps that step.
func noteBreakerState(name string, from, to gobreaker.State) {
	ensureBreakerInstruments()
	stateSlot(name).Store(int64(to))
	if transitions == nil {
		return
	}
	transitions.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String(attrPartner, name),
		attribute.String(attrFromState, from.String()),
		attribute.String(attrToState, to.String()),
	))
}

// markShortCircuit stamps the current span when the partner is not called.
// Attributes are the partner name and the breaker state. No document, plate or quote id.
func markShortCircuit(ctx context.Context, partnerName, state string) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(eventShortCircuit, trace.WithAttributes(
		attribute.String(attrPartner, partnerName),
		attribute.String(attrBreakerState, state),
	))
}

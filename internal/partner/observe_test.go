package partner

import (
	"context"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

var metricReader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	metricReader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader)))
	os.Exit(m.Run())
}

func TestBreakerGaugeAndTransitionsDrawTheThreeStates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fail := atomic.Bool{}
		fail.Store(true)
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if fail.Load() {
				return partnerResponse(r, http.StatusServiceUnavailable, `{"error":"partner unavailable"}`), nil
			}
			return partnerResponse(r, http.StatusOK, quoteBody), nil
		})

		settings := platform.DefaultResilience()
		client := newClient(settings, transport)
		target := platform.Partner{Name: "partner-observe", BaseURL: "http://partner-observe"}

		if client.StateName(target.Name) != "closed" {
			t.Fatalf("initial state %q, want closed", client.StateName(target.Name))
		}
		beforeOpen := transitionCount(target.Name, "closed", "open")
		beforeHalf := transitionCount(target.Name, "open", "half-open")
		beforeClose := transitionCount(target.Name, "half-open", "closed")
		if gaugeValue(target.Name) != 0 {
			t.Fatalf("gauge %d at creation, want 0 closed", gaugeValue(target.Name))
		}

		for range settings.ConsecutiveFailures {
			if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
				t.Fatal("failure was treated as success")
			}
		}
		if gaugeValue(target.Name) != 2 {
			t.Fatalf("gauge %d after five failures, want 2 open", gaugeValue(target.Name))
		}
		if got := transitionCount(target.Name, "closed", "open"); got != beforeOpen+1 {
			t.Fatalf("closed→open transitions %d, want %d", got, beforeOpen+1)
		}

		time.Sleep(settings.OpenFor + time.Nanosecond)
		if client.StateName(target.Name) != "half-open" {
			t.Fatalf("state %q, want half-open", client.StateName(target.Name))
		}
		if gaugeValue(target.Name) != 1 {
			t.Fatalf("gauge %d in the open window, want 1 half-open", gaugeValue(target.Name))
		}
		if got := transitionCount(target.Name, "open", "half-open"); got != beforeHalf+1 {
			t.Fatalf("open→half-open transitions %d, want %d", got, beforeHalf+1)
		}

		fail.Store(false)
		if _, err := client.Quote(context.Background(), target, map[string]string{}); err != nil {
			t.Fatalf("probe: %v", err)
		}
		if gaugeValue(target.Name) != 0 {
			t.Fatalf("gauge %d after the probe, want 0 closed", gaugeValue(target.Name))
		}
		if got := transitionCount(target.Name, "half-open", "closed"); got != beforeClose+1 {
			t.Fatalf("half-open→closed transitions %d, want %d", got, beforeClose+1)
		}
		assertMetricAttributesSafe(t)
	})
}

func TestOpenCircuitMarksTheTraceWithoutCallingThePartner(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return partnerResponse(r, http.StatusServiceUnavailable, `{"error":"partner unavailable"}`), nil
	})
	client := newClient(platform.DefaultResilience(), transport)
	target := platform.Partner{Name: "partner-mark", BaseURL: "http://partner-mark"}

	for range platform.DefaultResilience().ConsecutiveFailures {
		if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
			t.Fatal("failure was treated as success")
		}
	}
	before := calls.Load()

	exporter := tracetest.NewInMemoryExporter()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter)).Tracer("test")
	ctx, span := tracer.Start(context.Background(), "POST /quotes")
	_, err := client.Quote(ctx, target, map[string]string{})
	span.End()
	if err == nil {
		t.Fatal("open circuit returned a quote")
	}
	if calls.Load() != before {
		t.Fatalf("short circuit called the partner %d extra times", calls.Load()-before)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("%d spans, want the server span only", len(spans))
	}
	if spans[0].ChildSpanCount != 0 {
		t.Fatalf("short circuit created %d child spans", spans[0].ChildSpanCount)
	}
	event, ok := eventByName(spans[0].Events, eventShortCircuit)
	if !ok {
		t.Fatal("trace has no circuit.short_circuit event")
	}
	if got := attributeString(event.Attributes, attrPartner); got != target.Name {
		t.Fatalf("event partner %q, want %s", got, target.Name)
	}
	if got := attributeString(event.Attributes, attrBreakerState); got != "open" {
		t.Fatalf("event breaker.state %q, want open", got)
	}
	assertKeyValuesSafe(t, event.Attributes)
}

func transitionCount(partnerName, from, to string) int64 {
	var total int64
	eachPoint(metricBreakerTransitions, func(set attribute.Set, value int64) {
		if attributeString(set.ToSlice(), attrPartner) == partnerName &&
			attributeString(set.ToSlice(), attrFromState) == from &&
			attributeString(set.ToSlice(), attrToState) == to {
			total += value
		}
	})
	return total
}

func gaugeValue(partnerName string) int64 {
	var value int64
	var found bool
	eachPoint(metricBreakerState, func(set attribute.Set, observed int64) {
		if attributeString(set.ToSlice(), attrPartner) == partnerName {
			value = observed
			found = true
		}
	})
	if !found {
		return -1
	}
	return value
}

func eachPoint(name string, fn func(attribute.Set, int64)) {
	var collected metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &collected); err != nil {
		panic(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			switch data := item.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					fn(point.Attributes, point.Value)
				}
			case metricdata.Gauge[int64]:
				for _, point := range data.DataPoints {
					fn(point.Attributes, point.Value)
				}
			}
		}
	}
}

func assertMetricAttributesSafe(t *testing.T) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			switch data := item.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					assertKeyValuesSafe(t, point.Attributes.ToSlice())
				}
			case metricdata.Gauge[int64]:
				for _, point := range data.DataPoints {
					assertKeyValuesSafe(t, point.Attributes.ToSlice())
				}
			}
		}
	}
}

func eventByName(events []sdktrace.Event, name string) (sdktrace.Event, bool) {
	for _, event := range events {
		if event.Name == name {
			return event, true
		}
	}
	return sdktrace.Event{}, false
}

func attributeString(values []attribute.KeyValue, key string) string {
	for _, value := range values {
		if string(value.Key) == key {
			return value.Value.AsString()
		}
	}
	return ""
}

func assertKeyValuesSafe(t *testing.T, values []attribute.KeyValue) {
	t.Helper()
	for _, value := range values {
		if personalAttribute(string(value.Key)) || personalAttribute(value.Value.Emit()) {
			t.Fatalf("personal data in attribute %s=%s", value.Key, value.Value.Emit())
		}
	}
}

func personalAttribute(value string) bool {
	switch value {
	case "quote_id", "document", "plate", "cpf", "12345678901", "ABC1D23":
		return true
	default:
		return false
	}
}

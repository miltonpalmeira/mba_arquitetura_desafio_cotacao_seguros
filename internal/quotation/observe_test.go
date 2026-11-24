package quotation

import (
	"context"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var metricReader *sdkmetric.ManualReader

func TestMain(m *testing.M) {
	metricReader = sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader)))
	os.Exit(m.Run())
}

func TestCacheHitAndMissAreCountedAndTheHitMarksTheTrace(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), quoteIDs: originQuotes()}
	service, _ := cachedService(t, quoter)
	request := validRequest()

	hitsBefore, missesBefore := cacheCounts()
	if _, err := service.Quote(context.Background(), "corretora-a", request); err != nil {
		t.Fatalf("first Quote: %v", err)
	}
	hits, misses := cacheCounts()
	if hits-hitsBefore != 0 {
		t.Fatalf("first quote recorded %d hits, want 0", hits-hitsBefore)
	}
	if misses-missesBefore != int64(len(threePartners)) {
		t.Fatalf("first quote recorded %d misses, want %d", misses-missesBefore, len(threePartners))
	}
	if len(quoter.calls) != len(threePartners) {
		t.Fatalf("first quote made %d calls, want %d", len(quoter.calls), len(threePartners))
	}

	exporter := tracetest.NewInMemoryExporter()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter)).Tracer("test")
	ctx, span := tracer.Start(context.Background(), "POST /quotes")
	if _, err := service.Quote(ctx, "corretora-a", request); err != nil {
		t.Fatalf("second Quote: %v", err)
	}
	span.End()

	hits, misses = cacheCounts()
	if hits-hitsBefore != int64(len(threePartners)) {
		t.Fatalf("hits %d, want %d", hits-hitsBefore, len(threePartners))
	}
	if misses-missesBefore != int64(len(threePartners)) {
		t.Fatalf("second quote added misses: delta %d, want %d", misses-missesBefore, len(threePartners))
	}
	if len(quoter.calls) != len(threePartners) {
		t.Fatalf("cache hit called the partners again: %v", quoter.calls)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("%d spans, want the server span only", len(spans))
	}
	if spans[0].ChildSpanCount != 0 {
		t.Fatalf("cache hit created %d child spans", spans[0].ChildSpanCount)
	}
	seen := map[string]bool{}
	for _, event := range spans[0].Events {
		if event.Name != eventCacheHit {
			t.Fatalf("unexpected event %q", event.Name)
		}
		if got := attributeString(event.Attributes, attrTenantID); got != "corretora-a" {
			t.Fatalf("tenant_id %q", got)
		}
		seen[attributeString(event.Attributes, attrPartner)] = true
		assertKeyValuesSafe(t, event.Attributes)
	}
	if len(seen) != len(threePartners) {
		t.Fatalf("cache.hit partners %v, want the three", seen)
	}
	for _, partnerName := range []string{"partner-slow", "partner-flaky", "partner-degrading"} {
		if !seen[partnerName] {
			t.Fatalf("no cache.hit for %s", partnerName)
		}
	}
	assertMetricAttributesSafe(t)
}

func cacheCounts() (hits, misses int64) {
	eachPoint(metricCacheHit, func(_ attribute.Set, value int64) { hits += value })
	eachPoint(metricCacheMiss, func(_ attribute.Set, value int64) { misses += value })
	return hits, misses
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
			sum, ok := item.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range sum.DataPoints {
				fn(point.Attributes, point.Value)
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
			sum, ok := item.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range sum.DataPoints {
				assertKeyValuesSafe(t, point.Attributes.ToSlice())
				if attributeString(point.Attributes.ToSlice(), attrPartner) == "" && (item.Name == metricCacheHit || item.Name == metricCacheMiss) {
					t.Fatalf("%s has no partner attribute", item.Name)
				}
			}
		}
	}
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
	case "quote_id", "document", "plate", "cpf", "12345678901", "ABC1D23", "slow-origin", "flaky-origin", "degrading-origin":
		return true
	default:
		return false
	}
}

func TestCacheCountersCarryThePartner(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), quoteIDs: originQuotes()}
	service, _ := cachedService(t, quoter)
	before := partnerCounter(metricCacheMiss, "partner-slow")
	if _, err := service.Quote(context.Background(), "corretora-a", validRequest()); err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if got := partnerCounter(metricCacheMiss, "partner-slow"); got != before+1 {
		t.Fatalf("partner-slow misses %d, want %d", got, before+1)
	}
	if _, err := service.Quote(context.Background(), "corretora-a", validRequest()); err != nil {
		t.Fatalf("second Quote: %v", err)
	}
	hits := partnerCounter(metricCacheHit, "partner-slow")
	if hits < 1 {
		t.Fatal("partner-slow has no hit")
	}
}

func partnerCounter(name, partnerName string) int64 {
	var total int64
	eachPoint(name, func(set attribute.Set, value int64) {
		if attributeString(set.ToSlice(), attrPartner) == partnerName {
			total += value
		}
	})
	return total
}

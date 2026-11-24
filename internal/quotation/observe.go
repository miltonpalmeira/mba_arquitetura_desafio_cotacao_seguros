package quotation

import (
	"context"
	"log"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Instrument names are the ones the SAD and the process README repeat.
const (
	metricCacheHit  = "quote.cache.hit"
	metricCacheMiss = "quote.cache.miss"
	eventCacheHit   = "cache.hit"
)

const (
	attrPartner  = "partner"
	attrTenantID = "tenant_id"
)

var (
	cacheInstrumentsOnce sync.Once
	cacheHits            metric.Int64Counter
	cacheMisses          metric.Int64Counter
)

func ensureCacheInstruments() {
	cacheInstrumentsOnce.Do(func() {
		meter := otel.Meter("github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/quotation")
		var err error
		cacheHits, err = meter.Int64Counter(
			metricCacheHit,
			metric.WithDescription("Cache hits for one partner. The entry was inside the TTL and the partner was not called."),
		)
		if err != nil {
			log.Printf("quotation-api: cache hit counter: %v", err)
		}
		cacheMisses, err = meter.Int64Counter(
			metricCacheMiss,
			metric.WithDescription("Cache misses for one partner. The partner is consulted, or the consultation fails."),
		)
		if err != nil {
			log.Printf("quotation-api: cache miss counter: %v", err)
		}
	})
}

func noteCacheHit(ctx context.Context, tenant, partnerName string) {
	ensureCacheInstruments()
	if cacheHits != nil {
		cacheHits.Add(ctx, 1, metric.WithAttributes(attribute.String(attrPartner, partnerName)))
	}
	span := trace.SpanFromContext(ctx)
	span.AddEvent(eventCacheHit, trace.WithAttributes(
		attribute.String(attrPartner, partnerName),
		attribute.String(attrTenantID, tenant),
	))
}

func noteCacheMiss(ctx context.Context, partnerName string) {
	ensureCacheInstruments()
	if cacheMisses == nil {
		return
	}
	cacheMisses.Add(ctx, 1, metric.WithAttributes(attribute.String(attrPartner, partnerName)))
}

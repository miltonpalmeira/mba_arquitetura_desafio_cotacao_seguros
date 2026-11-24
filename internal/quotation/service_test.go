package quotation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

var threePartners = []platform.Partner{
	{Name: "partner-slow", BaseURL: "http://slow"},
	{Name: "partner-flaky", BaseURL: "http://flaky"},
	{Name: "partner-degrading", BaseURL: "http://degrading"},
}

func validRequest() Request {
	r := Request{
		Driver:  Driver{Document: "12345678901", BirthYear: 1988},
		Vehicle: Vehicle{Plate: "ABC1D23", Model: "Gol 1.0", Year: 2020, ValueCents: 8500000},
	}
	if err := r.Normalize(); err != nil {
		panic(err)
	}
	return r
}

type fakeQuoter struct {
	delay      time.Duration
	premiums   map[string]int64
	quoteIDs   map[string]string
	failOn     string
	failAll    bool
	failReason string
	calls      []string
	requests   []any
	concurrent atomic.Int32
	peak       atomic.Int32
}

func (f *fakeQuoter) Quote(_ context.Context, p platform.Partner, request any) (partner.Quote, error) {
	if now := f.concurrent.Add(1); now > f.peak.Load() {
		f.peak.Store(now)
	}
	defer f.concurrent.Add(-1)

	f.calls = append(f.calls, p.Name)
	f.requests = append(f.requests, request)
	time.Sleep(f.delay)

	if f.failAll || p.Name == f.failOn {
		reason := f.failReason
		if reason == "" {
			reason = "partner answered 503"
		}
		return partner.Quote{}, &partner.Error{Partner: p.Name, Status: 503, Reason: reason}
	}
	return partner.Quote{
		Partner:      p.Name,
		QuoteID:      f.quoteIDs[p.Name],
		PremiumCents: f.premiums[p.Name],
		Currency:     "BRL",
	}, nil
}

func defaultPremiums() map[string]int64 {
	return map[string]int64{"partner-slow": 180000, "partner-flaky": 90000, "partner-degrading": 120000}
}

func TestQuoteAggregatesTheThreePartnersSortedByPremium(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums()}
	response, err := NewService(threePartners, quoter).Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}

	if len(response.Quotes) != 3 {
		t.Fatalf("%d quotes, expected 3", len(response.Quotes))
	}
	expected := []string{"partner-flaky", "partner-degrading", "partner-slow"}
	for i, name := range expected {
		if response.Quotes[i].Partner != name {
			t.Errorf("quote %d is from %q, expected from %q", i, response.Quotes[i].Partner, name)
		}
	}
	if response.TenantID != "corretora-a" {
		t.Errorf("tenant_id %q, expected corretora-a", response.TenantID)
	}
	if response.Degraded || len(response.UnavailablePartners) != 0 {
		t.Fatalf("complete quote marked degraded: %+v", response.UnavailablePartners)
	}
	if response.UnavailablePartners == nil {
		t.Fatal("unavailable_partners is null; the broker needs a list")
	}
}

func TestQuoteCallsThePartnersSerially(t *testing.T) {
	quoter := &fakeQuoter{delay: 40 * time.Millisecond, premiums: defaultPremiums()}

	start := time.Now()
	if _, err := NewService(threePartners, quoter).Quote(context.Background(), "corretora-a", validRequest()); err != nil {
		t.Fatalf("Quote: %v", err)
	}
	elapsed := time.Since(start)

	if minimum := 3 * quoter.delay; elapsed < minimum {
		t.Fatalf("aggregation took %s; serially it should take at least %s", elapsed, minimum)
	}
	if peak := quoter.peak.Load(); peak != 1 {
		t.Fatalf("%d concurrent calls at the peak, expected 1 (serial calls)", peak)
	}
}

func TestOneNamedPartnerDownKeepsThePremiumsAlreadyObtained(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), failOn: "partner-flaky"}

	response, err := NewService(threePartners, quoter).Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if !response.Degraded {
		t.Fatal("degraded is false while partner-flaky produced no premium")
	}
	if len(response.UnavailablePartners) != 1 || response.UnavailablePartners[0] != "partner-flaky" {
		t.Fatalf("unavailable %v, want [partner-flaky]", response.UnavailablePartners)
	}
	if len(quoter.calls) != 3 || quoter.calls[2] != "partner-degrading" {
		t.Fatalf("calls %v, want the loop to continue after partner-flaky", quoter.calls)
	}

	if len(response.Quotes) != 2 {
		t.Fatalf("%d quotes, want the two partners that answered", len(response.Quotes))
	}
	if response.Quotes[0].Partner != "partner-degrading" || response.Quotes[0].PremiumCents != 120000 {
		t.Fatalf("first quote %+v, want partner-degrading at 120000", response.Quotes[0])
	}
	if response.Quotes[1].Partner != "partner-slow" || response.Quotes[1].PremiumCents != 180000 {
		t.Fatalf("second quote %+v, want partner-slow at 180000", response.Quotes[1])
	}
	for _, quote := range response.Quotes {
		if quote.Partner == "partner-flaky" {
			t.Fatalf("failed partner contributed a premium: %+v", quote)
		}
	}
}

func TestEveryPartnerDownIsNoQuote(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), failAll: true}

	_, err := NewService(threePartners, quoter).Quote(context.Background(), "corretora-a", validRequest())
	var missed *noQuoteError
	if !errors.As(err, &missed) {
		t.Fatalf("error %v, want no_quote", err)
	}
	if missed.Error() != messageNoQuote {
		t.Fatalf("error text %q, want %s", missed.Error(), messageNoQuote)
	}
	want := []string{"partner-slow", "partner-flaky", "partner-degrading"}
	if len(missed.partners) != len(want) {
		t.Fatalf("partners %v, want %v", missed.partners, want)
	}
	for i, name := range want {
		if missed.partners[i] != name {
			t.Fatalf("partners %v, want %v", missed.partners, want)
		}
	}
	if len(quoter.calls) != 3 {
		t.Fatalf("calls %v, want the three partners", quoter.calls)
	}
}

func TestBrokerGoesInThePartnerRequest(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums()}
	service := NewService(threePartners[:1], quoter)

	if _, err := service.Quote(context.Background(), "corretora-a", validRequest()); err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if _, err := service.Quote(context.Background(), "corretora-b", validRequest()); err != nil {
		t.Fatalf("Quote: %v", err)
	}

	toA, toB := serialize(t, quoter.requests[0]), serialize(t, quoter.requests[1])
	if toA == toB {
		t.Fatalf("different brokers produced the same request to the partner: %s", toA)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(toA), &body); err != nil {
		t.Fatalf("request to the partner is not JSON: %v", err)
	}
	if body["broker"] != "corretora-a" {
		t.Errorf("broker %v, expected corretora-a", body["broker"])
	}
	if _, ok := body["vehicle"]; !ok {
		t.Errorf("request to the partner does not carry the vehicle: %s", toA)
	}
}

func originQuotes() map[string]string {
	return map[string]string{
		"partner-slow":      "slow-origin",
		"partner-flaky":     "flaky-origin",
		"partner-degrading": "degrading-origin",
	}
}

func cachedService(t *testing.T, quoter Quoter) (*Service, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	server.SetTime(time.Date(2026, 11, 22, 22, 0, 0, 0, time.UTC))
	store, err := NewRedisStore(server.Addr(), 900*time.Second)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewService(threePartners, quoter).WithCache(store), server
}

func TestCacheKeyCarriesTheBrokerAndNormalizesTheRisk(t *testing.T) {
	request := validRequest()
	request.Driver.Document = "123.456.789-01"
	request.Vehicle.Plate = "abc-1d23"
	request.Vehicle.Model = "  gol   1.0 "

	key := cacheKey("corretora-a", "partner-slow", request)
	const want = "quote:corretora-a:partner-slow:12345678901:1988:ABC1D23:GOL 1.0:2020:8500000:comprehensive"
	if key != want {
		t.Fatalf("key %q, want %q", key, want)
	}
	if !strings.Contains(key, ":corretora-a:") {
		t.Fatal("key has no tenant_id")
	}
	if cacheKey("corretora-b", "partner-slow", request) == key {
		t.Fatal("another broker reused the key")
	}
}

func TestSecondQuoteWithTheSameBrokerDoesNotCallThePartners(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), quoteIDs: originQuotes()}
	service, server := cachedService(t, quoter)

	first, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("first Quote: %v", err)
	}
	if len(quoter.calls) != 3 {
		t.Fatalf("first quote made %d calls, want 3", len(quoter.calls))
	}

	second, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("second Quote: %v", err)
	}
	if len(quoter.calls) != 3 {
		t.Fatalf("second quote called the partners again: %v", quoter.calls)
	}
	if len(second.Quotes) != len(first.Quotes) {
		t.Fatalf("second quote returned %d proposals, want %d", len(second.Quotes), len(first.Quotes))
	}
	for i := range first.Quotes {
		if second.Quotes[i].QuoteID != first.Quotes[i].QuoteID || second.Quotes[i].PremiumCents != first.Quotes[i].PremiumCents {
			t.Fatalf("second quote %+v is not the stored origin %+v", second.Quotes[i], first.Quotes[i])
		}
	}

	if len(server.Keys()) != 3 {
		t.Fatalf("%d cache keys, want one per partner", len(server.Keys()))
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "origin") || strings.Contains(key, "quote_id") {
			t.Fatalf("origin id leaked into the key %q", key)
		}
		if !strings.Contains(key, ":corretora-a:") {
			t.Fatalf("key %q has no broker", key)
		}
		raw, err := server.Get(key)
		if err != nil {
			t.Fatalf("Get %s: %v", key, err)
		}
		if !strings.Contains(raw, `"quote_id"`) || !strings.Contains(raw, `"partner"`) {
			t.Fatalf("stored value %s does not point at the origin quote", raw)
		}
		if ttl := server.TTL(key); ttl != 900*time.Second {
			t.Fatalf("ttl of %s is %s, want 900s", key, ttl)
		}
	}
}

func TestAnotherBrokerDoesNotReuseTheCacheEntry(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), quoteIDs: originQuotes()}
	service, _ := cachedService(t, quoter)
	request := validRequest()

	if _, err := service.Quote(context.Background(), "corretora-a", request); err != nil {
		t.Fatalf("Quote for corretora-a: %v", err)
	}
	if _, err := service.Quote(context.Background(), "corretora-b", request); err != nil {
		t.Fatalf("Quote for corretora-b: %v", err)
	}
	if len(quoter.calls) != 6 {
		t.Fatalf("%d partner calls, want 6: the second broker must not reuse the entry", len(quoter.calls))
	}
}

func TestOpenCircuitServesTheEntryInsideTheTTL(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), quoteIDs: originQuotes()}
	service, _ := cachedService(t, quoter)

	first, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("first Quote: %v", err)
	}

	quoter.failOn = "partner-flaky"
	quoter.failReason = "circuit open"
	second, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("fresh entry was not served while the partner would fail: %v", err)
	}
	if len(quoter.calls) != 3 {
		t.Fatalf("open circuit called the partners again: %v", quoter.calls)
	}
	if second.Quotes[0].Partner != "partner-flaky" || second.Quotes[0].QuoteID != "flaky-origin" || second.Quotes[0].PremiumCents != first.Quotes[0].PremiumCents {
		t.Fatalf("served %+v, want the stored flaky quote", second.Quotes[0])
	}
	if second.Degraded || len(second.UnavailablePartners) != 0 {
		t.Fatalf("cache hit was marked degraded: %v", second.UnavailablePartners)
	}
}

func TestExpiredEntryIsNotServedWhenTheCircuitIsOpen(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), quoteIDs: originQuotes()}
	service, server := cachedService(t, quoter)
	request := validRequest()

	if _, err := service.Quote(context.Background(), "corretora-a", request); err != nil {
		t.Fatalf("first Quote: %v", err)
	}

	key := cacheKey("corretora-a", "partner-flaky", request)
	server.SetTTL(key, time.Millisecond)
	server.FastForward(time.Second)
	quoter.failOn = "partner-flaky"
	quoter.failReason = "circuit open"
	response, err := service.Quote(context.Background(), "corretora-a", request)
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if !response.Degraded {
		t.Fatal("expired partner was not marked degraded")
	}
	if len(response.UnavailablePartners) != 1 || response.UnavailablePartners[0] != "partner-flaky" {
		t.Fatalf("unavailable %v, want partner-flaky", response.UnavailablePartners)
	}
	for _, quote := range response.Quotes {
		if quote.Partner == "partner-flaky" || quote.QuoteID == "flaky-origin" {
			t.Fatalf("expired entry was served: %+v", quote)
		}
	}
	if len(response.Quotes) != 2 {
		t.Fatalf("%d quotes, want the two still inside the TTL", len(response.Quotes))
	}
	if response.Quotes[0].QuoteID != "degrading-origin" || response.Quotes[1].QuoteID != "slow-origin" {
		t.Fatalf("quotes %+v, want the cached degrading and slow origins", response.Quotes)
	}
	if len(quoter.calls) != 4 {
		t.Fatalf("calls %v, want only the expired partner consulted again", quoter.calls)
	}

	if server.Exists(key) {
		t.Fatal("expired flaky quote is still stored")
	}
	flakyCalls := 0
	for _, name := range quoter.calls {
		if name == "partner-flaky" {
			flakyCalls++
		}
	}
	if flakyCalls != 2 {
		t.Fatalf("partner-flaky calls %d, want the origin and the miss after expiry", flakyCalls)
	}
}

func serialize(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

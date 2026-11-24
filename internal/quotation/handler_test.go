package quotation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

const validBody = `{
  "driver": {"document": "12345678901", "birth_year": 1988},
  "vehicle": {"plate": "abc1d23", "model": "Gol 1.0", "year": 2020, "value_cents": 8500000},
  "coverage": "comprehensive"
}`

func testAPI(quoter Quoter) http.Handler {
	return NewAPI(NewService(threePartners, quoter), []string{"corretora-a", "corretora-b"}).Routes()
}

func postQuotes(h http.Handler, tenant, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader(body))
	if tenant != "" {
		req.Header.Set(TenantHeader, tenant)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	return response
}

func TestQuotesReturnsAggregatedQuotes(t *testing.T) {
	response := postQuotes(testAPI(&fakeQuoter{premiums: defaultPremiums()}), "corretora-a", validBody)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d, expected 200: %s", response.Code, response.Body)
	}

	var body Response
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unreadable response: %v", err)
	}
	if len(body.Quotes) != 3 {
		t.Fatalf("%d quotes, expected 3", len(body.Quotes))
	}
	if body.TenantID != "corretora-a" {
		t.Errorf("tenant_id %q, expected corretora-a", body.TenantID)
	}
	if body.Degraded || len(body.UnavailablePartners) != 0 {
		t.Fatalf("complete quote marked degraded: %v", body.UnavailablePartners)
	}
	if !strings.Contains(response.Body.String(), `"unavailable_partners":[]`) {
		t.Fatalf("body %s does not show an empty list", response.Body)
	}
}

func TestQuotesRejectsRequestWithoutTenant(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums()}
	response := postQuotes(testAPI(quoter), "", validBody)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status %d, expected 400", response.Code)
	}
	if len(quoter.calls) != 0 {
		t.Fatalf("the partners were called (%v) even without a tenant", quoter.calls)
	}
}

func TestQuotesRejectsUnknownBroker(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums()}
	response := postQuotes(testAPI(quoter), "corretora-pirata", validBody)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status %d, expected 403", response.Code)
	}
	if len(quoter.calls) != 0 {
		t.Fatalf("the partners were called (%v) for a broker that is not enabled", quoter.calls)
	}
}

func TestQuotesRejectsInvalidBody(t *testing.T) {
	cases := map[string]string{
		"broken json":      `{"driver":`,
		"missing document": `{"driver":{"birth_year":1988},"vehicle":{"plate":"ABC1D23","year":2020,"value_cents":100}}`,
		"missing plate":    `{"driver":{"document":"1","birth_year":1988},"vehicle":{"year":2020,"value_cents":100}}`,
		"zero value":       `{"driver":{"document":"1","birth_year":1988},"vehicle":{"plate":"A","year":2020,"value_cents":0}}`,
		"invalid coverage": `{"driver":{"document":"1","birth_year":1988},"vehicle":{"plate":"A","year":2020,"value_cents":1},"coverage":"vip"}`,
		"unknown field":    `{"driver":{"document":"1","birth_year":1988},"vehicle":{"plate":"A","year":2020,"value_cents":1},"discount":true}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			response := postQuotes(testAPI(&fakeQuoter{premiums: defaultPremiums()}), "corretora-a", body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status %d, expected 400: %s", response.Code, response.Body)
			}
		})
	}
}

func TestQuotesReturnsPartialWhenOneNamedPartnerFails(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), failOn: "partner-flaky"}
	response := postQuotes(testAPI(quoter), "corretora-a", validBody)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, expected 200: %s", response.Code, response.Body)
	}
	if response.Header().Get(TenantHeader) != "corretora-a" {
		t.Errorf("tenant header %q, expected corretora-a", response.Header().Get(TenantHeader))
	}
	if strings.Contains(response.Body.String(), messagePartnerUnavailable) {
		t.Fatalf("partial quote used the 502 message: %s", response.Body)
	}

	var body Response
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unreadable response: %v", err)
	}
	if !body.Degraded {
		t.Fatal("degraded is false")
	}
	if len(body.UnavailablePartners) != 1 || body.UnavailablePartners[0] != "partner-flaky" {
		t.Fatalf("unavailable %v, want [partner-flaky]", body.UnavailablePartners)
	}
	if len(body.Quotes) != 2 || body.Quotes[0].PremiumCents > body.Quotes[1].PremiumCents {
		t.Fatalf("quotes %+v, want two premiums in ascending order", body.Quotes)
	}
	for _, quote := range body.Quotes {
		if quote.Partner == "partner-flaky" {
			t.Fatalf("failed partner contributed a premium: %+v", quote)
		}
	}
}

func TestQuotesResponds503NoQuoteWhenEveryPartnerFails(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums(), failAll: true}
	response := postQuotes(testAPI(quoter), "corretora-a", validBody)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, expected 503: %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), messagePartnerUnavailable) {
		t.Fatalf("no_quote reused the 502 message: %s", response.Body)
	}

	var body noQuoteBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("unreadable error: %v", err)
	}
	if body.Message != messageNoQuote {
		t.Errorf("error %q, want %s", body.Message, messageNoQuote)
	}
	want := []string{"partner-slow", "partner-flaky", "partner-degrading"}
	if len(body.UnavailablePartners) != len(want) {
		t.Fatalf("partners %v, want %v", body.UnavailablePartners, want)
	}
	for i, name := range want {
		if body.UnavailablePartners[i] != name {
			t.Fatalf("partners %v, want %v", body.UnavailablePartners, want)
		}
	}
	if body.RetryAfterSeconds != retryAfterSeconds {
		t.Errorf("retry_after_seconds %d, want %d", body.RetryAfterSeconds, retryAfterSeconds)
	}
}

func TestRetryAfterMatchesTheBreakerOpenTime(t *testing.T) {
	open := int(platform.DefaultResilience().OpenFor / time.Second)
	if retryAfterSeconds != open {
		t.Fatalf("retry_after_seconds %d, breaker open time %d s", retryAfterSeconds, open)
	}
}

func TestHealthz(t *testing.T) {
	response := httptest.NewRecorder()
	testAPI(&fakeQuoter{premiums: defaultPremiums()}).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, expected 200", response.Code)
	}
}

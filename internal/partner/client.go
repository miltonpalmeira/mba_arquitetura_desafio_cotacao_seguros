package partner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/sony/gobreaker/v2"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

type Quote struct {
	Partner         string `json:"partner"`
	QuoteID         string `json:"quote_id"`
	PremiumCents    int64  `json:"premium_cents"`
	Currency        string `json:"currency"`
	CoverageCents   int64  `json:"coverage_cents"`
	ValidForSeconds int64  `json:"valid_for_seconds"`
}

type Client struct {
	http       *http.Client
	resilience platform.Resilience

	mu       sync.Mutex
	breakers map[string]*gobreaker.CircuitBreaker[Quote]
}

func NewClient(r platform.Resilience) *Client {
	return newClient(r, nil)
}

func newClient(r platform.Resilience, base http.RoundTripper) *Client {
	if r.PartnerTimeout <= 0 || r.ConsecutiveFailures == 0 || r.Interval <= 0 || r.OpenFor <= 0 || r.HalfOpenMaxRequests == 0 {
		r = platform.DefaultResilience()
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &Client{
		http: &http.Client{
			Timeout:   r.PartnerTimeout,
			Transport: platform.InstrumentTransport(base),
		},
		resilience: r,
		breakers:   map[string]*gobreaker.CircuitBreaker[Quote]{},
	}
}

// StateName reports closed, open or half-open for the breaker of one partner.
func (c *Client) StateName(partner string) string {
	switch c.breaker(partner).State() {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

func (c *Client) breaker(name string) *gobreaker.CircuitBreaker[Quote] {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.breakers[name]; ok {
		return existing
	}

	created := gobreaker.NewCircuitBreaker[Quote](gobreaker.Settings{
		Name:        name,
		MaxRequests: c.resilience.HalfOpenMaxRequests,
		Interval:    c.resilience.Interval,
		Timeout:     c.resilience.OpenFor,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= c.resilience.ConsecutiveFailures
		},
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled)
		},
	})
	c.breakers[name] = created
	return created
}

const responseLimit = 1 << 20

func (c *Client) Quote(ctx context.Context, p platform.Partner, request any) (Quote, error) {
	if err := ctx.Err(); err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: err.Error()}
	}

	quote, err := c.breaker(p.Name).Execute(func() (Quote, error) {
		return c.call(ctx, p, request)
	})
	if err == nil {
		return quote, nil
	}
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return Quote{}, &Error{Partner: p.Name, Reason: "circuit open"}
	}
	return Quote{}, err
}

func (c *Client) call(ctx context.Context, p platform.Partner, request any) (Quote, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: fmt.Sprintf("invalid request: %v", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/quotes", bytes.NewReader(body))
	if err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(req)
	if err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: err.Error()}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, responseLimit))
		return Quote{}, &Error{
			Partner: p.Name,
			Status:  response.StatusCode,
			Reason:  fmt.Sprintf("partner replied %d", response.StatusCode),
		}
	}

	var quote Quote
	if err := json.NewDecoder(io.LimitReader(response.Body, responseLimit)).Decode(&quote); err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: fmt.Sprintf("unreadable response: %v", err)}
	}

	quote.Partner = p.Name
	return quote, nil
}

type Error struct {
	Partner string
	Status  int
	Reason  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("partner %s: %s", e.Partner, e.Reason)
}

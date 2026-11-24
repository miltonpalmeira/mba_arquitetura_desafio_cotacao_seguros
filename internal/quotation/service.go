package quotation

import (
	"context"
	"sort"
	"time"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

type Quoter interface {
	Quote(ctx context.Context, p platform.Partner, request any) (partner.Quote, error)
}

type Service struct {
	partners []platform.Partner
	quoter   Quoter
	cache    quoteCache
}

func NewService(partners []platform.Partner, quoter Quoter) *Service {
	return &Service{partners: partners, quoter: quoter}
}

// WithCache reads a per-partner quote before calling the insurer.
// A nil cache leaves the service as it was: every partner is called.
func (s *Service) WithCache(cache quoteCache) *Service {
	s.cache = cache
	return s
}

// Quote keeps every premium already obtained from a partner or from a cache
// entry inside the TTL. A partner that fails is named and the loop continues.
// At least one premium is a partial quote. None is noQuote.
func (s *Service) Quote(ctx context.Context, tenant string, request Request) (Response, error) {
	start := time.Now()
	forPartner := partnerRequest{Broker: tenant, Request: request}

	quotes := make([]partner.Quote, 0, len(s.partners))
	unavailable := make([]string, 0)
	for _, p := range s.partners {
		quote, err := s.quotePartner(ctx, tenant, p, request, forPartner)
		if err != nil {
			unavailable = append(unavailable, p.Name)
			continue
		}
		quotes = append(quotes, quote)
	}

	if len(quotes) == 0 {
		return Response{}, &noQuoteError{partners: unavailable}
	}

	sort.Slice(quotes, func(i, j int) bool {
		return quotes[i].PremiumCents < quotes[j].PremiumCents
	})

	return Response{
		TenantID:            tenant,
		Quotes:              quotes,
		ElapsedMs:           time.Since(start).Milliseconds(),
		Degraded:            len(unavailable) > 0,
		UnavailablePartners: unavailable,
	}, nil
}

// quotePartner serves an entry inside the TTL without calling the insurer.
// That holds when the breaker is open: the call never leaves. An expired
// entry is a miss. The partner quote is stored only after a successful call,
// with partner and quote_id in the value.
func (s *Service) quotePartner(ctx context.Context, tenant string, p platform.Partner, request Request, forPartner partnerRequest) (partner.Quote, error) {
	key := cacheKey(tenant, p.Name, request)
	if s.cache != nil {
		if quote, ok := s.cache.Get(ctx, key); ok {
			noteCacheHit(ctx, tenant, p.Name)
			return quote, nil
		}
		noteCacheMiss(ctx, p.Name)
	}

	quote, err := s.quoter.Quote(ctx, p, forPartner)
	if err != nil {
		return partner.Quote{}, err
	}
	if s.cache != nil {
		s.cache.Set(ctx, key, quote)
	}
	return quote, nil
}

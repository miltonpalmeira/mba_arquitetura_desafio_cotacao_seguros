package platform

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port       string
	Partners   []Partner
	Tenants    []string
	Telemetry  Telemetry
	Resilience Resilience
	Cache      Cache
}

// Cache is the Redis address and the per-partner quote TTL.
// The numbers are the ones defended in the SAD.
type Cache struct {
	Addr string
	TTL  time.Duration
}

// Resilience is the partner timeout and the per-partner circuit breaker.
// The numbers are the ones defended in the SAD.
type Resilience struct {
	PartnerTimeout      time.Duration
	ConsecutiveFailures uint32
	Interval            time.Duration
	OpenFor             time.Duration
	HalfOpenMaxRequests uint32
}

type Partner struct {
	Name    string
	BaseURL string
}

type Telemetry struct {
	ServiceName string
	Endpoint    string
	Enabled     bool
}

const defaultEndpoints = "partner-slow=http://localhost:9001," +
	"partner-flaky=http://localhost:9002," +
	"partner-degrading=http://localhost:9003"

const defaultTenants = "corretora-a,corretora-b"

const defaultCollector = "http://localhost:4317"

const defaultServiceName = "quotation-api"

const (
	defaultPartnerTimeoutMS    = 2500
	defaultConsecutiveFailures = 5
	defaultBreakerIntervalMS   = 60000
	defaultBreakerOpenMS       = 30000
	defaultHalfOpenRequests    = 1
)

const defaultRedisAddr = "localhost:6379"

const defaultCacheTTLSeconds = 900

func DefaultResilience() Resilience {
	return Resilience{
		PartnerTimeout:      defaultPartnerTimeoutMS * time.Millisecond,
		ConsecutiveFailures: defaultConsecutiveFailures,
		Interval:            defaultBreakerIntervalMS * time.Millisecond,
		OpenFor:             defaultBreakerOpenMS * time.Millisecond,
		HalfOpenMaxRequests: defaultHalfOpenRequests,
	}
}

func DefaultCache() Cache {
	return Cache{
		Addr: defaultRedisAddr,
		TTL:  defaultCacheTTLSeconds * time.Second,
	}
}

func LoadConfig(env func(string) string) (Config, error) {
	cfg := Config{Port: text(env, "PORT", "8080")}

	var err error
	if cfg.Partners, err = parsePartners(text(env, "PARTNER_ENDPOINTS", defaultEndpoints)); err != nil {
		return Config{}, err
	}
	if cfg.Tenants, err = parseTenants(text(env, "TENANTS", defaultTenants)); err != nil {
		return Config{}, err
	}
	if cfg.Telemetry, err = parseTelemetry(env); err != nil {
		return Config{}, err
	}
	if cfg.Resilience, err = parseResilience(env); err != nil {
		return Config{}, err
	}
	if cfg.Cache, err = parseCache(env); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseCache(env func(string) string) (Cache, error) {
	addr := strings.TrimSpace(text(env, "REDIS_ADDR", defaultRedisAddr))
	if addr == "" {
		return Cache{}, fmt.Errorf("REDIS_ADDR is required")
	}
	seconds, err := positiveInt(env, "CACHE_TTL_SECONDS", defaultCacheTTLSeconds)
	if err != nil {
		return Cache{}, err
	}
	return Cache{Addr: addr, TTL: time.Duration(seconds) * time.Second}, nil
}

func parseResilience(env func(string) string) (Resilience, error) {
	cfg := DefaultResilience()

	timeout, err := positiveInt(env, "PARTNER_TIMEOUT_MS", defaultPartnerTimeoutMS)
	if err != nil {
		return Resilience{}, err
	}
	failures, err := positiveInt(env, "BREAKER_CONSECUTIVE_FAILURES", defaultConsecutiveFailures)
	if err != nil {
		return Resilience{}, err
	}
	interval, err := positiveInt(env, "BREAKER_INTERVAL_MS", defaultBreakerIntervalMS)
	if err != nil {
		return Resilience{}, err
	}
	openFor, err := positiveInt(env, "BREAKER_OPEN_MS", defaultBreakerOpenMS)
	if err != nil {
		return Resilience{}, err
	}
	halfOpen, err := positiveInt(env, "BREAKER_HALF_OPEN_MAX", defaultHalfOpenRequests)
	if err != nil {
		return Resilience{}, err
	}

	cfg.PartnerTimeout = time.Duration(timeout) * time.Millisecond
	cfg.ConsecutiveFailures = uint32(failures)
	cfg.Interval = time.Duration(interval) * time.Millisecond
	cfg.OpenFor = time.Duration(openFor) * time.Millisecond
	cfg.HalfOpenMaxRequests = uint32(halfOpen)
	return cfg, nil
}

func positiveInt(env func(string) string, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(env(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: %q is not a positive integer", key, raw)
	}
	return n, nil
}

func parseTelemetry(env func(string) string) (Telemetry, error) {
	telemetry := Telemetry{
		ServiceName: text(env, "OTEL_SERVICE_NAME", defaultServiceName),
		Endpoint:    strings.TrimSpace(text(env, "OTEL_EXPORTER_OTLP_ENDPOINT", defaultCollector)),
	}

	switch disabled := strings.ToLower(strings.TrimSpace(env("OTEL_SDK_DISABLED"))); disabled {
	case "", "false":
		telemetry.Enabled = true
	case "true":
		return Telemetry{Enabled: false}, nil
	default:
		return Telemetry{}, fmt.Errorf("OTEL_SDK_DISABLED: %q is neither true nor false", disabled)
	}

	if u, err := url.Parse(telemetry.Endpoint); err != nil || u.Scheme == "" || u.Host == "" {
		return Telemetry{}, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT: %q is not an absolute URL", telemetry.Endpoint)
	}
	return telemetry, nil
}

func parsePartners(raw string) ([]Partner, error) {
	var partners []Partner
	seen := map[string]bool{}

	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		name, endpoint, found := strings.Cut(item, "=")
		name, endpoint = strings.TrimSpace(name), strings.TrimSpace(endpoint)
		if !found || name == "" || endpoint == "" {
			return nil, fmt.Errorf("PARTNER_ENDPOINTS: %q is not in the name=url format", item)
		}

		if u, err := url.Parse(endpoint); err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("PARTNER_ENDPOINTS: %q is not an absolute URL", endpoint)
		}
		if seen[name] {
			return nil, fmt.Errorf("PARTNER_ENDPOINTS: partner %q is repeated", name)
		}

		seen[name] = true
		partners = append(partners, Partner{Name: name, BaseURL: strings.TrimRight(endpoint, "/")})
	}

	if len(partners) == 0 {
		return nil, fmt.Errorf("PARTNER_ENDPOINTS: at least one partner is required")
	}
	return partners, nil
}

func parseTenants(raw string) ([]string, error) {
	var tenants []string
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tenants = append(tenants, t)
		}
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("TENANTS: at least one broker is required")
	}
	return tenants, nil
}

func Environment() func(string) string { return os.Getenv }

func text(env func(string) string, key, fallback string) string {
	if v := env(key); v != "" {
		return v
	}
	return fallback
}

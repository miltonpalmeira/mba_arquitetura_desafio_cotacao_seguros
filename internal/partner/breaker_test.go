package partner

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

func TestDefaultClientTimeoutIs2500ms(t *testing.T) {
	client := NewClient(platform.DefaultResilience())
	if client.http.Timeout != 2500*time.Millisecond {
		t.Fatalf("client timeout %s, want 2500ms", client.http.Timeout)
	}
}

func TestTimeoutCountsAsBreakerFailure(t *testing.T) {
	var calls atomic.Int32
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})

	settings := platform.DefaultResilience()
	settings.PartnerTimeout = 15 * time.Millisecond
	client := newClient(settings, transport)
	target := platform.Partner{Name: "partner-degrading", BaseURL: "http://partner-degrading"}

	for range settings.ConsecutiveFailures {
		if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
			t.Fatal("partner timeout was treated as success")
		}
	}
	if client.StateName(target.Name) != "open" {
		t.Fatalf("state %q after timeouts, want open", client.StateName(target.Name))
	}

	before := calls.Load()
	if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
		t.Fatal("open circuit returned a quote")
	}
	if calls.Load() != before {
		t.Fatalf("open circuit called the partner %d extra times", calls.Load()-before)
	}
}

func TestOpenCircuitDoesNotAdvanceTheFlakySequence(t *testing.T) {
	base, observed := startFlakyMock(t)
	client := newClient(platform.DefaultResilience(), &sequenceTransport{
		base:     http.DefaultTransport,
		observed: observed,
	})
	target := platform.Partner{Name: "partner-flaky", BaseURL: base}

	opened := false
	for range 210 {
		_, err := client.Quote(context.Background(), target, map[string]string{})
		if client.StateName(target.Name) != "open" {
			continue
		}
		opened = true
		if err == nil {
			t.Fatal("the call that opened the circuit was a success")
		}
		if observed.calls.Load() != 42 || observed.lastSeq.Load() != 42 {
			t.Fatalf("opened after %d calls at sequence %d, want 42 and 42", observed.calls.Load(), observed.lastSeq.Load())
		}
		break
	}
	if !opened {
		t.Fatal("five consecutive failures did not open the circuit within 210 calls")
	}

	before, seq := observed.calls.Load(), observed.lastSeq.Load()
	if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
		t.Fatal("open circuit returned a quote")
	}
	if observed.calls.Load() != before || observed.lastSeq.Load() != seq {
		t.Fatalf("open circuit reached the mock: calls=%d sequence=%d", observed.calls.Load(), observed.lastSeq.Load())
	}
}

type mockObservation struct {
	calls   atomic.Int32
	lastSeq atomic.Uint64
}

func startFlakyMock(t *testing.T) (string, *mockObservation) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))

	binary := filepath.Join(t.TempDir(), "partner-mock.exe")
	build := exec.Command("go", "build", "-o", binary, "./cmd/partner-mock")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build partner-mock: %v\n%s", err, output)
	}

	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PORT="+strconv.Itoa(port),
		"PARTNER_NAME=partner-flaky",
		"PARTNER_SEED=20260729",
		"PARTNER_LATENCY_MS=0",
		"PARTNER_JITTER_MS=0",
		"PARTNER_FAILURE_RATE=0.4",
		"PARTNER_FAILURE_STATUS=503",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start partner-mock: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	observed := &mockObservation{}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(base + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return base, observed
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("partner-mock did not become healthy")
	return "", nil
}

type sequenceTransport struct {
	base     http.RoundTripper
	observed *mockObservation
}

func (s *sequenceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := s.base.RoundTrip(r)
	if err != nil || response == nil || !strings.HasSuffix(r.URL.Path, "/quotes") {
		return response, err
	}
	s.observed.calls.Add(1)
	if raw := response.Header.Get("X-Partner-Seq"); raw != "" {
		seq, conv := strconv.ParseUint(raw, 10, 64)
		if conv == nil {
			s.observed.lastSeq.Store(seq)
		}
	}
	return response, nil
}

func TestBreakerVisitsTheThreeStates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		fail := atomic.Bool{}
		fail.Store(true)
		transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			if fail.Load() {
				return partnerResponse(r, http.StatusServiceUnavailable, `{"error":"partner unavailable"}`), nil
			}
			return partnerResponse(r, http.StatusOK, quoteBody), nil
		})

		settings := platform.DefaultResilience()
		client := newClient(settings, transport)
		target := platform.Partner{Name: "partner-flaky", BaseURL: "http://partner-flaky"}

		if client.StateName(target.Name) != "closed" {
			t.Fatalf("initial state %q, want closed", client.StateName(target.Name))
		}
		for range settings.ConsecutiveFailures {
			if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
				t.Fatal("failure was treated as success")
			}
		}
		if client.StateName(target.Name) != "open" {
			t.Fatalf("state %q, want open", client.StateName(target.Name))
		}

		// The open window is the configured 30s. synctest advances that clock without waiting on the machine.
		time.Sleep(settings.OpenFor + time.Nanosecond)
		if client.StateName(target.Name) != "half-open" {
			t.Fatalf("state %q after the open window, want half-open", client.StateName(target.Name))
		}

		fail.Store(false)
		if _, err := client.Quote(context.Background(), target, map[string]string{}); err != nil {
			t.Fatalf("half-open probe: %v", err)
		}
		if client.StateName(target.Name) != "closed" {
			t.Fatalf("state %q after one successful probe, want closed", client.StateName(target.Name))
		}

		fail.Store(true)
		for range settings.ConsecutiveFailures {
			_, _ = client.Quote(context.Background(), target, map[string]string{})
		}
		time.Sleep(settings.OpenFor + time.Nanosecond)
		if client.StateName(target.Name) != "half-open" {
			t.Fatalf("state %q, want half-open", client.StateName(target.Name))
		}
		before := calls.Load()
		if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
			t.Fatal("failed probe was treated as success")
		}
		if calls.Load() != before+1 {
			t.Fatal("half-open did not let the single probe through")
		}
		if client.StateName(target.Name) != "open" {
			t.Fatalf("state %q after a failed probe, want open", client.StateName(target.Name))
		}
		if _, err := client.Quote(context.Background(), target, map[string]string{}); err == nil {
			t.Fatal("reopened circuit returned a quote")
		}
		if calls.Load() != before+1 {
			t.Fatal("reopened circuit called the partner")
		}
	})
}

func TestOnePartnerOpeningLeavesTheOthersClosed(t *testing.T) {
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "flaky") {
			return partnerResponse(r, http.StatusServiceUnavailable, `{"error":"partner unavailable"}`), nil
		}
		return partnerResponse(r, http.StatusOK, quoteBody), nil
	})
	client := newClient(platform.DefaultResilience(), transport)
	flaky := platform.Partner{Name: "partner-flaky", BaseURL: "http://partner-flaky"}
	slow := platform.Partner{Name: "partner-slow", BaseURL: "http://partner-slow"}

	for range platform.DefaultResilience().ConsecutiveFailures {
		_, _ = client.Quote(context.Background(), flaky, map[string]string{})
	}
	if client.StateName(flaky.Name) != "open" {
		t.Fatalf("flaky state %q, want open", client.StateName(flaky.Name))
	}
	if _, err := client.Quote(context.Background(), slow, map[string]string{}); err != nil {
		t.Fatalf("slow partner: %v", err)
	}
	if client.StateName(slow.Name) != "closed" {
		t.Fatalf("slow state %q, want closed", client.StateName(slow.Name))
	}
}

const quoteBody = `{"quote_id":"q-1","premium_cents":100,"currency":"BRL","coverage_cents":1000,"valid_for_seconds":300}`

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func partnerResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

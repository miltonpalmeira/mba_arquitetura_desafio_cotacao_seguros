package quotation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

const TenantHeader = "X-Tenant-Id"

const requestLimit = 1 << 20

const (
	messagePartnerUnavailable = "partner insurer unavailable"
	messageNoQuote            = "no_quote"
	// retryAfterSeconds is the breaker open time in the SAD: 30 s.
	retryAfterSeconds = 30
)

// noQuoteError means every partner missed and there is no premium to show.
// partners follows the configured order.
type noQuoteError struct {
	partners []string
}

func (e *noQuoteError) Error() string { return messageNoQuote }

type noQuoteBody struct {
	Message             string   `json:"error"`
	UnavailablePartners []string `json:"unavailable_partners"`
	RetryAfterSeconds   int      `json:"retry_after_seconds"`
}

type API struct {
	service *Service
	tenants map[string]bool
}

func NewAPI(service *Service, tenants []string) *API {
	known := make(map[string]bool, len(tenants))
	for _, t := range tenants {
		known[t] = true
	}
	return &API{service: service, tenants: known}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /quotes", a.quote)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func (a *API) quote(w http.ResponseWriter, r *http.Request) {
	tenant := r.Header.Get(TenantHeader)
	if tenant == "" {
		platform.WriteError(w, http.StatusBadRequest, TenantHeader+" is required")
		return
	}

	if !a.tenants[tenant] {
		platform.WriteError(w, http.StatusForbidden, "broker not enabled on this platform")
		return
	}

	var request Request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, requestLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		platform.WriteError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := request.Normalize(); err != nil {
		platform.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	response, err := a.service.Quote(r.Context(), tenant, request)
	if err != nil {
		var missed *noQuoteError
		if errors.As(err, &missed) {
			partners := missed.partners
			if partners == nil {
				partners = []string{}
			}
			platform.WriteJSON(w, http.StatusServiceUnavailable, noQuoteBody{
				Message:             messageNoQuote,
				UnavailablePartners: partners,
				RetryAfterSeconds:   retryAfterSeconds,
			})
			return
		}
		a.respondPartnerFailure(w, err)
		return
	}

	w.Header().Set("X-Tenant-Id", tenant)
	platform.WriteJSON(w, http.StatusOK, response)
}

func (a *API) respondPartnerFailure(w http.ResponseWriter, err error) {
	var failure *partner.Error
	if errors.As(err, &failure) {
		platform.WriteJSON(w, http.StatusBadGateway, platform.ErrorBody{
			Message: messagePartnerUnavailable,
			Partner: failure.Partner,
		})
		return
	}
	platform.WriteError(w, http.StatusBadGateway, "failed to query the partner insurers")
}

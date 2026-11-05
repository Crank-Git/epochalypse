package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

type capsuleAPI struct {
	store   *capsule.Store
	now     func() time.Time
	slots   chan struct{}
	timeout time.Duration
	beacon  capsule.BeaconSource
}

func (api *capsuleAPI) routes(mux *http.ServeMux, owners *owner.Store, lab bool) {
	api.ownerControls(mux, owners, lab)
	api.publicControls(mux, lab)
	for _, route := range []struct {
		path, method string
		serve        http.HandlerFunc
	}{
		{"/api/v1/capsules", "POST", api.create},
		{"/api/v1/capsules/prepared", "POST", api.importPrepared},
		{"/api/v1/capsules/{id}", "GET", api.metadata},
		{"/api/v1/capsules/{id}/publish", "POST", api.publish},
	} {
		mux.Handle(route.path, owner.SecureHandler(lab, owners.RequireScope("capsule:write", api.route(route.method, route.serve))))
	}
	mux.Handle("/api/v1/public/capsules/{id}/key", owner.SecureHandler(lab, api.route("GET", api.key)))
}

func (api *capsuleAPI) route(method string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			capsuleError(w, 405, "method_not_allowed", "Use "+method+".")
			return
		}
		if r.URL.RawQuery != "" || (method == "GET" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0)) {
			capsuleRequestError(w, capsule.ErrInput)
			return
		}
		api.work(w, r, next)
	})
}

func (api *capsuleAPI) work(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if api.store == nil || api.slots == nil || api.timeout <= 0 {
		capsuleDomainError(w, capsule.ErrUnavailable)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	select {
	case api.slots <- struct{}{}:
		defer func() { <-api.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		capsuleError(w, 429, "too_many_requests", "Capsule work is busy. Try again.")
		return
	}
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(api.timeout)
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		capsuleDomainError(w, err)
		return
	}
	if err := controller.SetWriteDeadline(deadline.Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		capsuleDomainError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), api.timeout)
	defer cancel()
	next(w, r.WithContext(ctx))
}

func (api *capsuleAPI) create(w http.ResponseWriter, r *http.Request) {
	input, content, err := capsuleBody(w, r, api.store.PlaintextLimit())
	if err != nil {
		capsuleRequestError(w, err)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	record, err := api.store.Create(input, bytes.NewReader(content), api.now())
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/capsules/"+record.ID)
	capsuleJSON(w, 201, record.Manifest)
}

func (api *capsuleAPI) metadata(w http.ResponseWriter, r *http.Request) {
	record, err := api.store.Get(r.PathValue("id"))
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	capsuleJSON(w, 200, record.Manifest)
}

func (api *capsuleAPI) importPrepared(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		capsuleRequestError(w, capsule.ErrInput)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	record, err := api.store.ImportPrepared(r.Context(), r.Body, api.now())
	if err != nil {
		if r.Context().Err() != nil {
			capsuleRequestError(w, r.Context().Err())
		} else {
			capsuleDomainError(w, err)
		}
		return
	}
	w.Header().Set("Location", "/api/v1/capsules/"+record.ID)
	capsuleJSON(w, 201, record.Manifest)
}

func (api *capsuleAPI) publish(w http.ResponseWriter, r *http.Request) {
	if err := capsuleEmptyBody(w, r); err != nil {
		capsuleRequestError(w, err)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	record, err := api.store.Publish(r.PathValue("id"), api.now())
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	capsuleJSON(w, 200, record.Manifest)
}

func (api *capsuleAPI) key(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	key, err := api.store.Key(id, api.now())
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	capsuleJSON(w, 200, struct {
		Version int    `json:"version"`
		ID      string `json:"id"`
		Key     string `json:"key"`
	}{capsule.FormatVersion, id, base64.RawURLEncoding.EncodeToString(key[:])})
}

func capsuleJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func capsuleError(w http.ResponseWriter, status int, code, message string) {
	owner.APIError(w, status, code, message, map[string]string{})
}

func capsuleRequestError(w http.ResponseWriter, err error) {
	var size *http.MaxBytesError
	var timeout net.Error
	switch {
	case errors.Is(err, capsule.ErrLimit), errors.As(err, &size):
		capsuleError(w, 413, "request_too_large", "The capsule request exceeds its byte limit.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		capsuleError(w, 408, "request_timeout", "The capsule request times out.")
	case errors.As(err, &timeout) && timeout.Timeout():
		capsuleError(w, 408, "request_timeout", "The capsule request times out.")
	default:
		capsuleError(w, 400, "invalid_request", "Provide valid capsule metadata and bounded content.")
	}
}

func capsuleDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, capsule.ErrInput), errors.Is(err, capsule.ErrUnsupportedMode):
		capsuleRequestError(w, err)
	case errors.Is(err, capsule.ErrLimit):
		capsuleRequestError(w, err)
	case errors.Is(err, capsule.ErrNotFound):
		capsuleError(w, 404, "not_found", "The capsule does not exist.")
	case errors.Is(err, capsule.ErrTooEarly):
		capsuleError(w, 425, "too_early", "The capsule key is not released.")
	case errors.Is(err, installation.ErrQuota):
		capsuleError(w, 409, "quota_exceeded", "The retained state quota is full.")
	case errors.Is(err, capsule.ErrIntegrity):
		capsuleError(w, 503, "integrity_error", "Capsule integrity verification fails.")
	default:
		capsuleError(w, 503, "unavailable", "Capsule content or its key is unavailable.")
	}
}

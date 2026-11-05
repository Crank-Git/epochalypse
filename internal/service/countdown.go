package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

type countdownAPI struct {
	store *countdown.Store
	now   func() time.Time
	slots chan struct{}
}

func (api *countdownAPI) routes(mux *http.ServeMux, owners *owner.Store, lab bool) {
	api.ownerControls(mux, owners, lab)
	for _, path := range []string{"/api/v1/countdowns", "/api/v1/countdowns/{id}"} {
		mux.Handle(path, owner.SecureHandler(lab, owners.RequireScope("countdown:write", http.HandlerFunc(api.serve))))
	}
}

func (api *countdownAPI) serve(w http.ResponseWriter, r *http.Request) {
	method := "POST"
	if r.PathValue("id") != "" {
		method = "GET, DELETE"
	}
	if (method == "POST" && r.Method != "POST") || (method != "POST" && r.Method != "GET" && r.Method != "DELETE") {
		w.Header().Set("Allow", method)
		countdownError(w, 405, "method_not_allowed", "Use "+method+".")
		return
	}
	if r.URL.RawQuery != "" {
		countdownDomainError(w, countdown.ErrInput)
		return
	}
	if api.store == nil || api.slots == nil {
		countdownDomainError(w, countdown.ErrUnavailable)
		return
	}
	select {
	case api.slots <- struct{}{}:
		defer func() { <-api.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		countdownError(w, 429, "too_many_requests", "Countdown work is busy. Try again.")
		return
	}
	api.work(w, r)
}

func (api *countdownAPI) work(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(10 * time.Second)
	for _, err := range []error{controller.SetReadDeadline(deadline), controller.SetWriteDeadline(deadline.Add(5 * time.Second))} {
		if err != nil && !errors.Is(err, http.ErrNotSupported) {
			countdownDomainError(w, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if err := ctx.Err(); err != nil {
		countdownDomainError(w, err)
		return
	}
	if r.Method == "POST" {
		api.create(w, r)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(data) != 0 {
		countdownDomainError(w, countdown.ErrInput)
		return
	}
	if r.Method == "DELETE" {
		if err := api.store.Delete(ctx, r.PathValue("id")); err != nil {
			countdownDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	record, err := api.store.Get(r.PathValue("id"))
	if err != nil {
		countdownDomainError(w, err)
		return
	}
	countdownJSON(w, 200, record)
}

func (api *countdownAPI) create(w http.ResponseWriter, r *http.Request) {
	input, err := countdownBody(w, r)
	if err != nil {
		countdownDomainError(w, err)
		return
	}
	record, err := api.store.Create(r.Context(), input, api.now())
	if err != nil {
		countdownDomainError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/countdowns/"+record.ID)
	countdownJSON(w, 201, record)
}

func countdownBody(w http.ResponseWriter, r *http.Request) (countdown.Input, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return countdown.Input{}, countdown.ErrInput
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		return countdown.Input{}, err
	}
	if !utf8.Valid(data) {
		return countdown.Input{}, countdown.ErrInput
	}
	var input countdown.Input
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return countdown.Input{}, countdown.ErrInput
	}
	return input, nil
}

func countdownJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func countdownError(w http.ResponseWriter, status int, code, message string) {
	owner.APIError(w, status, code, message, map[string]string{})
}

func countdownDomainError(w http.ResponseWriter, err error) {
	var size *http.MaxBytesError
	switch {
	case errors.Is(err, countdown.ErrInput):
		countdownError(w, 400, "invalid_request", "Provide a title and a future event_at in whole UTC seconds within 2147483647 seconds.")
	case errors.As(err, &size):
		countdownError(w, 413, "request_too_large", "The countdown request exceeds 4096 bytes.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		countdownError(w, 408, "request_timeout", "The countdown request times out.")
	case errors.Is(err, countdown.ErrNotFound):
		countdownError(w, 404, "not_found", "The countdown does not exist.")
	case errors.Is(err, installation.ErrQuota):
		countdownError(w, 409, "quota_exceeded", "The retained state quota is full.")
	default:
		countdownError(w, 503, "unavailable", "The countdown service is unavailable.")
	}
}

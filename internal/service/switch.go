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

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/switches"
)

type switchAPI struct {
	store *switches.Store
	now   func() time.Time
	slots chan struct{}
}

func (api *switchAPI) routes(mux *http.ServeMux, owners *owner.Store, lab bool) {
	api.ownerControls(mux, owners, lab)
	for _, path := range []string{"/api/v1/switches", "/api/v1/switches/{id}"} {
		mux.Handle(path, owner.SecureHandler(lab, owners.RequireScope("switch:write", http.HandlerFunc(api.serve))))
	}
	mux.Handle("/api/v1/switches/{id}/arm", owner.SecureHandler(lab, owners.RequireScope("switch:write", http.HandlerFunc(api.armEvidence))))
}

func (api *switchAPI) armEvidence(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		switchDomainError(w, err)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		switchError(w, 405, "method_not_allowed", "Use POST.")
		return
	}
	if api.store == nil || api.slots == nil {
		switchDomainError(w, switches.ErrUnavailable)
		return
	}
	select {
	case api.slots <- struct{}{}:
		defer func() { <-api.slots }()
	default:
		switchError(w, 429, "too_many_requests", "Switch work is busy. Try again.")
		return
	}
	evidence, err := switchArmBody(w, r)
	if err != nil {
		switchDomainError(w, err)
		return
	}
	record, err := api.store.ObserveArm(r.Context(), r.PathValue("id"), evidence)
	if err != nil {
		switchDomainError(w, err)
		return
	}
	status := http.StatusAccepted
	if record.State == switches.StateArmed {
		status = http.StatusOK
	}
	switchJSON(w, status, record)
}

func switchArmBody(w http.ResponseWriter, r *http.Request) (switches.ArmEvidence, error) {
	var evidence switches.ArmEvidence
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || r.URL.RawQuery != "" {
		return evidence, switches.ErrInput
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, switches.MaxRecordBytes))
	if err != nil {
		return evidence, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !utf8.Valid(data) || decoder.Decode(&evidence) != nil || decoder.Decode(&struct{}{}) != io.EOF || evidence.OwnerKey == [32]byte{} {
		return switches.ArmEvidence{}, switches.ErrInput
	}
	return evidence, nil
}

func (api *switchAPI) serve(w http.ResponseWriter, r *http.Request) {
	method := "POST"
	if r.PathValue("id") != "" {
		method = "GET"
	}
	if (method == "POST" && r.Method != "POST") || (method != "POST" && r.Method != "GET") {
		w.Header().Set("Allow", method)
		switchError(w, 405, "method_not_allowed", "Use "+method+".")
		return
	}
	if r.URL.RawQuery != "" {
		switchDomainError(w, switches.ErrInput)
		return
	}
	if api.store == nil || api.slots == nil {
		switchDomainError(w, switches.ErrUnavailable)
		return
	}
	select {
	case api.slots <- struct{}{}:
		defer func() { <-api.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		switchError(w, 429, "too_many_requests", "Switch work is busy. Try again.")
		return
	}
	api.work(w, r)
}

func (api *switchAPI) work(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(10 * time.Second)
	for _, err := range []error{controller.SetReadDeadline(deadline), controller.SetWriteDeadline(deadline.Add(5 * time.Second))} {
		if err != nil && !errors.Is(err, http.ErrNotSupported) {
			switchDomainError(w, err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if err := ctx.Err(); err != nil {
		switchDomainError(w, err)
		return
	}
	if r.Method == "POST" {
		api.create(w, r)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
	if err != nil || len(data) != 0 {
		switchDomainError(w, switches.ErrInput)
		return
	}
	record, err := api.store.Get(r.PathValue("id"))
	if err != nil {
		switchDomainError(w, err)
		return
	}
	switchJSON(w, 200, record)
}

func (api *switchAPI) create(w http.ResponseWriter, r *http.Request) {
	input, err := switchBody(w, r)
	if err != nil {
		switchDomainError(w, err)
		return
	}
	record, err := api.store.Create(r.Context(), input, api.now())
	if err != nil {
		switchDomainError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/switches/"+record.ID)
	switchJSON(w, 201, record)
}

func switchBody(w http.ResponseWriter, r *http.Request) (switches.Input, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return switches.Input{}, switches.ErrInput
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		return switches.Input{}, err
	}
	if !utf8.Valid(data) {
		return switches.Input{}, switches.ErrInput
	}
	var input switches.Input
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return input, switches.ErrInput
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return switches.Input{}, switches.ErrInput
	}
	return input, nil
}

func switchJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func switchError(w http.ResponseWriter, status int, code, message string) {
	owner.APIError(w, status, code, message, map[string]string{})
}

func switchDomainError(w http.ResponseWriter, err error) {
	var size *http.MaxBytesError
	switch {
	case errors.Is(err, switches.ErrInput):
		switchError(w, 400, "invalid_request", "Provide valid switch configuration or authenticated arm evidence.")
	case errors.As(err, &size):
		switchError(w, 413, "request_too_large", "The switch request exceeds its byte limit.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		switchError(w, 408, "request_timeout", "The switch request times out.")
	case errors.Is(err, switches.ErrNotFound):
		switchError(w, 404, "not_found", "The switch does not exist.")
	case errors.Is(err, installation.ErrQuota):
		switchError(w, 409, "quota_exceeded", "The retained state quota is full.")
	default:
		switchError(w, 503, "unavailable", "The switch service is unavailable.")
	}
}

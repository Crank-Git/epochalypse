package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func windowHandler(store *window.Store, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/windows", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			owner.APIError(w, 503, "unavailable", "Window service is unavailable.", map[string]string{})
			return
		}
		input, err := windowBody(w, r)
		if err != nil {
			windowRequestError(w, err)
			return
		}
		at := now()
		if err := input.Validate(at); err != nil {
			field := err.(*window.ValidationError)
			owner.APIError(w, 400, "invalid_window", field.Message, map[string]string{field.Field: field.Message})
			return
		}
		record, err := store.Create(input, at)
		if errors.Is(err, window.ErrQuota) {
			owner.APIError(w, 409, "quota_exceeded", "The retained state quota is full.", map[string]string{})
			return
		}
		if err != nil {
			owner.APIError(w, 503, "unavailable", "Window service is unavailable.", map[string]string{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(record)
	})
	return mux
}

func windowBody(w http.ResponseWriter, r *http.Request) (window.Input, error) {
	if r.ContentLength > 8192 {
		return window.Input{}, &http.MaxBytesError{Limit: 8192}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return window.Input{}, errors.New("use application/json")
	}
	var body struct {
		Content    *string `json:"content"`
		Inception  *string `json:"inception"`
		Expiration *string `json:"expiration"`
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return window.Input{}, err
	}
	if !utf8.Valid(data) {
		return window.Input{}, errors.New("use UTF-8 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return window.Input{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return window.Input{}, errors.New("provide one JSON document")
	}
	if body.Content == nil || body.Inception == nil || body.Expiration == nil {
		return window.Input{}, errors.New("provide content, inception, and expiration strings")
	}
	return window.Input{Content: *body.Content, Inception: *body.Inception, Expiration: *body.Expiration}, nil
}

func windowRequestError(w http.ResponseWriter, err error) {
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		owner.APIError(w, 413, "request_too_large", "The request exceeds 8192 bytes.", map[string]string{})
		return
	}
	owner.APIError(w, 400, "invalid_request", "Provide one JSON document with content, inception, and expiration strings.", map[string]string{})
}

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/resolver"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func (s *mainState) observationRoutes(mux *http.ServeMux, lab bool) {
	post := s.owner.RequireOwner(http.HandlerFunc(s.startObservation))
	mux.Handle("/api/v1/windows/{id}/observations", owner.SecureHandler(lab, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.listObservations(w, r)
		case http.MethodPost:
			post.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			owner.APIError(w, 405, "method_not_allowed", "Use GET or POST.", map[string]string{})
		}
	})))
}

func observationID(value string) bool {
	return len(value) == 32 && strings.Trim(value, "0123456789abcdef") == ""
}

func (s *mainState) startObservation(w http.ResponseWriter, r *http.Request) {
	if !observationID(r.PathValue("id")) {
		observationInvalid(w)
		return
	}
	if err := observationBody(w, r); err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			owner.APIError(w, 413, "request_too_large", "The request exceeds 1024 bytes.", map[string]string{})
			return
		}
		observationInvalid(w)
		return
	}
	if s.windows == nil || s.jobs == nil {
		observationUnavailable(w)
		return
	}
	run, err := s.jobs.Start(r.PathValue("id"))
	if err != nil {
		observationError(w, err)
		return
	}
	w.Header().Set("Location", r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(publicRun(run))
}

func observationBody(w http.ResponseWriter, r *http.Request) error {
	if r.ContentLength > 1024 {
		return &http.MaxBytesError{Limit: 1024}
	}
	if r.URL.RawQuery != "" {
		return errors.New("observation requests do not accept query parameters")
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		return err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || data[0] != '{' {
		return errors.New("provide an empty JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&struct{}{}); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("provide one JSON object")
	}
	return nil
}

func (s *mainState) listObservations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	values, err := url.ParseQuery(r.URL.RawQuery)
	after := values.Get("after")
	if !observationID(id) || err != nil || len(values) > 1 || (len(values) == 1 && len(values["after"]) != 1) || (after != "" && !observationID(after)) {
		observationInvalid(w)
		return
	}
	if s.windows == nil || (s.jobs != nil && !s.jobs.Available()) {
		observationUnavailable(w)
		return
	}
	runs, next, err := s.windows.ListRuns(id, after)
	if err != nil {
		observationError(w, err)
		return
	}
	for i, run := range runs {
		runs[i] = publicRun(run)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Version          int               `json:"version"`
		Runs             []observation.Run `json:"runs"`
		NextCursor       string            `json:"next_cursor"`
		PollAfterSeconds int               `json:"poll_after_seconds"`
	}{1, runs, next, 1})
}

func publicRun(run observation.Run) observation.Run {
	run.Results = append([]observation.Result(nil), run.Results...)
	for i := range run.Results {
		run.Results[i].Resolver.Address = ""
	}
	return run
}

func observationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, resolver.ErrBusy):
		w.Header().Set("Retry-After", "1")
		owner.APIError(w, 429, "busy", "Resolver jobs are busy.", map[string]string{})
	case errors.Is(err, window.ErrQuota):
		owner.APIError(w, 409, "quota_exceeded", "The retained state quota is full.", map[string]string{})
	case errors.Is(err, window.ErrNotFound):
		owner.APIError(w, 404, "not_found", "The window does not exist.", map[string]string{})
	default:
		observationUnavailable(w)
	}
}

func observationInvalid(w http.ResponseWriter) {
	owner.APIError(w, 400, "invalid_request", "Use a window identifier, an empty POST body, or a valid GET cursor.", map[string]string{})
}
func observationUnavailable(w http.ResponseWriter) {
	owner.APIError(w, 503, "unavailable", "Observation service is unavailable.", map[string]string{})
}

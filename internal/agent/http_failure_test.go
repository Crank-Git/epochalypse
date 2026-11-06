package agent

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransitionHTTPRejectsInvalidBodiesBeforeMutation(t *testing.T) {
	for name, input := range map[string]struct {
		method, path, body string
		status             int
	}{
		"method":        {"GET", "/agent/v1/switches/example/prepare", "", 405},
		"operation":     {"POST", "/agent/v1/switches/example/renew", "{}", 404},
		"invalid JSON":  {"POST", "/agent/v1/switches/example/prepare", "{", 400},
		"unknown field": {"POST", "/agent/v1/switches/example/prepare", `{"unknown":true}`, 400},
		"trailing JSON": {"POST", "/agent/v1/switches/example/prepare", "{}{}", 400},
		"oversized":     {"POST", "/agent/v1/switches/example/prepare", strings.Repeat("x", maxTransitionBody+1), 413},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			config := fixture.config(0)
			clockCalls := 0
			config.ReadClock = func() (Reading, error) { clockCalls++; return Reading{}, ErrClock }
			transitions, err := NewTransitions(config)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			transitions.Handler().ServeHTTP(response, httptest.NewRequest(input.method, input.path, strings.NewReader(input.body)))
			state, err := fixture.journal.Snapshot()
			if err != nil || response.Code != input.status || state.Generation != 0 || clockCalls != 0 {
				t.Fatalf("invalid HTTP reaches authority: status=%d generation=%d clock=%d error=%v", response.Code, state.Generation, clockCalls, err)
			}
		})
	}
}

func TestTransitionHTTPRejectsBodyIOFailureWithoutDisclosure(t *testing.T) {
	for _, stage := range []string{"missing", "read", "close"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newTransitionFixture(t)
			transitions, err := NewTransitions(fixture.config(0))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/agent/v1/switches/example/prepare", nil)
			request.Body = nil
			if stage != "missing" {
				request.Body = brokenTransitionBody{stage: stage}
			}
			response := httptest.NewRecorder()
			transitions.Handler().ServeHTTP(response, request)
			state, err := fixture.journal.Snapshot()
			if err != nil || response.Code != 400 || state.Generation != 0 || strings.Contains(response.Body.String(), "private") {
				t.Fatalf("body failure response=%d body=%q generation=%d error=%v", response.Code, response.Body.String(), state.Generation, err)
			}
		})
	}
}

type brokenTransitionBody struct{ stage string }

func (b brokenTransitionBody) Read(_ []byte) (int, error) {
	if b.stage == "read" {
		return 0, errors.New("private reader failure")
	}
	return 0, io.EOF
}
func (b brokenTransitionBody) Close() error {
	if b.stage == "close" {
		return errors.New("private close failure")
	}
	return nil
}

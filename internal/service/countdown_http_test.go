package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

func countdownServiceKey(t *testing.T) string {
	t.Helper()
	return serviceKeyForZone(t, "countdown.example.")
}

func newCountdownHTTP(t *testing.T) (capsuleHTTPFixture, string) {
	t.Helper()
	h := newCapsuleHTTP(t)
	grant, err := h.state.owner.CreateToken(owner.TokenRequest{Name: "Synthetic countdown writer", Scopes: []string{"countdown:write"}, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.state.routes(mux, false)
	h.handler = mux
	return h, grant.Token
}

func countdownRequest(method, path, body, token string) *http.Request {
	r := httptest.NewRequest(method, "https://localhost"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func TestCountdownHTTPRetainsMetadataWithoutDNS(t *testing.T) {
	h, token := newCountdownHTTP(t)
	w := h.send(countdownRequest("GET", "/api/v1/countdowns/"+strings.Repeat("a", 32), "", token))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"not_found"`) {
		t.Fatalf("the disabled DNS configuration omits countdown inspection: %d %s", w.Code, w.Body)
	}
}

func TestCountdownHTTPReturnsAbsoluteEvent(t *testing.T) {
	config := Config{StateDir: t.TempDir(), CountdownZone: "countdown.example.", CountdownKeyFile: countdownServiceKey(t), Environment: "lab"}
	state, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.close)
	if err := state.owner.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	grant, err := state.owner.CreateToken(owner.TokenRequest{Name: "Synthetic countdown writer", Scopes: []string{"countdown:write"}, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	state.routes(mux, false)
	event := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	data, _ := json.Marshal(countdown.Input{Title: "Synthetic event", EventAt: event.Format(time.RFC3339)})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, countdownRequest("POST", "/api/v1/countdowns", string(data), grant.Token))
	if w.Code != 201 {
		t.Fatalf("creation fails: %d %s", w.Code, w.Body)
	}
	var record countdown.Record
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil || !record.EventAt.Equal(event) || record.Name != "c"+record.ID+".countdown.example." {
		t.Fatalf("creation changes the event or name: %+v %v", record, err)
	}
	for _, step := range []struct {
		method string
		status int
	}{{"GET", 200}, {"DELETE", 204}, {"GET", 404}} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, countdownRequest(step.method, "/api/v1/countdowns/"+record.ID, "", grant.Token))
		if response.Code != step.status {
			t.Fatalf("%s fails: %d %s", step.method, response.Code, response.Body)
		}
	}
}

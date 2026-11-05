package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func serviceKey(t *testing.T) string {
	t.Helper()
	return serviceKeyForZone(t, "windows.example.")
}

func serviceKeyForZone(t *testing.T, zone string) string {
	t.Helper()
	key := dns.NewDNSKEY(zone, dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	for name, text := range map[string]string{path: key.PrivateKeyString(private), strings.TrimSuffix(path, ".private") + ".key": key.String()} {
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

type windowHTTP struct {
	state       *mainState
	handler     http.Handler
	token       string
	proof       string
	proofCookie *http.Cookie
	now         time.Time
}

func newWindowHTTP(t *testing.T, limits ...window.Limits) windowHTTP {
	t.Helper()
	config := Config{StateDir: t.TempDir(), Zone: "windows.example.", KeyFile: serviceKey(t)}
	if len(limits) != 0 {
		config.WindowLimits = &limits[0]
	}
	state, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.close)
	if err := state.owner.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	token, err := state.owner.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	proofResponse := httptest.NewRecorder()
	state.owner.Handler(false).ServeHTTP(proofResponse, httptest.NewRequest("GET", "https://localhost/api/v1/csrf", nil))
	var proof struct {
		Token string `json:"csrf_token"`
	}
	if err := json.Unmarshal(proofResponse.Body.Bytes(), &proof); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	handler := owner.SecureHandler(false, state.owner.RequireOwner(windowHandler(state.windows, func() time.Time { return at })))
	return windowHTTP{state, handler, token, proof.Token, proofResponse.Result().Cookies()[0], at}
}

func (h windowHTTP) request(body string) *http.Request {
	r := httptest.NewRequest("POST", "https://localhost/api/v1/windows", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", h.proof)
	r.AddCookie(h.proofCookie)
	r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.token, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return r
}

const windowJSON = `{"content":"public HTTP experiment","inception":"2026-11-04T13:00:00Z","expiration":"2026-11-04T14:00:00Z"}`

func TestWindowHTTPPersistsExplicitSignedRecords(t *testing.T) {
	h := newWindowHTTP(t)
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, h.request(windowJSON))
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("creation omits shared safeguards")
	}
	var record window.Window
	if err := json.Unmarshal(w.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	stored, err := h.state.windows.Get(record.ID)
	if err != nil || stored != record {
		t.Fatalf("response differs from durable record: %v", err)
	}
	_, sig, _, err := record.Records()
	if err != nil {
		t.Fatal(err)
	}
	if sig.ValidPeriod(h.now) || !sig.ValidPeriod(record.Inception) {
		t.Fatal("HTTP creation changes explicit validity")
	}
}

func TestWindowHTTPRejectsAuthorizationAndCSRFFailures(t *testing.T) {
	h := newWindowHTTP(t)
	for name, change := range map[string]struct {
		status int
		change func(*http.Request)
	}{
		"TLS":        {426, func(r *http.Request) { r.TLS = nil }},
		"session":    {401, func(r *http.Request) { r.Header.Del("Cookie") }},
		"bearer":     {401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer read-token") }},
		"CSRF":       {403, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }},
		"wrong CSRF": {403, func(r *http.Request) { r.Header.Set("X-CSRF-Token", strings.Repeat("x", 43)) }},
		"origin":     {403, func(r *http.Request) { r.Header.Set("Origin", "https://other.example") }},
		"cross-site": {403, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
	} {
		t.Run(name, func(t *testing.T) {
			r := h.request(windowJSON)
			change.change(r)
			w := httptest.NewRecorder()
			h.handler.ServeHTTP(w, r)
			if w.Code != change.status {
				t.Fatalf("status = %d: %s", w.Code, w.Body)
			}
			if !strings.Contains(w.Body.String(), `"request_id":"`+w.Header().Get("X-Request-ID")) {
				t.Fatal("error omits the request identifier")
			}
		})
	}
	all, err := h.state.windows.List()
	if err != nil || len(all) != 0 {
		t.Fatal("rejected authorization changes state")
	}
}

func TestWindowHTTPRejectsInvalidBodies(t *testing.T) {
	h := newWindowHTTP(t)
	for _, body := range []string{`{}`, `null`, `[]`, `bad`, windowJSON + ` {}`, strings.Replace(windowJSON, `"content":"public HTTP experiment"`, `"content":null`, 1), strings.Replace(windowJSON, `"content":`, `"unknown":`, 1), string([]byte{'{', 255, '}'}), strings.Replace(windowJSON, "public HTTP experiment", strings.Repeat("x", 513), 1)} {
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, h.request(body))
		if w.Code != 400 {
			t.Fatalf("invalid body status = %d", w.Code)
		}
	}
	for _, length := range []int64{8193, -1} {
		r := h.request(strings.Repeat("x", 8193))
		r.ContentLength = length
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		if w.Code != 413 {
			t.Fatalf("large body status = %d", w.Code)
		}
	}
	r := h.request(windowJSON)
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("non-JSON body passes")
	}
	all, err := h.state.windows.List()
	if err != nil || len(all) != 0 {
		t.Fatal("invalid body changes state")
	}
}

func TestWindowHTTPNamesTheInvalidField(t *testing.T) {
	h := newWindowHTTP(t)
	for old, change := range map[string]struct{ value, field string }{"public HTTP experiment": {strings.Repeat("x", 513), "content"}, "2026-11-04T13:00:00Z": {"tomorrow", "inception"}, "2026-11-04T14:00:00Z": {"2026-11-04T12:00:00Z", "expiration"}} {
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, h.request(strings.Replace(windowJSON, old, change.value, 1)))
		var body struct {
			Error struct {
				Fields map[string]string `json:"fields"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 400 || len(body.Error.Fields) != 1 || body.Error.Fields[change.field] == "" {
			t.Fatal("invalid field lacks a specific error")
		}
	}
}

func TestWindowHTTPRejectsARevokedOwnerSession(t *testing.T) {
	h := newWindowHTTP(t)
	if err := h.state.owner.SignOut(h.token); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, h.request(windowJSON))
	if w.Code != 401 {
		t.Fatal("revoked session creates a window")
	}
	all, err := h.state.windows.List()
	if err != nil || len(all) != 0 {
		t.Fatal("revoked session changes state")
	}
}

func TestWindowHTTPReportsUnavailableState(t *testing.T) {
	h := newWindowHTTP(t)
	_, err := h.state.windows.Create(window.Input{Content: strings.Repeat("x", 512), Inception: "2026-11-04T13:00:00Z", Expiration: "2026-11-04T14:00:00Z"}, h.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.state.windows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, store := range []*window.Store{nil, h.state.windows} {
		handler := owner.SecureHandler(false, h.state.owner.RequireOwner(windowHandler(store, func() time.Time { return h.now })))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, h.request(windowJSON))
		if w.Code != 503 || strings.Contains(w.Body.String(), "public HTTP experiment") {
			t.Fatal("unavailable state exposes content or returns success")
		}
	}
}

func TestWindowHTTPRejectsCreationAtTheQuota(t *testing.T) {
	h := newWindowHTTP(t, window.Limits{MaxObjects: 1, MaxBytes: 1 << 30})
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, h.request(windowJSON))
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.handler.ServeHTTP(w, h.request(windowJSON))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "quota_exceeded") {
		t.Fatalf("quota: %d %s", w.Code, w.Body)
	}
	all, err := h.state.windows.List()
	if err != nil || len(all) != 1 {
		t.Fatal("quota rejection changes accepted state")
	}
}

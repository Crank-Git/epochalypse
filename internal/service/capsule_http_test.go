package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

type capsuleHTTPFixture struct {
	state       *mainState
	store       *capsule.Store
	api         *capsuleAPI
	handler     http.Handler
	clock       *time.Time
	session     string
	proof       string
	proofCookie *http.Cookie
	bearer      string
}

func newCapsuleHTTP(t *testing.T) capsuleHTTPFixture {
	t.Helper()
	state, err := openMainState(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(state.close)
	store, err := capsule.Attach(state.content, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := state.owner.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	session, err := state.owner.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	grant, err := state.owner.CreateToken(owner.TokenRequest{Name: "Synthetic capsule writer", Scopes: []string{"capsule:write"}, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	state.owner.Handler(false).ServeHTTP(response, httptest.NewRequest("GET", "https://localhost/api/v1/csrf", nil))
	var proof struct {
		Token string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &proof); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	api := &capsuleAPI{store: store, now: func() time.Time { return clock }, slots: make(chan struct{}, 4), timeout: 10 * time.Second}
	mux := http.NewServeMux()
	api.routes(mux, state.owner, false)
	return capsuleHTTPFixture{state, store, api, mux, &clock, session, proof.Token, response.Result().Cookies()[0], grant.Token}
}

func (h capsuleHTTPFixture) upload(t *testing.T, mode, content string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range [][2]string{{"mode", mode}, {"kind", "text"}, {"title", "Synthetic capsule"}, {"release_at", h.clock.Add(time.Hour).Format(time.RFC3339)}} {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteField("content", content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://localhost/api/v1/capsules", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+h.bearer)
	return r
}

func (h capsuleHTTPFixture) send(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w
}

func createHTTPCapsule(t *testing.T, h capsuleHTTPFixture, mode string) capsule.Manifest {
	t.Helper()
	w := h.send(h.upload(t, mode, "1234"))
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), `"object":`) || strings.Contains(w.Body.String(), `"key":`) || strings.Contains(w.Body.String(), `"content":`) || strings.Contains(w.Body.String(), `"plaintext":`) {
		t.Fatal("creation exposes an object, key, or plaintext")
	}
	var manifest capsule.Manifest
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func (h capsuleHTTPFixture) publish(id string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "https://localhost/api/v1/capsules/"+id+"/publish", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+h.bearer)
	return h.send(r)
}

func (h capsuleHTTPFixture) key(id string) *httptest.ResponseRecorder {
	return h.send(httptest.NewRequest("GET", "https://localhost/api/v1/public/capsules/"+id+"/key", nil))
}

func TestCapsuleHTTPCreatesAndPublishesTimelockCeilingRound(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "tlock")
	_, effective, err := capsule.SelectRound(draft.RequestedReleaseAt)
	if err != nil || draft.Mode != capsule.Tlock || draft.EffectiveReleaseAt == nil || !draft.EffectiveReleaseAt.Equal(effective) {
		t.Fatalf("release metadata: %+v %v", draft, err)
	}
	if w := h.publish(draft.ID); w.Code != 200 {
		t.Fatalf("publication: %d %s", w.Code, w.Body)
	}
	bundle, err := h.store.Bundle(draft.ID, *h.clock)
	if err != nil || bundle.Record.Timelock == nil || bundle.ReleasedKey != "" {
		t.Fatalf("recovery metadata: %+v %v", bundle, err)
	}
	if w := h.key(draft.ID); w.Code != 425 {
		t.Fatalf("early key: %d %s", w.Code, w.Body)
	}
}

func TestCapsuleUploadRejectsContentBeyondConfiguredLimit(t *testing.T) {
	h := newCapsuleHTTP(t)
	w := h.send(h.upload(t, "server-held", "12345"))
	if w.Code != 413 || strings.Contains(w.Body.String(), "12345") {
		t.Fatalf("oversize: %d %s", w.Code, w.Body)
	}
	manifest := createHTTPCapsule(t, h, "server-held")
	if manifest.PlaintextBytes != 4 {
		t.Fatal("the exact limit changes content size")
	}
}

func TestCapsuleKeyHTTPHidesUnpublishedDraft(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	*h.clock = manifest.RequestedReleaseAt.Add(time.Hour)
	if w := h.key(manifest.ID); w.Code != 404 || strings.Contains(w.Body.String(), `"key":`) {
		t.Fatalf("draft key: %d %s", w.Code, w.Body)
	}
}

func TestCapsuleKeyHTTPUsesExactReleaseBoundary(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	if w := h.publish(manifest.ID); w.Code != 200 || strings.Contains(w.Body.String(), `"object":`) || strings.Contains(w.Body.String(), `"key":`) {
		t.Fatalf("publish: %d %s", w.Code, w.Body)
	}
	*h.clock = manifest.RequestedReleaseAt.Add(-time.Nanosecond)
	if w := h.key(manifest.ID); w.Code != 425 || strings.Contains(w.Body.String(), `"key":`) {
		t.Fatalf("early key: %d %s", w.Code, w.Body)
	}
	*h.clock = manifest.RequestedReleaseAt
	w := h.key(manifest.ID)
	if w.Code != 200 {
		t.Fatalf("released key: %d %s", w.Code, w.Body)
	}
	var response struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	key, err := base64.RawURLEncoding.Strict().DecodeString(response.Key)
	if err != nil || len(key) != 32 {
		t.Fatal("the response omits a 32-byte key")
	}
	record, err := h.store.Published(manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := capsule.OpenObject(record.ID, record.Version, record.Object, [32]byte(key), 4)
	if err != nil || string(plaintext) != "1234" {
		t.Fatalf("HTTP key cannot authenticate content: %v", err)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("the key response omits shared safeguards")
	}
}

func TestPublicCapsuleKeyHTTPReleasesAtPublication(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "public")
	if w := h.key(manifest.ID); w.Code != 404 {
		t.Fatalf("unpublished public key: %d", w.Code)
	}
	w := h.publish(manifest.ID)
	if w.Code != 200 {
		t.Fatalf("public publication: %d %s", w.Code, w.Body)
	}
	var published capsule.Manifest
	if err := json.Unmarshal(w.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published.EffectiveReleaseAt == nil || !published.EffectiveReleaseAt.Equal(*h.clock) || !published.RequestedReleaseAt.Equal(manifest.RequestedReleaseAt) {
		t.Fatal("public publication loses its advisory or effective time")
	}
	if w := h.key(manifest.ID); w.Code != 200 {
		t.Fatalf("public data remains withheld: %d %s", w.Code, w.Body)
	}
}

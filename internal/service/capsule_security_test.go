package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	bolt "go.etcd.io/bbolt"
)

func (h capsuleHTTPFixture) cookies(r *http.Request, proof bool) {
	r.Header.Del("Authorization")
	r.AddCookie(&http.Cookie{Name: "__Host-epochalypse-session", Value: h.session, Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	r.AddCookie(h.proofCookie)
	if proof {
		r.Header.Set("X-CSRF-Token", h.proof)
	}
}

func TestCapsuleMutationRequiresExactScopeOrOwnerProof(t *testing.T) {
	h := newCapsuleHTTP(t)
	read, err := h.state.owner.CreateToken(owner.TokenRequest{Name: "Synthetic summary reader", Scopes: []string{"read"}, ExpiresAt: time.Now().Add(time.Hour)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*http.Request)
		status int
	}{
		{"absent credentials", func(r *http.Request) { r.Header.Del("Authorization") }, 401},
		{"wrong scope", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+read.Token) }, 403},
		{"owner without proof", func(r *http.Request) { h.cookies(r, false) }, 403},
		{"owner with proof", func(r *http.Request) { h.cookies(r, true) }, 201},
		{"invalid bearer with owner", func(r *http.Request) { h.cookies(r, true); r.Header.Set("Authorization", "Bearer invalid") }, 401},
		{"duplicate bearer", func(r *http.Request) { r.Header.Add("Authorization", "Bearer "+h.bearer) }, 401},
		{"cross-origin owner", func(r *http.Request) { h.cookies(r, true); r.Header.Set("Origin", "https://foreign.invalid") }, 403},
		{"plaintext transport", func(r *http.Request) { r.TLS = nil }, 426},
		{"validated bearer without proof", func(*http.Request) {}, 201},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := h.upload(t, "server-held", "1234")
			test.change(r)
			before := capsuleHTTPCount(t, h)
			w := h.send(r)
			if w.Code != test.status {
				t.Fatalf("authorization: %d %s", w.Code, w.Body)
			}
			if test.status != 201 && capsuleHTTPCount(t, h) != before {
				t.Fatal("rejected authorization mutates content")
			}
		})
	}
}

func capsuleHTTPCount(t *testing.T, h capsuleHTTPFixture) int {
	t.Helper()
	count := 0
	if err := h.state.content.View(func(tx *bolt.Tx) error {
		count = tx.Bucket([]byte(installation.CapsulesBucket)).Stats().KeyN
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCapsuleMetadataRequiresWriteScopeAndOmitsObject(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	path := "https://localhost/api/v1/capsules/" + manifest.ID
	if w := h.send(httptest.NewRequest("GET", path, nil)); w.Code != 401 {
		t.Fatalf("anonymous draft metadata: %d", w.Code)
	}
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer "+h.bearer)
	w := h.send(r)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"object":`) || strings.Contains(w.Body.String(), `"key":`) {
		t.Fatalf("owner metadata: %d %s", w.Code, w.Body)
	}
}

func TestCapsulePublicationRequiresOwnerProof(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	r := httptest.NewRequest("POST", "https://localhost/api/v1/capsules/"+manifest.ID+"/publish", nil)
	h.cookies(r, false)
	if w := h.send(r); w.Code != 403 {
		t.Fatalf("publication without proof: %d", w.Code)
	}
	if _, err := h.store.Published(manifest.ID); !errors.Is(err, capsule.ErrNotFound) {
		t.Fatalf("rejected owner publication exposes the draft: %v", err)
	}
	r = httptest.NewRequest("POST", "https://localhost/api/v1/capsules/"+manifest.ID+"/publish", nil)
	h.cookies(r, true)
	if w := h.send(r); w.Code != 200 {
		t.Fatalf("verified owner publication: %d %s", w.Code, w.Body)
	}
}

func TestPublicCapsuleKeyRequiresTLS(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "public")
	if w := h.publish(manifest.ID); w.Code != 200 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "http://localhost/api/v1/public/capsules/"+manifest.ID+"/key", nil)
	if w := h.send(r); w.Code != 426 || strings.Contains(w.Body.String(), `"key":`) {
		t.Fatalf("public key over plaintext HTTP: %d %s", w.Code, w.Body)
	}
}

func TestCapsuleKeyHTTPDistinguishesUnavailableAndCorruptKeys(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	if w := h.publish(manifest.ID); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err := h.state.content.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.CapsuleKeysBucket)).Delete([]byte(manifest.ID))
	}); err != nil {
		t.Fatal(err)
	}
	if w := h.key(manifest.ID); w.Code != 425 {
		t.Fatalf("unavailable early key: %d", w.Code)
	}
	*h.clock = manifest.RequestedReleaseAt
	if w := h.key(manifest.ID); w.Code != 503 || strings.Contains(w.Body.String(), `"key":`) || !strings.Contains(w.Body.String(), `"unavailable"`) {
		t.Fatalf("unavailable eligible key: %d %s", w.Code, w.Body)
	}
	if err := h.state.content.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(installation.CapsuleKeysBucket)).Put([]byte(manifest.ID), make([]byte, 32))
	}); err != nil {
		t.Fatal(err)
	}
	if w := h.key(manifest.ID); w.Code != 503 || strings.Contains(w.Body.String(), `"key":`) || !strings.Contains(w.Body.String(), `"integrity_error"`) {
		t.Fatalf("corrupt eligible key: %d %s", w.Code, w.Body)
	}
}

func TestCapsuleHTTPReportsQuotaAndUnavailableState(t *testing.T) {
	h := newCapsuleHTTP(t)
	h.state.content.Limits.MaxObjects = 1
	createHTTPCapsule(t, h, "server-held")
	if w := h.send(h.upload(t, "server-held", "1234")); w.Code != 409 {
		t.Fatalf("installation quota: %d %s", w.Code, w.Body)
	}
	h.api.store = nil
	if w := h.send(h.upload(t, "server-held", "1234")); w.Code != 503 {
		t.Fatalf("unavailable store: %d", w.Code)
	}
}

func TestCapsuleWorkSlotsCoverEveryCryptoRoute(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "public")
	if w := h.publish(manifest.ID); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for range cap(h.api.slots) {
		h.api.slots <- struct{}{}
	}
	requests := []*http.Request{h.upload(t, "server-held", "1234"), httptest.NewRequest("GET", "https://localhost/api/v1/capsules/"+manifest.ID, nil), httptest.NewRequest("POST", "https://localhost/api/v1/capsules/"+manifest.ID+"/publish", nil), httptest.NewRequest("GET", "https://localhost/api/v1/public/capsules/"+manifest.ID+"/key", nil)}
	for _, request := range requests {
		request.Header.Set("Authorization", "Bearer "+h.bearer)
		w := h.send(request)
		if w.Code != 429 || w.Header().Get("Retry-After") != "1" || strings.Contains(w.Body.String(), `"key":`) {
			t.Fatalf("work limit: %d %s", w.Code, w.Body)
		}
	}
	for range cap(h.api.slots) {
		<-h.api.slots
	}
	if w := h.key(manifest.ID); w.Code != 200 || len(h.api.slots) != 0 {
		t.Fatalf("work capacity does not recover: %d", w.Code)
	}
}

func TestCapsuleCanceledRequestRetainsNoDraft(t *testing.T) {
	h := newCapsuleHTTP(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := h.upload(t, "server-held", "1234").WithContext(ctx)
	if w := h.send(r); w.Code != 408 {
		t.Fatalf("canceled request: %d %s", w.Code, w.Body)
	}
	if capsuleHTTPCount(t, h) != 0 || len(h.api.slots) != 0 {
		t.Fatal("cancellation retains content or a work slot")
	}
}

type capsuleDeadlineWriter struct {
	*httptest.ResponseRecorder
	readError, writeError error
}

func (w capsuleDeadlineWriter) SetReadDeadline(time.Time) error  { return w.readError }
func (w capsuleDeadlineWriter) SetWriteDeadline(time.Time) error { return w.writeError }

func TestCapsuleDeadlineFailureExcludesPrivateErrors(t *testing.T) {
	h := newCapsuleHTTP(t)
	private := errors.New("synthetic private deadline failure")
	for _, writer := range []capsuleDeadlineWriter{{httptest.NewRecorder(), private, nil}, {httptest.NewRecorder(), nil, private}} {
		h.handler.ServeHTTP(writer, h.upload(t, "server-held", "1234"))
		if writer.Code != 503 || strings.Contains(writer.Body.String(), private.Error()) {
			t.Fatalf("deadline failure: %d %s", writer.Code, writer.Body)
		}
	}
	if capsuleHTTPCount(t, h) != 0 || len(h.api.slots) != 0 {
		t.Fatal("deadline failure retains content or a work slot")
	}
}

func TestCapsuleSlowBodyUsesReadDeadline(t *testing.T) {
	h := newCapsuleHTTP(t)
	h.api.timeout = 30 * time.Millisecond
	mux := http.NewServeMux()
	h.api.routes(mux, h.state.owner, true)
	server := httptest.NewServer(mux)
	defer server.Close()
	connection, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(connection, "POST /api/v1/capsules HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer %s\r\nContent-Type: multipart/form-data; boundary=synthetic\r\nContent-Length: 100\r\n\r\n", h.bearer); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 408 {
		t.Fatalf("slow body returns %d", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if capsuleHTTPCount(t, h) != 0 || len(h.api.slots) != 0 {
		t.Fatal("a slow body retains content or a work slot")
	}
}

func TestCapsuleRoutesRejectQueriesBodiesAndWrongMethods(t *testing.T) {
	h := newCapsuleHTTP(t)
	for _, test := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/v1/capsules?unknown=1", "", 400},
		{"GET", "/api/v1/capsules", "", 405},
		{"POST", "/api/v1/public/capsules/0123456789abcdef0123456789abcdef/key", "", 405},
		{"GET", "/api/v1/public/capsules/0123456789abcdef0123456789abcdef/key", "body", 400},
		{"GET", "/api/v1/capsules/invalid", "", 400},
		{"GET", "/api/v1/public/capsules/invalid/key", "", 400},
	} {
		r := httptest.NewRequest(test.method, "https://localhost"+test.path, strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer "+h.bearer)
		if w := h.send(r); w.Code != test.status || !strings.Contains(w.Body.String(), `"request_id":`) {
			t.Fatalf("invalid request: %d %s", w.Code, w.Body)
		}
	}
}

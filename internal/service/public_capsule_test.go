package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func TestPublicCapsuleSeparatesReleaseAndAvailability(t *testing.T) {
	for _, mode := range []string{"public", "server-held", "tlock"} {
		t.Run(mode, func(t *testing.T) {
			h := newCapsuleHTTP(t)
			draft := createHTTPCapsule(t, h, mode)
			path := "/c/" + draft.ID
			if w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil)); w.Code != 404 {
				t.Fatalf("draft page: %d", w.Code)
			}
			if w := h.publish(draft.ID); w.Code != 200 {
				t.Fatal(w.Body)
			}
			w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil))
			for _, text := range []string{"Selected mode", mode, "Requested release", "Effective release", "Content availability", "Public copies remain outside"} {
				if w.Code != 200 || !strings.Contains(w.Body.String(), text) {
					t.Fatalf("missing %q: %d %s", text, w.Code, w.Body)
				}
			}
			if mode == "public" {
				if !strings.Contains(w.Body.String(), "Verified and available") {
					t.Fatal(w.Body)
				}
			} else {
				if !strings.Contains(w.Body.String(), "Sealed") || strings.Contains(w.Body.String(), ">Download content</a>") {
					t.Fatal(w.Body)
				}
			}
			*h.clock = h.clock.Add(2 * time.Hour)
			w = h.send(httptest.NewRequest("GET", "https://localhost"+path, nil))
			expected := "Verified and available"
			if mode == "tlock" {
				expected = "Delayed availability"
			}
			if !strings.Contains(w.Body.String(), expected) || !strings.Contains(w.Body.String(), "Release condition is due") {
				t.Fatal(w.Body)
			}
		})
	}
}

type capsuleFixtureBeacon struct {
	signature []byte
	state     atomic.Int32
}

func (b *capsuleFixtureBeacon) Fetch(_ context.Context, round uint64) ([]byte, error) {
	if round != 1000 || b.state.Load() == 1 {
		return nil, capsule.ErrUnavailable
	}
	if b.state.Load() == 2 {
		return make([]byte, 48), nil
	}
	return bytes.Clone(b.signature), nil
}

func publicCapsuleFixtures(t *testing.T) (capsuleHTTPFixture, map[string]string, *capsuleFixtureBeacon) {
	t.Helper()
	h := newCapsuleHTTP(t)
	h.store = h.state.capsules
	h.api.store = h.store
	*h.clock = time.Unix(1692806364, 0).UTC().Add(-time.Hour)
	data, err := os.ReadFile("../../tests/compatibility/tlock/testdata/quicknet-1000.json")
	if err != nil {
		t.Fatal(err)
	}
	var beacon struct{ Signature string }
	if err := json.Unmarshal(data, &beacon); err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(beacon.Signature)
	if err != nil {
		t.Fatal(err)
	}
	source := &capsuleFixtureBeacon{signature: signature}
	h.api.beacon = source
	ids := map[string]string{}
	for _, name := range []string{"public", "server-held", "tlock", "file", "preview", "corrupt", "lost"} {
		input := capsule.Input{Mode: capsule.Public, Kind: capsule.Text, Title: "<script>fixture</script>", ReleaseAt: h.clock.Add(time.Hour)}
		content := "<script>globalThis.uploadRan=true</script>"
		switch name {
		case "server-held", "tlock":
			input.Mode = capsule.Mode(name)
		case "file":
			input.Kind = capsule.File
			input.Filename = "active.html"
		case "preview":
			content = strings.Repeat("x", 4095) + "€ending"
		}
		record, err := h.store.Create(input, strings.NewReader(content), *h.clock)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.store.Publish(record.ID, *h.clock); err != nil {
			t.Fatal(err)
		}
		ids[name] = record.ID
	}
	return h, ids, source
}

func damageCapsuleKey(t *testing.T, h capsuleHTTPFixture, id string, missing bool) {
	t.Helper()
	if err := h.state.content.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(installation.CapsuleKeysBucket))
		if missing {
			return bucket.Delete([]byte(id))
		}
		return bucket.Put([]byte(id), make([]byte, 32))
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicCapsuleVerifiesBeaconAndWithholdsCorruptContent(t *testing.T) {
	h, ids, source := publicCapsuleFixtures(t)
	*h.clock = h.clock.Add(time.Hour)
	path := "https://localhost/api/v1/public/capsules/" + ids["tlock"] + "/download"
	for _, state := range []int32{0, 1, 2} {
		source.state.Store(state)
		w := h.send(httptest.NewRequest("GET", path, nil))
		if state == 0 {
			if w.Code != 200 || !strings.Contains(w.Body.String(), "uploadRan") {
				t.Fatalf("verified fixture: %d %s", w.Code, w.Body)
			}
		} else if w.Code != 503 || strings.Contains(w.Body.String(), "uploadRan") {
			t.Fatalf("failed beacon: %d %s", w.Code, w.Body)
		}
	}
	damageCapsuleKey(t, h, ids["corrupt"], false)
	damageCapsuleKey(t, h, ids["lost"], true)
	for _, name := range []string{"corrupt", "lost"} {
		w := h.send(httptest.NewRequest("GET", "https://localhost/c/"+ids[name], nil))
		want := "Integrity error"
		if name == "lost" {
			want = "Delayed availability"
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) || strings.Contains(w.Body.String(), `id="download"`) || strings.Contains(w.Body.String(), "uploadRan") {
			t.Fatalf("failed key page: %d %s", w.Code, w.Body)
		}
		w = h.send(httptest.NewRequest("GET", "https://localhost/api/v1/public/capsules/"+ids[name]+"/download", nil))
		if w.Code != 503 || strings.Contains(w.Body.String(), "uploadRan") {
			t.Fatalf("failed key download: %d %s", w.Code, w.Body)
		}
	}
}

func TestPublicCapsuleBoundsAndEscapesPreview(t *testing.T) {
	h, ids, _ := publicCapsuleFixtures(t)
	for _, name := range []string{"public", "file", "preview"} {
		w := h.send(httptest.NewRequest("GET", "https://localhost/c/"+ids[name], nil))
		body := w.Body.String()
		if w.Code != 200 || strings.Contains(body, "<script>") || strings.Contains(body, "€") || strings.Contains(body, "ending") {
			t.Fatalf("unsafe preview: %d %s", w.Code, w.Body)
		}
		if name == "file" && strings.Contains(body, `id="preview"`) {
			t.Fatal("file has a preview")
		}
		if name == "preview" && !strings.Contains(body, strings.Repeat("x", 4095)) {
			t.Fatal("UTF-8 boundary loses valid bytes")
		}
	}
}

func TestPublicCapsulePreservesCanonicalBundlesAndReadState(t *testing.T) {
	h, ids, _ := publicCapsuleFixtures(t)
	for _, mode := range []string{"public", "server-held", "tlock"} {
		before, err := h.store.Get(ids[mode])
		if err != nil {
			t.Fatal(err)
		}
		path := "https://localhost/api/v1/public/capsules/" + ids[mode] + "/bundle"
		w := h.send(httptest.NewRequest("GET", path, nil))
		bundle, err := capsule.DecodeBundle(bytes.NewReader(w.Body.Bytes()), 8192)
		if w.Code != 200 || err != nil {
			t.Fatalf("bundle: %d %v", w.Code, err)
		}
		if (bundle.ReleasedKey != "") != (mode == "public") {
			t.Fatal("early bundle exposes an encrypted-mode key")
		}
		after, err := h.store.Get(ids[mode])
		if err != nil {
			t.Fatal(err)
		}
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(after)
		if !bytes.Equal(a, b) {
			t.Fatal("public read changes retained state")
		}
	}
}

func TestPublicCapsuleRejectsDraftsAndInvalidRequests(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "public")
	for _, path := range []string{"/c/" + draft.ID, "/api/v1/public/capsules/" + draft.ID + "/download", "/api/v1/public/capsules/" + draft.ID + "/bundle"} {
		if w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil)); w.Code != 404 {
			t.Fatalf("draft: %s %d", path, w.Code)
		}
		if w := h.send(httptest.NewRequest("POST", "https://localhost"+path, nil)); w.Code != 405 {
			t.Fatalf("method: %d", w.Code)
		}
		if w := h.send(httptest.NewRequest("GET", "https://localhost"+path+"?key=1", nil)); w.Code != 400 {
			t.Fatalf("query: %d", w.Code)
		}
		if w := h.send(httptest.NewRequest("GET", "https://localhost"+path, strings.NewReader("secret"))); w.Code != 400 {
			t.Fatalf("GET body: %d", w.Code)
		}
		if w := h.send(httptest.NewRequest("GET", "http://localhost"+path, nil)); w.Code != 426 {
			t.Fatalf("TLS: %d", w.Code)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if w := h.send(httptest.NewRequest("GET", "https://localhost"+path, nil).WithContext(ctx)); w.Code != 408 {
			t.Fatalf("canceled: %d", w.Code)
		}
	}
	for range cap(h.api.slots) {
		h.api.slots <- struct{}{}
	}
	if w := h.send(httptest.NewRequest("GET", "https://localhost/c/"+draft.ID, nil)); w.Code != 429 {
		t.Fatalf("slots: %d", w.Code)
	}
}

// TestPublicCapsuleBrowserFixture supplies a fake clock and a signed offline beacon to Chromium.
func TestPublicCapsuleBrowserFixture(t *testing.T) {
	h, ids, source := publicCapsuleFixtures(t)
	w := h.send(httptest.NewRequest("GET", "https://localhost/c/"+ids["public"], nil))
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if os.Getenv("EPOCHALYPSE_CAPSULE_BROWSER_FIXTURE") != "1" {
		return
	}
	clock := &atomic.Int64{}
	clock.Store(h.clock.Unix())
	h.api.now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	// This control surface exists only in the test executable.
	mux := http.NewServeMux()
	h.api.routes(mux, h.state.owner, true)
	mux.HandleFunc("GET /fixture", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(ids) })
	mux.HandleFunc("POST /fixture/{state}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("state") {
		case "due":
			clock.Store(1692806364)
		case "delayed":
			source.state.Store(1)
		case "invalid-beacon":
			source.state.Store(2)
		case "corrupt":
			damageCapsuleKey(t, h, ids["corrupt"], false)
		case "lost":
			damageCapsuleKey(t, h, ids["lost"], true)
		default:
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(204)
	})
	server := &http.Server{Addr: "127.0.0.1:8081", Handler: mux, ReadHeaderTimeout: time.Second}
	if err := server.ListenAndServe(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicCapsuleAuthenticatesAttachment(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "server-held")
	h.publish(draft.ID)
	path := "https://localhost/api/v1/public/capsules/" + draft.ID + "/download"
	if w := h.send(httptest.NewRequest("GET", path, nil)); w.Code != 425 || strings.Contains(w.Body.String(), "1234") {
		t.Fatalf("early: %d %s", w.Code, w.Body)
	}
	*h.clock = draft.RequestedReleaseAt
	w := h.send(httptest.NewRequest("GET", path, nil))
	if w.Code != 200 || w.Body.String() != "1234" || w.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("download: %d %s %v", w.Code, w.Body, w.Header())
	}
}

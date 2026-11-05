package service

import (
	"context"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/window"
)

func TestPublicWindowLabelsAndEscapesContent(t *testing.T) {
	h := newWindowHTTP(t)
	record, err := h.state.windows.Create(window.Input{Content: `<script>alert(1)</script>`, Inception: "2026-11-04T13:00:00Z", Expiration: "2026-11-04T14:00:00Z"}, h.now)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.state.routes(mux, false)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "https://localhost/windows/"+record.ID, nil))
	if w.Code != 200 {
		t.Fatalf("public page: %d %s", w.Code, w.Body)
	}
	for _, text := range []string{"Public data", "does not keep content secret", "Direct authority", "unavailable", "Check time (UTC)", "&lt;script&gt;"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Errorf("page omits %q", text)
		}
	}
	if !strings.Contains(html.UnescapeString(w.Body.String()), record.RRSIG) {
		t.Fatal("page changes the stored signature")
	}
	if strings.Contains(w.Body.String(), `<script>alert`) {
		t.Fatal("page contains executable content")
	}
	runs, _, err := h.state.windows.ListRuns(record.ID, "")
	if err != nil || len(runs) != 0 {
		t.Fatal("public read creates retained work")
	}
}

func TestDirectProbeUsesTheBoundListener(t *testing.T) {
	var absent *mainState
	absent.setDirectAddress(&net.TCPAddr{})
	for address, expected := range map[string]string{"0.0.0.0": "127.0.0.1:1053", "::": "[::1]:1053", "127.0.0.1": "127.0.0.1:1053"} {
		state := &mainState{}
		state.setDirectAddress(&net.TCPAddr{IP: net.ParseIP(address), Port: 1053})
		if state.directAddress != expected {
			t.Fatalf("probe address = %s", state.directAddress)
		}
	}
}

func TestPublicWindowObservesEarlyLiveAuthority(t *testing.T) {
	h := newWindowHTTP(t)
	at := time.Now().UTC().Truncate(time.Second)
	record, err := h.state.windows.Create(window.Input{Content: "early public content", Inception: at.Add(time.Hour).Format(time.RFC3339), Expiration: at.Add(2 * time.Hour).Format(time.RFC3339)}, at)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := openDNS(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.respond = h.state.dnsResponder()
	go func() { _ = listener.serveUDP() }()
	go func() { _ = listener.serveTCP() }()
	defer listener.close()
	h.state.directAddress = listener.tcp.Addr().String()
	request := httptest.NewRequest("GET", "https://localhost/windows/"+record.ID, nil)
	evidence := h.state.probeAuthority(request, record)
	if evidence.State != "observed" || !evidence.At.Before(record.Inception) || len(evidence.Records) != 2 || evidence.Transport != "udp" {
		t.Fatalf("early evidence: %+v", evidence)
	}
	for range cap(h.state.directSlots) {
		h.state.directSlots <- struct{}{}
	}
	if h.state.probeAuthority(request, record).State != "unavailable" {
		t.Fatal("probe exceeds concurrency bound")
	}
	for range cap(h.state.directSlots) {
		<-h.state.directSlots
	}
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	if h.state.probeAuthority(request.WithContext(ctx), record).State != "unavailable" {
		t.Fatal("canceled request reports live evidence")
	}
	listener.close()
	if h.state.probeAuthority(request, record).State != "unavailable" {
		t.Fatal("closed authority reports live evidence")
	}
}

func TestPublicWindowRejectsInvalidReads(t *testing.T) {
	h := newWindowHTTP(t)
	mux := http.NewServeMux()
	h.state.routes(mux, false)
	for path, code := range map[string]int{"/windows/bad": 400, "/windows/" + strings.Repeat("a", 32): 404, "/windows/style.css": 200, "/windows/results.js": 200} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "https://localhost"+path, nil))
		if w.Code != code || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	store := h.state.windows
	for _, closed := range []bool{false, true} {
		if closed {
			h.state.windows = store
			_ = store.Close()
		} else {
			h.state.windows = nil
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "https://localhost/windows/"+strings.Repeat("a", 32), nil))
		if w.Code != 503 {
			t.Fatalf("unavailable store: %d", w.Code)
		}
	}
}

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func startWindowService(t *testing.T, config Config) (Addresses, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan Addresses, 1), make(chan error, 1)
	go func() { done <- Serve(ctx, config, func(a Addresses) { ready <- a }) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("window service shutdown timeout")
			}
		})
	}
	t.Cleanup(stop)
	select {
	case a := <-ready:
		return a, stop
	case err := <-done:
		cancel()
		once.Do(func() {})
		t.Fatalf("window service start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("window service start timeout")
	}
	return Addresses{}, stop
}

func liveWindow(t *testing.T, address string) window.Window {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	origin := "http://" + address
	r, err := client.Get(origin + "/api/v1/csrf")
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Token string `json:"csrf_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&proof); err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	cookies := r.Cookies()[0].String()
	post := func(path, body string) *http.Response {
		request, err := http.NewRequest("POST", origin+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Cookie", cookies)
		request.Header.Set("X-CSRF-Token", proof.Token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	r = post("/api/v1/session", `{"password":"synthetic"}`)
	_ = r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatal("live owner sign-in fails")
	}
	cookies += "; " + r.Cookies()[0].String()
	inception := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	input := window.Input{Content: "public live experiment", Inception: inception.Format(time.RFC3339), Expiration: inception.Add(time.Hour).Format(time.RFC3339)}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r = post("/api/v1/windows", string(body))
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != 201 {
		t.Fatalf("live creation: %d", r.StatusCode)
	}
	var record window.Window
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestMainServesHTTPCreatedWindowsBeforeInceptionAndAfterRestart(t *testing.T) {
	config := Config{Mode: "server", HTTPAddr: "127.0.0.1:0", DNSAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true, Environment: "lab", Zone: "windows.example.", KeyFile: serviceKey(t)}
	owners, err := owner.Open(config.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := owners.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	if err := owners.Close(); err != nil {
		t.Fatal(err)
	}
	a, stop := startWindowService(t, config)
	w := liveWindow(t, a.HTTP)
	if a.DNS == "" {
		t.Fatal("main server does not bind authoritative DNS")
	}
	assertLiveRecords(t, a.DNS, w)
	stop()
	restarted, _ := startWindowService(t, config)
	assertLiveRecords(t, restarted.DNS, w)
}

func assertLiveRecords(t *testing.T, address string, w window.Window) {
	t.Helper()
	for _, network := range []string{"udp", "tcp"} {
		q := dns.NewMsg(w.Name, dns.TypeTXT)
		q.Security = true
		q.UDPSize = 1232
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		r, err := dns.Exchange(ctx, q, network, address)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if r.Rcode != dns.RcodeSuccess || !r.Authoritative || r.RecursionAvailable || r.AuthenticatedData || len(r.Answer) != 2 {
			t.Fatalf("live %s: %s", network, r)
		}
		if r.Answer[0].String() != w.TXT || r.Answer[1].String() != w.RRSIG {
			t.Fatal("live authority changes the committed signature")
		}
		if r.Answer[1].(*dns.RRSIG).ValidPeriod(time.Now()) {
			t.Fatal("live authority adds a current TXT signature")
		}
	}
}

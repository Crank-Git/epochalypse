package switchclient

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestHTTPSReplicaPinsEndpointAndBoundsResponse(t *testing.T) {
	p, _, r := armFixture(t)
	f, err := switchwire.DecodeFrame(p.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := r.receipt("agent1", f.State, switchwire.PhasePrepared)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(receipt) }))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	pin := sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
	h := &HTTPSReplica{Pins: map[string][32]byte{server.URL: pin}, Roots: roots}
	a := switchwire.Agent{ID: "agent1", Endpoint: server.URL}
	if _, err := h.Prepare(context.Background(), a, p.Prepare, p.Ciphertext, nil); err != nil {
		t.Fatal(err)
	}
	h.Pins[server.URL] = [32]byte{1}
	if _, err := h.Prepare(context.Background(), a, p.Prepare, p.Ciphertext, nil); err == nil {
		t.Fatal("wrong TLS pin accepted")
	}
	for _, data := range []string{strings.Repeat("x", (32<<10)+1), "{}{}", "{\"unknown\":1}", "{}"} {
		if _, err := readReplicaResponse(strings.NewReader(data)); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}

func TestHTTPSReplicaRejectsRedirectAndStatus(t *testing.T) {
	p, _, _ := armFixture(t)
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "https://other.invalid")
				w.WriteHeader(status)
			}))
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			h := &HTTPSReplica{Pins: map[string][32]byte{server.URL: sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)}, Roots: roots}
			if _, err := h.Arm(context.Background(), switchwire.Agent{Endpoint: server.URL}, p.Prepare); err == nil {
				t.Fatal("failed endpoint accepted")
			}
		})
	}
}

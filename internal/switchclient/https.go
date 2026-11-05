package switchclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// HTTPSReplica authenticates TLS endpoints with operator-pinned public keys.
type HTTPSReplica struct {
	Pins  map[string][32]byte
	Roots *x509.CertPool
	// Instrumented functional tests need a separate budget from production latency.
	requestTimeout time.Duration
}

type replicaRequest struct {
	Request    switchwire.Envelope `json:"request"`
	Ciphertext []byte              `json:"ciphertext,omitempty"`
	Manifest   []byte              `json:"manifest,omitempty"`
}

// Prepare sends complete encrypted material to one enrolled endpoint.
func (h *HTTPSReplica) Prepare(ctx context.Context, a switchwire.Agent, e switchwire.Envelope, ciphertext, manifest []byte) (switchwire.Envelope, error) {
	return h.call(ctx, a, "prepare", replicaRequest{Request: e, Ciphertext: ciphertext, Manifest: manifest})
}

// Arm sends the signed certificate of three prepared receipts.
func (h *HTTPSReplica) Arm(ctx context.Context, a switchwire.Agent, e switchwire.Envelope) (switchwire.Envelope, error) {
	return h.call(ctx, a, "arm", replicaRequest{Request: e})
}

func (h *HTTPSReplica) call(ctx context.Context, a switchwire.Agent, operation string, payload replicaRequest) (switchwire.Envelope, error) {
	if len(payload.Ciphertext) > 11<<20 || len(payload.Manifest) != 0 {
		return switchwire.Envelope{}, errors.New("agent material exceeds limit")
	}
	frame, err := switchwire.DecodeFrame(payload.Request.Body)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	if len(data) > 16<<20 {
		return switchwire.Envelope{}, errors.New("agent request exceeds limit")
	}
	transport, err := h.transport(a)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	defer transport.CloseIdleConnections()
	timeout := h.requestTimeout
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 5 * time.Second
	}
	client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("agent redirect refused") }}
	endpoint, err := agentURL(a, frame.State.Switch, operation)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return switchwire.Envelope{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return switchwire.Envelope{}, errors.New("agent HTTPS request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return switchwire.Envelope{}, fmt.Errorf("agent response status %d", response.StatusCode)
	}
	return readReplicaResponse(response.Body)
}

func (h *HTTPSReplica) transport(a switchwire.Agent) (*http.Transport, error) {
	pin, ok := h.Pins[a.Endpoint]
	if !ok || pin == [32]byte{} || !strings.HasPrefix(a.Endpoint, "https://") {
		return nil, errors.New("agent HTTPS identity is not pinned")
	}
	return &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: h.Roots, VerifyConnection: func(c tls.ConnectionState) error {
		if len(c.PeerCertificates) == 0 || sha256.Sum256(c.PeerCertificates[0].RawSubjectPublicKeyInfo) != pin {
			return errors.New("agent TLS key does not match pin")
		}
		return nil
	}}}, nil
}

func agentURL(a switchwire.Agent, id, operation string) (string, error) {
	endpoint, err := url.Parse(a.Endpoint)
	if err != nil || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", errors.New("invalid agent endpoint")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/agent/v1/switches/" + id + "/" + operation
	return endpoint.String(), nil
}

func readReplicaResponse(r io.Reader) (switchwire.Envelope, error) {
	data, err := io.ReadAll(io.LimitReader(r, 32<<10+1))
	if err != nil {
		return switchwire.Envelope{}, err
	}
	if len(data) > 32<<10 {
		return switchwire.Envelope{}, errors.New("agent response exceeds limit")
	}
	var envelope switchwire.Envelope
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&envelope) != nil || d.Decode(&struct{}{}) != io.EOF {
		return switchwire.Envelope{}, errors.New("invalid agent response")
	}
	if _, err := switchwire.MarshalEnvelope(envelope); err != nil {
		return switchwire.Envelope{}, err
	}
	return envelope, nil
}

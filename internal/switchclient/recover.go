package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Recover returns authenticated content through independent public agents.
func Recover(ctx context.Context, bundle Bundle, owner ed25519.PublicKey, source *HTTPSReplica) ([]byte, error) {
	if source == nil {
		return nil, errors.New("public recovery transport is absent")
	}
	if err := validateBundle(bundle, owner); err != nil {
		return nil, err
	}
	enrollment, err := switchwire.AdmitCertificate(bundle.Plan.Certificate, bundle.Plan.CertificateSignature, owner)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	results := make(chan recoveredAgent, 3)
	for _, a := range enrollment.Agents {
		go func() { results <- source.recoverAgent(ctx, a, bundle, enrollment) }()
	}
	var valid []recoveredAgent
	defer func() {
		for _, result := range valid {
			clear(result.share)
		}
	}()
	for range enrollment.Agents {
		result := <-results
		if result.err == nil {
			valid = append(valid, result)
		}
	}
	for i := range valid {
		for j := i + 1; j < len(valid); j++ {
			plaintext, err := switchcontent.Recover(valid[i].ciphertext, [][]byte{valid[i].share, valid[j].share})
			if err == nil {
				return plaintext, nil
			}
		}
	}
	return nil, errors.New("two authenticated release shares are unavailable")
}

type recoveredAgent struct {
	share, ciphertext []byte
	err               error
}

func (h *HTTPSReplica) recoverAgent(ctx context.Context, a switchwire.Agent, b Bundle, enrollment switchwire.AdmittedEnrollment) recoveredAgent {
	manifestBytes, err := h.publicGet(ctx, a, b.Plan.State.Switch, "", 64<<10)
	if err != nil {
		return recoveredAgent{err: err}
	}
	var manifest switchwire.PublicManifest
	d := json.NewDecoder(bytes.NewReader(manifestBytes))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || d.Decode(&struct{}{}) != io.EOF {
		return recoveredAgent{err: errors.New("invalid public manifest")}
	}
	share, err := validatePublicManifest(manifest, a, b, enrollment)
	if err != nil {
		return recoveredAgent{err: err}
	}
	ciphertext, err := h.publicGet(ctx, a, b.Plan.State.Switch, "/ciphertext", 11<<20)
	if err != nil {
		return recoveredAgent{err: err}
	}
	if err := switchcontent.ValidateBinding(ciphertext, b.Plan.State.Key, b.Plan.State.Content); err != nil {
		return recoveredAgent{err: err}
	}
	return recoveredAgent{share: share, ciphertext: ciphertext}
}

func validatePublicManifest(m switchwire.PublicManifest, a switchwire.Agent, b Bundle, enrollment switchwire.AdmittedEnrollment) ([]byte, error) {
	if m.Version != 1 || !bytes.Equal(m.Certificate, b.Plan.Certificate) || !bytes.Equal(m.CertificateSignature, b.Plan.CertificateSignature) || m.Release == nil {
		return nil, errors.New("public enrollment or release is invalid")
	}
	accepted, err := switchwire.VerifyOwnerRequest(m.Accepted, enrollment)
	if err != nil || (accepted.Operation != switchwire.OperationArm && accepted.Operation != switchwire.OperationCheckin) || accepted.Phase != switchwire.PhaseArmed || !validPublicSuccessor(b.Plan.State, accepted.State) {
		return nil, errors.New("public accepted state is invalid")
	}
	release, err := switchwire.VerifyEnvelope(*m.Release, map[string]ed25519.PublicKey{a.ID: ed25519.PublicKey(a.SigningKey[:])})
	if err != nil || release.Operation != switchwire.OperationRelease || release.State != accepted.State || release.Phase != switchwire.PhaseStopped || release.Entries[0].Key != a.SigningKey {
		return nil, errors.New("public release signature or binding is invalid")
	}
	return bytes.Clone(release.Entries[0].Data), nil
}

func validPublicSuccessor(before, after switchwire.State) bool {
	seq, deadline := before.Sequence, before.Deadline
	before.Sequence, before.Deadline = after.Sequence, after.Deadline
	return before == after && after.Sequence >= seq && after.Deadline >= deadline
}

func (h *HTTPSReplica) publicGet(ctx context.Context, a switchwire.Agent, id, suffix string, limit int64) ([]byte, error) {
	transport, err := h.transport(a)
	if err != nil {
		return nil, err
	}
	defer transport.CloseIdleConnections()
	endpoint, err := url.Parse(a.Endpoint)
	if err != nil || endpoint.RawQuery != "" {
		return nil, errors.New("invalid public endpoint")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/agent/v1/public/switches/" + id + suffix
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("invalid public request")
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("public redirect refused") }}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("public HTTPS request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("public data is unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("public data exceeds limit or read fails")
	}
	return data, nil
}

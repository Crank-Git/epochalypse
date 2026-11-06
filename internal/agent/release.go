package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Publish persists irreversible disclosure before it permits a public share.
func (t *Transitions) Publish() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.publish()
}

func (t *Transitions) publish() error {
	current, err := t.journal.Snapshot()
	if err != nil {
		return err
	}
	if current.PossibleDisclosure {
		return t.validateRelease(current)
	}
	if current.Phase != switchwire.PhaseArmed {
		return nil
	}
	if err := validateArmedState(t, current); err != nil {
		return err
	}
	reading, err := t.assessClock(current)
	if err != nil {
		return err
	}
	accepted, err := switchwire.ReadEnvelope(bytes.NewReader(current.AcceptedRequest))
	if err != nil {
		return err
	}
	frame, err := switchwire.DecodeFrame(accepted.Body)
	if err != nil {
		return err
	}
	// The canonical frame bounds this sum before the signed integer conversion.
	if reading.LowerUTC() < frame.State.Deadline+int64(frame.State.Grace) {
		return nil
	} // #nosec G115 -- DecodeFrame bounds the sum to MaxInt64.
	share, err := switchcontent.DecryptShare(current.EncryptedShare, t.ageIdentity)
	if err != nil {
		return fmt.Errorf("read retained release material: %w", err)
	}
	defer clear(share)
	release, err := switchwire.SignFrame(switchwire.Frame{
		Operation: switchwire.OperationRelease, Signer: t.agentID, State: frame.State,
		Durable: frame.State.Sequence, Phase: switchwire.PhaseStopped, Released: true,
		Entries: []switchwire.Entry{{ID: t.agentID, Key: [32]byte(t.signingKey.Public().(ed25519.PublicKey)), Data: share}},
	}, t.signingKey)
	if err != nil {
		return err
	}
	current.ReleaseEnvelope, err = switchwire.MarshalEnvelope(release)
	if err != nil {
		return err
	}
	current.PossibleDisclosure = true
	_, err = t.commit(current)
	return err
}

func (t *Transitions) validateRelease(current State) error {
	if !current.PossibleDisclosure || current.UnsafeClock {
		return ErrQuarantined
	}
	private := current
	private.PossibleDisclosure, private.ReleaseEnvelope = false, nil
	if err := validateArmedState(t, private); err != nil {
		return err
	}
	accepted, err := switchwire.ReadEnvelope(bytes.NewReader(current.AcceptedRequest))
	if err != nil {
		return err
	}
	frame, err := switchwire.DecodeFrame(accepted.Body)
	if err != nil {
		return err
	}
	share, err := switchcontent.DecryptShare(current.EncryptedShare, t.ageIdentity)
	if err != nil {
		return err
	}
	defer clear(share)
	release, err := switchwire.ReadEnvelope(bytes.NewReader(current.ReleaseEnvelope))
	if err != nil {
		return err
	}
	keys := enrolledReleaseKeys(t.enrollment)
	f, err := switchwire.VerifyEnvelope(release, keys)
	if err != nil || f.Operation != switchwire.OperationRelease || f.Signer != t.agentID || f.State != frame.State ||
		f.Phase != switchwire.PhaseStopped || !bytes.Equal(f.Entries[0].Data, share) {
		return ErrQuarantined
	}
	return nil
}

// RunPublisher samples fresh clock evidence until the service stops or publication fails.
func (t *Transitions) RunPublisher(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(CandidateSampleGapSeconds) * time.Second)
	defer ticker.Stop()
	for {
		if err := t.Publish(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (t *Transitions) servePublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, contentType, status := t.publicResponse(r)
	if status != http.StatusOK {
		http.Error(w, "agent publication is unavailable", status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (t *Transitions) publicResponse(r *http.Request) ([]byte, string, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.publish(); err != nil {
		return nil, "", http.StatusServiceUnavailable
	}
	state, err := t.journal.Snapshot()
	if err != nil {
		return nil, "", http.StatusServiceUnavailable
	}
	if state.Phase != switchwire.PhaseArmed {
		return nil, "", http.StatusNotFound
	}
	accepted, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		return nil, "", http.StatusNotFound
	}
	frame, err := switchwire.DecodeFrame(accepted.Body)
	base := "/agent/v1/public/switches/" + frame.State.Switch
	if err != nil || (r.URL.Path != base && r.URL.Path != base+"/ciphertext") {
		return nil, "", http.StatusNotFound
	}
	if strings.HasSuffix(r.URL.Path, "/ciphertext") {
		return state.Ciphertext, "application/octet-stream", http.StatusOK
	}
	end := len(t.enrollmentB) - ed25519.SignatureSize
	manifest := switchwire.PublicManifest{Version: 1, Certificate: t.enrollmentB[4:end], CertificateSignature: t.enrollmentB[end:], Accepted: accepted}
	if state.PossibleDisclosure {
		release, err := switchwire.ReadEnvelope(bytes.NewReader(state.ReleaseEnvelope))
		if err != nil {
			return nil, "", http.StatusServiceUnavailable
		}
		manifest.Release = &release
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", http.StatusServiceUnavailable
	}
	return data, "application/json", http.StatusOK
}

func enrolledReleaseKeys(enrollment switchwire.AdmittedEnrollment) map[string]ed25519.PublicKey {
	keys := make(map[string]ed25519.PublicKey, 3)
	for _, a := range enrollment.Agents {
		keys[a.ID] = ed25519.PublicKey(a.SigningKey[:])
	}
	return keys
}

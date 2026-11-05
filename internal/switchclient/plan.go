package switchclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// NewPlan signs encrypted content for an existing main-state draft.
// The owner keeps the private key outside the main service.
func NewPlan(draft switches.Record, material switchcontent.Material, agents [3]switchwire.Agent, key ed25519.PrivateKey, issued time.Time) (Plan, error) {
	if switches.ValidateRecord(draft) != nil || len(key) != ed25519.PrivateKeySize || issued.IsZero() || issued.Unix() < 0 || issued.Unix() > 1<<62 {
		return Plan{}, errors.New("invalid switch preparation input")
	}
	// ValidateRecord bounds both durations before their unsigned representation.
	c := switchwire.Certificate{Version: switchwire.Version, Interval: uint64(draft.IntervalSeconds), Grace: uint64(draft.GraceSeconds), Key: material.KeyID, Content: material.ContentDigest, Agents: agents} // #nosec G115 -- ValidateRecord bounds both durations.
	copy(c.OwnerKey[:], key.Public().(ed25519.PublicKey))
	body, err := switchwire.EncodeCertificate(c)
	if err != nil {
		return Plan{}, err
	}
	f := switchwire.Frame{Operation: switchwire.OperationPrepare, Signer: "owner", Phase: switchwire.PhasePrepared, State: switchwire.State{Switch: draft.ID, Version: switchwire.Version, Sequence: 1, Deadline: issued.Unix() + draft.IntervalSeconds, Grace: c.Grace, Content: c.Content, Key: c.Key, Enrollment: sha256.Sum256(body)}}
	if _, err := rand.Read(f.Nonce[:]); err != nil {
		return Plan{}, err
	}
	for i, a := range agents {
		f.Entries = append(f.Entries, switchwire.Entry{ID: a.ID, Key: sha256.Sum256([]byte(a.Recipient)), Data: material.Shares[i]})
	}
	request, err := switchwire.SignFrame(f, key)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Certificate: body, CertificateSignature: ed25519.Sign(key, body), Prepare: request, Ciphertext: material.Ciphertext}
	_, _, err = validatePlan(p, key)
	return p, err
}

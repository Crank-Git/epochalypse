package switchclient

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

type replicaFixture struct {
	t                      *testing.T
	keys                   map[string]ed25519.PrivateKey
	bundle                 string
	prepareCalls, armCalls int
	failPrepare, failArm   string
}

func (r *replicaFixture) Prepare(_ context.Context, a switchwire.Agent, e switchwire.Envelope, _, _ []byte) (switchwire.Envelope, error) {
	r.prepareCalls++
	if a.ID == r.failPrepare {
		return switchwire.Envelope{}, errors.New("offline")
	}
	f, err := switchwire.DecodeFrame(e.Body)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	return r.receipt(a.ID, f.State, switchwire.PhasePrepared)
}
func (r *replicaFixture) Arm(_ context.Context, a switchwire.Agent, e switchwire.Envelope) (switchwire.Envelope, error) {
	r.armCalls++
	if _, err := os.Stat(r.bundle); err != nil {
		r.t.Fatal("arm precedes bundle export")
	}
	if a.ID == r.failArm {
		return switchwire.Envelope{}, errors.New("response lost")
	}
	f, err := switchwire.DecodeFrame(e.Body)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	return r.receipt(a.ID, f.State, switchwire.PhaseArmed)
}
func (r *replicaFixture) receipt(id string, s switchwire.State, phase byte) (switchwire.Envelope, error) {
	return switchwire.SignFrame(switchwire.Frame{Operation: switchwire.OperationReceipt, Signer: id, State: s, Phase: phase, Durable: s.Sequence}, r.keys[id])
}

func armFixture(t *testing.T) (Plan, ed25519.PrivateKey, *replicaFixture) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := Plan{}
	c := switchwire.Certificate{Version: switchwire.Version, Interval: 3600, Grace: 60, Content: sha256.Sum256(p.Ciphertext), Key: [32]byte{1}}
	copy(c.OwnerKey[:], pub)
	r := &replicaFixture{t: t, keys: map[string]ed25519.PrivateKey{}, bundle: filepath.Join(t.TempDir(), "recovery.json")}
	f := switchwire.Frame{Operation: switchwire.OperationPrepare, Signer: "owner", Phase: switchwire.PhasePrepared, State: switchwire.State{Switch: "switch1", Version: switchwire.Version, Sequence: 1, Deadline: 10000, Grace: c.Grace, Content: c.Content, Key: c.Key}}
	for i, id := range []string{"agent1", "agent2", "agent3"} {
		pub, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		a := switchwire.Agent{ID: id, Recipient: identity.Recipient().String(), Endpoint: "https://" + id + ".invalid"}
		copy(a.SigningKey[:], pub)
		c.Agents[i] = a
		r.keys[id] = private
		f.Entries = append(f.Entries, switchwire.Entry{ID: id, Key: sha256.Sum256([]byte(a.Recipient)), Data: []byte("encrypted share " + id)})
	}
	material, err := switchcontent.Prepare(strings.NewReader("authenticated ciphertext fixture"), [3]string{c.Agents[0].Recipient, c.Agents[1].Recipient, c.Agents[2].Recipient})
	if err != nil {
		t.Fatal(err)
	}
	p.Ciphertext = material.Ciphertext
	c.Content, c.Key = material.ContentDigest, material.KeyID
	f.State.Content, f.State.Key = c.Content, c.Key
	for i := range f.Entries {
		f.Entries[i].Data = material.Shares[i]
	}
	p.Certificate, err = switchwire.EncodeCertificate(c)
	if err != nil {
		t.Fatal(err)
	}
	p.CertificateSignature = ed25519.Sign(key, p.Certificate)
	f.State.Enrollment = sha256.Sum256(p.Certificate)
	p.Prepare, err = switchwire.SignFrame(f, key)
	if err != nil {
		t.Fatal(err)
	}
	return p, key, r
}

func TestArmRegistersDurableIntentBeforeTransmission(t *testing.T) {
	p, key, replicas := armFixture(t)
	called := false
	_, err := PrepareAndArmWithIntent(context.Background(), p, key, replicas, replicas.bundle, func(request switchwire.Envelope) error {
		called = true
		if replicas.armCalls != 0 {
			t.Fatal("registration follows arm transmission")
		}
		data, err := os.ReadFile(replicas.bundle + ".arm")
		if err != nil {
			t.Fatal(err)
		}
		expected, err := switchwire.MarshalEnvelope(request)
		if err != nil || string(data) != string(expected) {
			t.Fatal("registration precedes durable signed intent")
		}
		return errors.New("synthetic registration failure")
	})
	if !called || err == nil || replicas.armCalls != 0 {
		t.Fatalf("registration called=%t sends=%d err=%v", called, replicas.armCalls, err)
	}
}

func TestArmExportsBundleBeforeAllThreeArmedReceipts(t *testing.T) {
	p, key, r := armFixture(t)
	result, err := PrepareAndArm(context.Background(), p, key, r, r.bundle)
	if err != nil || !result.Confirmed || len(result.Prepared) != 3 || len(result.Armed) != 3 || r.armCalls != 3 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, r.armCalls)
	}
}
func TestArmRequiresThreePreparedReceipts(t *testing.T) {
	p, key, r := armFixture(t)
	r.failPrepare = "agent3"
	result, err := PrepareAndArm(context.Background(), p, key, r, r.bundle)
	if err == nil || result.Confirmed || r.armCalls != 0 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, r.armCalls)
	}
}
func TestArmRetainsUncertainPartialAcceptance(t *testing.T) {
	p, key, r := armFixture(t)
	r.failArm = "agent3"
	result, err := PrepareAndArm(context.Background(), p, key, r, r.bundle)
	if err == nil || result.Confirmed || len(result.Armed) != 2 || r.armCalls != 3 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, r.armCalls)
	}
	r.failArm = ""
	r.failPrepare = "agent1"
	result, err = PrepareAndArm(context.Background(), p, key, r, r.bundle)
	if err != nil || !result.Confirmed || r.armCalls != 6 {
		t.Fatalf("exact retry: %+v %v calls=%d", result, err, r.armCalls)
	}
}
func TestArmRejectsFailedBundleExport(t *testing.T) {
	p, key, r := armFixture(t)
	result, err := PrepareAndArm(context.Background(), p, key, r, filepath.Join(r.bundle, "missing"))
	if err == nil || result.Confirmed || r.armCalls != 0 {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, r.armCalls)
	}
}

package agent

import (
	"bytes"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestPublicReleasePersistsDisclosureAtAcceptedBoundary(t *testing.T) {
	f, tr, renewal := armedCheckinFixture(t)
	now := int64(4659)
	tr.clock = func() (Reading, error) { return Reading{lower: now}, nil }
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		tr.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agent/v1/public/switches/switch-1", nil))
		return w
	}
	before := get()
	if before.Code != http.StatusOK {
		t.Fatalf("public manifest status = %d", before.Code)
	}
	state, err := f.journal.Snapshot()
	if err != nil || state.PossibleDisclosure || len(state.ReleaseEnvelope) != 0 {
		t.Fatal("share precedes accepted boundary")
	}
	now++
	after := get()
	if after.Code != http.StatusOK {
		t.Fatalf("public release status = %d", after.Code)
	}
	state, err = f.journal.Snapshot()
	if err != nil || !state.PossibleDisclosure || len(state.ReleaseEnvelope) == 0 {
		t.Fatal("public share lacks durable irreversible history")
	}
	envelope, err := switchwire.ReadEnvelope(bytes.NewReader(state.ReleaseEnvelope))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.VerifyEnvelope(envelope, map[string]ed25519.PublicKey{tr.agentID: f.agentKeys[0].Public().(ed25519.PublicKey)})
	if err != nil || frame.Operation != switchwire.OperationRelease || !frame.Released || len(frame.Entries) != 1 {
		t.Fatal("release lacks authenticated share")
	}
	renewal.State.Deadline = 8200
	signed, err := switchwire.SignFrame(renewal, f.ownerKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Checkin(signed); err == nil {
		t.Fatal("released content key accepts renewal")
	}
	if err := f.journal.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(f.authorityConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	recovered, err := reopened.Snapshot()
	if err != nil || !recovered.PossibleDisclosure || !bytes.Equal(recovered.ReleaseEnvelope, state.ReleaseEnvelope) {
		t.Fatal("restart loses release history")
	}
}

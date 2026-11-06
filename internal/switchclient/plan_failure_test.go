package switchclient

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestNewPlanRejectsInvalidDraftKeyAndIssueTime(t *testing.T) {
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	draft := switches.Record{Version: 1, ID: "0123456789abcdef0123456789abcdef", IntervalSeconds: 3600, GraceSeconds: 60, CreatedAt: time.Unix(1000, 0).UTC(), State: switches.StateDraft}
	if err := switches.ValidateRecord(draft); err != nil {
		t.Fatal(err)
	}
	material := switchcontent.Material{ContentDigest: [32]byte{1}, KeyID: [32]byte{2}}
	var agents [3]switchwire.Agent
	for i, id := range []string{"agent-a", "agent-b", "agent-c"} {
		agents[i] = switchwire.Agent{ID: id, Recipient: "recipient", Endpoint: "https://" + id + ".invalid", SigningKey: [32]byte{byte(i + 1)}}
	}
	badDraft := draft
	badDraft.IntervalSeconds = 0
	shortKey := key[:len(key)-1]
	for _, tc := range []struct {
		name   string
		draft  switches.Record
		key    ed25519.PrivateKey
		issued time.Time
	}{
		{name: "invalid draft", draft: badDraft, key: key, issued: time.Unix(1000, 0)},
		{name: "invalid key", draft: draft, key: shortKey, issued: time.Unix(1000, 0)},
		{name: "zero issue time", draft: draft, key: key},
		{name: "negative issue time", draft: draft, key: key, issued: time.Unix(-1, 0)},
		{name: "issue time exceeds bound", draft: draft, key: key, issued: time.Unix(1<<62+1, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewPlan(tc.draft, material, agents, tc.key, tc.issued); err == nil || err.Error() != "invalid switch preparation input" {
				t.Fatalf("invalid preparation error = %v", err)
			}
		})
	}
}

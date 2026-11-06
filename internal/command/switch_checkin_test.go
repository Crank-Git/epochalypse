package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestSwitchCheckinRequiresLocalOwnerInputs(t *testing.T) {
	var out, stderr bytes.Buffer
	code := runSwitch([]string{"switch", "checkin"}, &out, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "provide --plan, --owner-key, --journal, and --pins") {
		t.Fatalf("check-in input validation: code=%d output=%q", code, stderr.String())
	}
}

func TestCheckinErrorPreservesUncertainReceiptEvidence(t *testing.T) {
	var out, stderr bytes.Buffer
	result := switchclient.Renewal{B0: 100, Bmax: 200, Request: switchwire.Envelope{Body: []byte("signed intent")}, Agents: []switchclient.AgentEvidence{{ID: "agent1", State: "durable"}, {ID: "agent2", State: "uncertain"}, {ID: "agent3", State: "uncertain"}}}
	code := writeRenewalResult(&out, &stderr, result, context.DeadlineExceeded)
	var retained switchclient.Renewal
	if err := json.Unmarshal(out.Bytes(), &retained); err != nil || code != 1 || retained.Confirmed || retained.B0 != 100 || retained.Bmax != 200 || len(retained.Agents) != 3 {
		t.Fatalf("partial renewal error discards evidence: code=%d output=%q error=%v", code, out.String(), err)
	}
}

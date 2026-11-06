package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switches"
)

func TestSwitchRenewalEvidenceRequiresAuthentication(t *testing.T) {
	h, _ := newSwitchHTTP(t)
	response := h.send(countdownRequest(http.MethodPost, "/api/v1/switches/0123456789abcdef0123456789abcdef/checkins", `{}`, ""))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("renewal evidence requires owner authentication: %d %s", response.Code, response.Body)
	}
}

func TestSwitchHTTPReportsAuthenticatedRenewalEvidence(t *testing.T) {
	h, token := newSwitchHTTP(t)
	draft := decodeSwitchRecord(t, h.send(countdownRequest(http.MethodPost, "/api/v1/switches", `{}`, token)))
	arm, proof := signedSwitchRenewalEvidence(t, draft, true)
	post := func(suffix string, evidence any) int {
		data, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		return h.send(countdownRequest(http.MethodPost, "/api/v1/switches/"+draft.ID+suffix, string(data), token)).Code
	}
	if code := post("/arm", arm); code != 200 {
		t.Fatalf("arm evidence: %d", code)
	}
	partial := proof
	partial.ConfirmedRequest, partial.ConfirmedReceipts = arm.Request, arm.Armed
	partial.Receipts = proof.Receipts[:2]
	if code := post("/checkins", partial); code != 202 {
		t.Fatalf("uncertain evidence: %d", code)
	}
	response := h.send(countdownRequest(http.MethodGet, "/api/v1/switches/"+draft.ID, "", token))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"confirmed":false`) || !strings.Contains(response.Body.String(), `"state":"uncertain"`) || !strings.Contains(response.Body.String(), `"allowance_assumptions"`) {
		t.Fatalf("uncertain response: %d %s", response.Code, response.Body)
	}
	var view struct {
		Status switches.RenewalStatus `json:"renewal_status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || view.Status.Bmax-view.Status.B0 != 1 || len(view.Status.Agents) != 3 || view.Status.Agents[2].State != "uncertain" {
		t.Fatalf("derived evidence: %+v %v", view, err)
	}
	if code := post("/checkins", proof); code != 200 {
		t.Fatalf("confirmed evidence: %d", code)
	}
	if code := post("/checkins", partial); code != 400 {
		t.Fatalf("receipt regression: %d", code)
	}
	if code := post("/checkins", map[string]any{"b0": 0}); code != 400 {
		t.Fatalf("untrusted bound: %d", code)
	}
}

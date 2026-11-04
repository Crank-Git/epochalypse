package switchprotocol

import "testing"

func fixture(t *testing.T) (Rules, State) {
	t.Helper()
	rules, err := NewRules([3]AgentID{"a", "b", "c"})
	if err != nil { t.Fatal(err) }
	return rules, State{SwitchID:"synthetic", ProtocolVersion:1, Sequence:2, Deadline:100, Grace:10, ContentDigest:[32]byte{1}}
}

func TestRejectsInvalidAgentIdentities(t *testing.T) {
	for _, agents := range [][3]AgentID{{"", "b", "c"}, {"a", "a", "c"}, {"a", "b", "a"}} {
		if _, err := NewRules(agents); err == nil { t.Fatal("accepts invalid identities") }
	}
}

func TestRequiresTwoDistinctPublicShares(t *testing.T) {
	rules, state := fixture(t)
	observer := rules.NewObserver(state)
	share := PublicShare{Agent:"a", SwitchID:state.SwitchID, ProtocolVersion:state.ProtocolVersion, ContentDigest:state.ContentDigest}
	if observer.ReleaseAvailable() { t.Fatal("releases without shares") }
	if !observer.Observe(share) { t.Fatal("rejects first share") }
	observer.Observe(share)
	if observer.ReleaseAvailable() { t.Fatal("counts duplicate shares") }
	share.Agent = "b"
	if !observer.Observe(share) || !observer.ReleaseAvailable() { t.Fatal("rejects two distinct shares") }
	share.Agent = "c"
	observer.Observe(share)
	if !observer.ReleaseAvailable() { t.Fatal("loses release after third share") }
}

func TestRejectsUnrelatedPublicShares(t *testing.T) {
	rules, state := fixture(t)
	valid := PublicShare{Agent:"a", SwitchID:state.SwitchID, ProtocolVersion:state.ProtocolVersion, ContentDigest:state.ContentDigest}
	for _, change := range []func(*PublicShare){
		func(s *PublicShare){s.Agent="unknown"},
		func(s *PublicShare){s.SwitchID="other"},
		func(s *PublicShare){s.ProtocolVersion++},
		func(s *PublicShare){s.ContentDigest[0]++},
	} {
		observer := rules.NewObserver(state)
		share := valid
		change(&share)
		if observer.Observe(share) || observer.ReleaseAvailable() { t.Fatal("accepts unrelated share") }
	}
}

func TestRequiresThreeDistinctDurableReceipts(t *testing.T) {
	rules, state := fixture(t)
	receipts := []Receipt{
		{Agent:"a", Accepted:state, DurableSequence:state.Sequence, Durable:true},
		{Agent:"b", Accepted:state, DurableSequence:state.Sequence, Durable:true},
		{Agent:"c", Accepted:state, DurableSequence:state.Sequence, Durable:true},
	}
	if rules.RenewalConfirmed(state, nil) || rules.RenewalConfirmed(state, receipts[:2]) { t.Fatal("confirms missing receipts") }
	if !rules.RenewalConfirmed(state, receipts) { t.Fatal("rejects three durable receipts") }
	for _, change := range []func(*Receipt){
		func(r *Receipt){r.Agent="a"},
		func(r *Receipt){r.Agent="unknown"},
		func(r *Receipt){r.Durable=false},
		func(r *Receipt){r.DurableSequence--},
		func(r *Receipt){r.Accepted.Sequence++},
		func(r *Receipt){r.Accepted.Deadline++},
		func(r *Receipt){r.Accepted.Grace++},
		func(r *Receipt){r.Accepted.SwitchID="other"},
		func(r *Receipt){r.Accepted.ProtocolVersion++},
		func(r *Receipt){r.Accepted.ContentDigest[0]++},
	} {
		altered := append([]Receipt(nil), receipts...)
		change(&altered[2])
		if rules.RenewalConfirmed(state, altered) { t.Fatal("confirms invalid receipt set") }
	}
}

func TestRetainsPublicSharesAcrossRenewal(t *testing.T) {
	rules, state := fixture(t)
	observer := rules.NewObserver(state)
	observer.Observe(PublicShare{Agent:"a", SwitchID:state.SwitchID, ProtocolVersion:state.ProtocolVersion, ContentDigest:state.ContentDigest})
	state.Sequence++
	state.Deadline += 100
	if observer.ReleaseAvailable() { t.Fatal("releases from one retained share") }
	observer.Observe(PublicShare{Agent:"b", SwitchID:state.SwitchID, ProtocolVersion:state.ProtocolVersion, ContentDigest:state.ContentDigest})
	if !observer.ReleaseAvailable() { t.Fatal("forgets the old public share") }
}

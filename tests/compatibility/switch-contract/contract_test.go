package contract

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"testing"
)

func fixture() (Message, ed25519.PrivateKey, map[string]ed25519.PublicKey) {
	k := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7},32))
	s := State{Switch:"synthetic",Version:2,Sequence:1,Deadline:100,Grace:10,Content:[32]byte{1},Key:[32]byte{2},Enrollment:[32]byte{3}}
	return Message{Operation:"checkin",Signer:"owner",State:s}, k, map[string]ed25519.PublicKey{"owner":k.Public().(ed25519.PublicKey),"agent-a":k.Public().(ed25519.PublicKey)}
}

func signed(t *testing.T, m Message, k ed25519.PrivateKey) Envelope {
	t.Helper(); b,err:=encode(m); if err!=nil {t.Fatal(err)}
	return Envelope{b,ed25519.Sign(k,b)}
}

func TestVerifiesEveryMessageDomain(t *testing.T) {
	for _,op:=range []string{"prepare","arm","checkin","status-request","status-response","release","receipt","cancel"} {
		t.Run(op,func(t *testing.T){
			m,k,keys:=fixture(); m.Operation=op
			if op=="receipt" || op=="status-response" || op=="release" {m.Signer="agent-a";m.Durable=1}
			if op=="prepare" || op=="arm" {m.Entries=[]Entry{{"agent-a",[32]byte{1},[]byte{1}},{"agent-b",[32]byte{2},[]byte{2}},{"agent-c",[32]byte{3},[]byte{3}}}}
			if op=="release" {m.Released=true;m.Entries=[]Entry{{"agent-a",[32]byte{1},bytes.Repeat([]byte{5},33)}}}
			e:=signed(t,m,k); got,err:=verify(e,keys)
			if err!=nil || got.State!=m.State || got.Operation!=op {t.Fatalf("domain or state differs: %v",err)}
			b,err:=encode(got);if err!=nil || !bytes.Equal(b,e.Body) {t.Fatal("canonical bytes differ")}
		})
	}
}

func TestRejectsAlteredSignedInput(t *testing.T) {
	m,k,keys:=fixture(); e:=signed(t,m,k)
	for i:=range e.Body {b:=bytes.Clone(e.Body);b[i]^=1;if _,err:=verify(Envelope{b,e.Signature},keys);err==nil {t.Fatalf("accepts altered byte %d",i)}}
	for i:=0;i<len(e.Body);i++ {if _,err:=verify(Envelope{e.Body[:i],e.Signature},keys);err==nil {t.Fatalf("accepts truncation %d",i)}}
	for _,mutate:=range []func(*Message){
		func(m *Message){m.State.Version=3},func(m *Message){m.Operation="unknown"},func(m *Message){m.Signer="stranger"},
		func(m *Message){m.Signer="agent-a"},func(m *Message){m.State.Switch=string(bytes.Repeat([]byte{'x'},129))},
		func(m *Message){m.State.Grace=^uint64(0)},func(m *Message){m.State.Deadline=-1},func(m *Message){m.State.Sequence=0},
	} {n:=m;mutate(&n);b,err:=encode(n);if err==nil {if _,err=verify(Envelope{b,ed25519.Sign(k,b)},keys);err==nil {t.Fatal("accepts invalid message")}}}
	wrong:=ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8},32));if _,err:=verify(Envelope{e.Body,ed25519.Sign(wrong,e.Body)},keys);err==nil {t.Fatal("accepts wrong key")}
	if _,err:=verify(Envelope{append(bytes.Clone(e.Body),0),e.Signature},keys);err==nil {t.Fatal("accepts trailing bytes")}
	if _,err:=verify(Envelope{make([]byte,16385),e.Signature},keys);err==nil {t.Fatal("accepts oversized body")}
	keys["owner"]=nil;if _,err:=verify(e,keys);err==nil {t.Fatal("accepts malformed key")}
}

func TestRejectsNoncanonicalAndUnboundedEntries(t *testing.T) {
	m,_,_:=fixture();m.Operation="prepare"
	good:=[]Entry{{"agent-a",[32]byte{1},[]byte{1}},{"agent-b",[32]byte{2},[]byte{2}},{"agent-c",[32]byte{3},[]byte{3}}}
	for _,entries:=range [][]Entry{nil,good[:2],{good[1],good[0],good[2]},{good[0],good[0],good[2]},{{"agent-a",[32]byte{1},make([]byte,4097)},good[1],good[2]}} {
		m.Entries=entries;if _,err:=encode(m);err==nil {t.Fatal("accepts invalid entries")}
	}
	m.Entries=good;b,err:=encode(m);if err!=nil {t.Fatal(err)}
	// The length limit applies before the decoder allocates an entry.
	binary.BigEndian.PutUint32(b[len(b)-5:len(b)-1],0xffffffff)
	k:=ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7},32));keys:=map[string]ed25519.PublicKey{"owner":k.Public().(ed25519.PublicKey)}
	if _,err:=verify(Envelope{b,ed25519.Sign(k,b)},keys);err==nil {t.Fatal("accepts an unbounded field")}
}

func TestBindsJournalBytesToStateAndEnrollment(t *testing.T) {
	m,k,keys:=fixture();e:=signed(t,m,k)
	if err:=journal(e,m.State,keys);err!=nil {t.Fatal(err)}
	for _,change:=range []func(*State){func(s *State){s.Switch="other"},func(s *State){s.Sequence++},func(s *State){s.Deadline++},func(s *State){s.Grace++},func(s *State){s.Content[0]++},func(s *State){s.Key[0]++},func(s *State){s.Enrollment[0]++},func(s *State){s.Version++}} {
		s:=m.State;change(&s);if err:=journal(e,s,keys);err==nil {t.Fatal("accepts mismatched decoded state")}
	}
	copy:=Envelope{bytes.Clone(e.Body),bytes.Clone(e.Signature)};if err:=journal(copy,m.State,keys);err!=nil {t.Fatal(err)}
	if !bytes.Equal(copy.Body,e.Body) {t.Fatal("retry changes bytes")}
}

func TestRejectsReplayAndSequenceConflict(t *testing.T) {
	m,_,_:=fixture();old:=m.State
	if err:=accept(old,old);err==nil {t.Fatal("accepts replay as new authority")}
	conflict:=old;conflict.Deadline++;if err:=accept(old,conflict);err==nil {t.Fatal("accepts same-sequence conflict")}
	next:=old;next.Sequence++;next.Deadline++;if err:=accept(old,next);err!=nil {t.Fatal(err)}
	next.Deadline=99;if err:=accept(old,next);err==nil {t.Fatal("shortens boundary")}
	next=old;next.Sequence++;next.Key[0]++;if err:=accept(old,next);err==nil {t.Fatal("changes key identity")}
}

func TestQuarantinesBeforeReceiptsOrShares(t *testing.T) {
	m,_,_:=fixture();s:=Snapshot{State:m.State,Phase:2,Generation:2};a:=Authority{s,true,true}
	if err:=restore(s,a);err!=nil {t.Fatal(err)}
	for _,change:=range []func(*Authority){func(a *Authority){a.Present=false},func(a *Authority){a.Complete=false},func(a *Authority){a.Head.Generation++},func(a *Authority){a.Head.State.Deadline++},func(a *Authority){a.Head.Released=true},func(a *Authority){a.Head.Unsafe=true}} {
		n:=a;change(&n);if err:=restore(s,n);err==nil {t.Fatal("accepts missing, stale, or inconsistent evidence")}
	}
}

func TestLifecyclePersistsBeforeOutput(t *testing.T) {
	m,_,_:=fixture();s:=Snapshot{State:m.State}
	for _,action:=range []string{"prepare","arm","release"} {
		before:=s
		if got,out,err:=transition(s,action,false,true);err==nil || out || got!=before {t.Fatal("outputs before durable commit")}
		var out bool;var err error;s,out,err=transition(s,action,true,true);if err!=nil || !out {t.Fatalf("transition %s: %v",action,err)}
		if action=="release" && !s.Released {t.Fatal("disclosure lacks durable latch")}
	}
	for _,action:=range []string{"renew","cancel","arm","replace"} {if _,_,err:=transition(s,action,true,true);err==nil {t.Fatal("recalls or renews a disclosed key")}}
	if _,out,err:=transition(s,"restart",true,false);err==nil || out {t.Fatal("restart skips fresh assessment")}
	got,out,err:=transition(s,"restart",true,true);if err!=nil || out || !got.Released {t.Fatal("restart loses release latch")}
}

func TestCancelAndReplacementPreserveOldAuthority(t *testing.T) {
	m,_,_:=fixture();s:=Snapshot{State:m.State,Phase:2}
	cancelled,out,err:=transition(s,"cancel",true,true);if err!=nil || !out || cancelled.Phase!=3 || cancelled.State!=s.State {t.Fatal("cancel changes deadline or loses receipt")}
	if _,_,err:=transition(cancelled,"renew",true,true);err==nil {t.Fatal("renews cancelled key")}
	if _,_,err:=transition(s,"replace",true,true);err==nil {t.Fatal("replaces threshold slot for same key")}
	if _,_,err:=transition(s,"arm",true,true);err==nil {t.Fatal("rearms existing key")}
}

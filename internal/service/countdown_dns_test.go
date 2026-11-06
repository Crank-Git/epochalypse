package service

import (
	"context"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/countdown"
)

func TestCountdownAuthorityServesAbsoluteTXTWithoutWindows(t *testing.T) {
	config := Config{StateDir: t.TempDir(), CountdownZone: "countdown.example.", CountdownKeyFile: countdownServiceKey(t), Environment: "lab"}
	s, err := openMainState(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	now := time.Now().UTC()
	event := now.Add(time.Hour).Truncate(time.Second)
	record, err := s.countdowns.Create(context.Background(), countdown.Input{Title: "Synthetic event", EventAt: event.Format(time.RFC3339)}, now)
	if err != nil {
		t.Fatal(err)
	}
	respond := s.dnsResponder()
	if respond == nil {
		t.Fatal("the window-disabled service omits the countdown authority")
	}
	query := dns.NewMsg(record.Name, dns.TypeTXT)
	query.Security = true
	if err := query.Pack(); err != nil {
		t.Fatal(err)
	}
	response := &dns.Msg{Data: respond(query.Data, true)}
	if err := response.Unpack(); err != nil {
		t.Fatal(err)
	}
	if !response.Authoritative || response.Rcode != dns.RcodeSuccess || len(response.Answer) != 2 {
		t.Fatalf("the signed TXT is unavailable: %s", response)
	}
	txt, ok := response.Answer[0].(*dns.TXT)
	if !ok || len(txt.Txt) != 1 || txt.Txt[0] != "v=epochalypse1 event="+event.Format(time.RFC3339) {
		t.Fatalf("the absolute event changes: %s", response)
	}
	sig, ok := response.Answer[1].(*dns.RRSIG)
	if !ok || sig.OrigTTL != countdown.MaxTTL {
		t.Fatalf("the signed Original TTL changes: %s", response)
	}
	_, _, key, err := s.countdownSigner.Records(record, now)
	if err != nil || sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}) != nil {
		t.Fatal("the countdown signature fails", err)
	}
	assertCountdownRenewal(t, s, record, now.Add(72*time.Hour))
}

func assertCountdownRenewal(t *testing.T, s *mainState, record countdown.Record, at time.Time) {
	t.Helper()
	query := dns.NewMsg(record.Name, dns.TypeTXT)
	query.Security = true
	if err := query.Pack(); err != nil {
		t.Fatal(err)
	}
	response := &dns.Msg{Data: s.countdownAuthority.Packet(query.Data, false, at)}
	if err := response.Unpack(); err != nil {
		t.Fatal(err)
	}
	if len(response.Answer) != 2 {
		t.Fatalf("the completed event is unavailable: %s", response)
	}
	sig := response.Answer[1].(*dns.RRSIG)
	if !sig.ValidPeriod(at) || response.Answer[0].(*dns.TXT).Txt[0] != record.TXT().Txt[0] {
		t.Fatal("ordinary renewal changes the completed event")
	}
	if err := s.countdowns.Delete(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	response = &dns.Msg{Data: s.countdownAuthority.Packet(query.Data, false, at)}
	if err := response.Unpack(); err != nil {
		t.Fatal(err)
	}
	if response.Rcode != dns.RcodeNameError || len(response.Answer) != 0 || len(response.Ns) == 0 {
		t.Fatalf("deletion retains a signed event: %s", response)
	}
	if _, err := authority.NewCountdown(nil, s.countdownSigner, "ns.lab.invalid."); err == nil {
		t.Fatal("a missing countdown store passes")
	}
}

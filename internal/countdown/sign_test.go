package countdown

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

func countdownKey(t *testing.T, zone string) string {
	t.Helper()
	key := dns.NewDNSKEY(zone, dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	for name, text := range map[string]string{path: key.PrivateKeyString(private), strings.TrimSuffix(path, ".private") + ".key": key.String()} {
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestCountdownSignerPreservesEventAcrossCompletionAndRenewal(t *testing.T) {
	zone := "countdown.example."
	s, err := LoadSigner(zone, countdownKey(t, zone))
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.TempDir(), installation.DefaultLimits(), zone)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	r, err := store.Create(context.Background(), validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, now := range []time.Time{testNow, r.EventAt, r.EventAt.Add(72 * time.Hour)} {
		txt, sig, key, err := s.Records(r, now)
		if err != nil {
			t.Fatal(err)
		}
		if txt.String() != r.TXT().String() || sig.OrigTTL != MaxTTL || !sig.ValidPeriod(now.Add(24*time.Hour)) || key.Hdr.Name != zone {
			t.Fatalf("event or ordinary validity changes: %s %s", txt, sig)
		}
		if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
			t.Fatal(err)
		}
		txt.Txt[0] = "changed"
		if r.TXT().Txt[0] != "v=epochalypse1 event="+validInput().EventAt {
			t.Fatal("caller changes the absolute event")
		}
	}
}

func TestCountdownSignerAcceptsZeroKeyTag(t *testing.T) {
	zone := "countdown.example."
	path := filepath.Join(t.TempDir(), "zero.private")
	for _, extension := range []string{"private", "key"} {
		data, err := os.ReadFile("../../testdata/dnssec/zero." + extension) // #nosec G304 -- Both extensions select the tracked synthetic fixture.
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(data), "windows.example.", zone)
		if err := os.WriteFile(strings.TrimSuffix(path, ".private")+"."+extension, []byte(text), 0600); err != nil { // #nosec G703 -- The test controls both temporary paths.
			t.Fatal(err)
		}
	}
	s, err := LoadSigner(zone, path)
	if err != nil {
		t.Fatal(err)
	}
	r := Record{Version: 1, ID: strings.Repeat("a", 32), Title: "Synthetic event", CreatedAt: testNow.Truncate(time.Second), EventAt: testNow.Add(time.Hour).Truncate(time.Second), Zone: zone, Name: "c" + strings.Repeat("a", 32) + "." + zone}
	for _, now := range []time.Time{testNow, r.EventAt, r.EventAt.Add(72 * time.Hour)} {
		txt, sig, key, err := s.Records(r, now)
		if err != nil {
			t.Fatal(err)
		}
		if key.KeyTag() != 0 || sig.KeyTag != 0 || txt.String() != r.TXT().String() || !sig.ValidPeriod(now) {
			t.Fatal("zero-tag signature changes the event or ordinary validity")
		}
		if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCountdownSignerRejectsInvalidMetadataAndKeyReferences(t *testing.T) {
	zone := "countdown.example."
	path := countdownKey(t, zone)
	for _, pair := range [][2]string{{"", path}, {zone, "missing.private"}, {"windows.example.", path}} {
		if _, err := LoadSigner(pair[0], pair[1]); err == nil {
			t.Fatal("accepts an invalid countdown key reference")
		}
	}
	s, err := LoadSigner(zone, path)
	if err != nil {
		t.Fatal(err)
	}
	if s.LabOnly() {
		t.Fatal("unmarked synthetic key claims the explicit lab marker")
	}
	r := Record{Version: 1, ID: strings.Repeat("a", 32), Title: "Synthetic event", CreatedAt: testNow.Truncate(time.Second), EventAt: testNow.Add(time.Hour).Truncate(time.Second), Zone: zone, Name: "c" + strings.Repeat("a", 32) + "." + zone}
	for _, change := range []func(*Record){
		func(r *Record) { r.Version = 2 },
		func(r *Record) { r.Zone = "windows.example."; r.Name = "c" + r.ID + "." + r.Zone },
		func(r *Record) { r.EventAt = time.Time{} },
	} {
		bad := r
		change(&bad)
		if _, _, _, err := s.Records(bad, testNow); err == nil {
			t.Fatal("signs invalid or foreign countdown metadata")
		}
	}
	if _, _, _, err := s.Records(r, time.Time{}); err == nil {
		t.Fatal("signs at an unsupported clock")
	}
}

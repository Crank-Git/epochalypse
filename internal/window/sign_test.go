package window

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
)

func testKeyFiles(t *testing.T) (string, *dns.DNSKEY) {
	t.Helper()
	key := dns.NewDNSKEY("windows.example.", dns.ECDSAP256SHA256)
	key.Flags = 257
	key.Hdr.TTL = 60
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic.private")
	if err := os.WriteFile(path, []byte(key.PrivateKeyString(private)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strings.TrimSuffix(path, ".private")+".key", []byte(key.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return path, key
}

func testSigner(t *testing.T) *Signer {
	t.Helper()
	path, _ := testKeyFiles(t)
	s, err := LoadSigner("windows.example.", path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignerAcceptsZeroKeyTag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero.private")
	for _, extension := range []string{"private", "key"} {
		data, err := os.ReadFile("../../testdata/dnssec/zero." + extension) // #nosec G304 -- Both extensions select the tracked synthetic fixture.
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, strings.TrimSuffix(path, ".private")+"."+extension, string(data))
	}
	s, err := LoadSigner("windows.example.", path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.create(strings.Repeat("a", 32), validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	txt, sig, key, err := w.Records()
	if err != nil {
		t.Fatal(err)
	}
	if key.KeyTag() != 0 || sig.KeyTag != 0 {
		t.Fatal("fixture loses key tag zero")
	}
	if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	infrastructure, err := s.SignInfrastructure([]dns.RR{key}, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if infrastructure.KeyTag != 0 {
		t.Fatal("infrastructure loses key tag zero")
	}
	if err := infrastructure.Verify(key, []dns.RR{key}, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	testConcurrentSignatures(t, s)
}

func TestSignerPreservesConcurrentSignatures(t *testing.T) {
	testConcurrentSignatures(t, testSigner(t))
}

func testConcurrentSignatures(t *testing.T, s *Signer) {
	t.Helper()
	for i := range 4 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			if _, err := s.create(strings.Repeat("a", 32), validInput(), testNow); err != nil {
				t.Error(err)
			}
			txt := makeTXT("event.windows.example.", "synthetic")
			sig, key, err := s.SignCurrentTXT(txt, testNow)
			if err != nil {
				t.Fatal(err)
			}
			if sig.KeyTag != key.KeyTag() {
				t.Fatal("concurrent signature changes the key tag")
			}
			if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
				t.Fatal(err)
			}
			public := s.PublicKey()
			infrastructure, err := s.SignInfrastructure([]dns.RR{public}, testNow)
			if err != nil {
				t.Fatal(err)
			}
			if err := infrastructure.Verify(public, []dns.RR{public}, &dns.SignOption{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSignerPreservesExplicitWindow(t *testing.T) {
	s := testSigner(t)
	in := validInput()
	in.Content = strings.Repeat("é", 128) + "\x00\n\"\\" + strings.Repeat("x", 252)
	w, err := s.create(strings.Repeat("a", 32), in, testNow)
	if err != nil {
		t.Fatal(err)
	}
	txt, sig, key, err := w.Records()
	if err != nil {
		t.Fatal(err)
	}
	var decoded strings.Builder
	if len(txt.Txt) != 3 {
		t.Fatalf("TXT chunks = %q", txt.Txt)
	}
	for _, chunk := range txt.Txt {
		raw := decodeTXT(t, chunk)
		decoded.WriteString(raw)
		if len(raw) > 255 {
			t.Fatal("TXT exceeds the wire limit")
		}
	}
	if decoded.String() != in.Content {
		t.Fatal("TXT changes the public content")
	}
	if int64(sig.Inception) != w.Inception.Unix() || int64(sig.Expiration) != w.Expiration.Unix() {
		t.Fatal("signature changes the boundaries")
	}
	if sig.ValidPeriod(testNow) || !sig.ValidPeriod(w.Inception) || !sig.ValidPeriod(w.Expiration) || sig.ValidPeriod(w.Expiration.Add(time.Second)) {
		t.Fatal("signature changes boundary validity")
	}
	if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	if w.Version != 1 || w.Name != "w"+w.ID+".windows.example." {
		t.Fatal("window changes the record identity")
	}
}

func decodeTXT(t *testing.T, escaped string) string {
	t.Helper()
	var raw strings.Builder
	for i := 0; i < len(escaped); i++ {
		if escaped[i] != '\\' {
			raw.WriteByte(escaped[i])
			continue
		}
		i++
		if escaped[i] < '0' || escaped[i] > '9' {
			raw.WriteByte(escaped[i])
			continue
		}
		n, err := strconv.ParseUint(escaped[i:i+3], 10, 8)
		if err != nil {
			t.Fatal(err)
		}
		raw.WriteByte(byte(n))
		i += 2
	}
	return raw.String()
}

func TestSignerRejectsUnsafeKeyFiles(t *testing.T) {
	for name, change := range map[string]func(*testing.T, string, *dns.DNSKEY) (string, string){
		"missing": func(_ *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			return "windows.example.", path + ".absent.private"
		},
		"permissions": func(t *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			} // #nosec G302 -- This case tests rejection of public private-key access.
			return "windows.example.", path
		},
		"symlink": func(t *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			link := filepath.Join(t.TempDir(), "link.private")
			if err := os.Symlink(path, link); err != nil {
				t.Fatal(err)
			}
			return "windows.example.", link
		},
		"directory": func(t *testing.T, _ string, _ *dns.DNSKEY) (string, string) {
			path := filepath.Join(t.TempDir(), "dir.private")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			return "windows.example.", path
		},
		"large": func(t *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			writeTestFile(t, path, strings.Repeat("x", 16385))
			return "windows.example.", path
		},
		"malformed": func(t *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			writeTestFile(t, path, "private secret content")
			return "windows.example.", path
		},
		"wrong zone": func(_ *testing.T, path string, _ *dns.DNSKEY) (string, string) { return "other.example.", path },
		"algorithm": func(t *testing.T, path string, key *dns.DNSKEY) (string, string) {
			key.Algorithm = dns.ED25519
			writeTestFile(t, strings.TrimSuffix(path, ".private")+".key", key.String())
			return "windows.example.", path
		},
		"mismatch": func(t *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			other, _ := testKeyFiles(t)
			data, err := os.ReadFile(other) // #nosec G304 -- The test creates this temporary synthetic file.
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, string(data))
			return "windows.example.", path
		},
		"wrong extension": func(_ *testing.T, path string, _ *dns.DNSKEY) (string, string) {
			return "windows.example.", strings.TrimSuffix(path, ".private") + ".key"
		},
		"bad zone": func(_ *testing.T, path string, _ *dns.DNSKEY) (string, string) { return "example.", path },
	} {
		t.Run(name, func(t *testing.T) {
			path, key := testKeyFiles(t)
			zone, path := change(t, path, key)
			if _, err := LoadSigner(zone, path); err == nil || strings.Contains(err.Error(), "private secret content") {
				t.Fatalf("unsafe key result: %v", err)
			}
		})
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	// Each caller selects a temporary synthetic file.
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	} // #nosec G703
}

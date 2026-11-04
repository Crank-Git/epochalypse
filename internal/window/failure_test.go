package window

import (
	"crypto/ecdsa"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeberg.org/miekg/dns"
	bolt "go.etcd.io/bbolt"
)

func fixtureWindow(t *testing.T) Window {
	t.Helper()
	data, err := os.ReadFile("testdata/window-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var w Window
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestVersionOneFixturePreservesTheSignature(t *testing.T) {
	w := fixtureWindow(t)
	txt, sig, key, err := w.Records()
	if err != nil {
		t.Fatal(err)
	}
	if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		t.Fatal(err)
	}
	if sig.ValidPeriod(testNow) {
		t.Fatal("fixture has a currently valid alternate signature")
	}
}

func TestRecordsRejectAlteredState(t *testing.T) {
	for name, change := range map[string]func(*Window){
		"version":            func(w *Window) { w.Version = 2 },
		"name":               func(w *Window) { w.Name = "other.windows.example." },
		"time":               func(w *Window) { w.Inception = w.Expiration },
		"content":            func(w *Window) { w.Content = "changed" },
		"TXT type":           func(w *Window) { w.TXT = "other.windows.example. 60 IN A 192.0.2.1" },
		"TXT format":         func(w *Window) { w.TXT = "malformed" },
		"signature type":     func(w *Window) { w.RRSIG = w.TXT },
		"key type":           func(w *Window) { w.DNSKEY = w.TXT },
		"signature boundary": func(w *Window) { w.RRSIG = strings.Replace(w.RRSIG, "20261104130000", "20261104120000", 1) },
		"signature content":  func(w *Window) { w.RRSIG = w.RRSIG[:len(w.RRSIG)-4] + "AAAA" },
	} {
		t.Run(name, func(t *testing.T) {
			w := fixtureWindow(t)
			change(&w)
			if _, _, _, err := w.Records(); err == nil {
				t.Fatal("altered window passes verification")
			}
		})
	}
}

func TestSignerRejectsMalformedPublicKey(t *testing.T) {
	path, _ := testKeyFiles(t)
	writeTestFile(t, strings.TrimSuffix(path, ".private")+".key", "malformed")
	if _, err := LoadSigner("windows.example.", path); err == nil {
		t.Fatal("malformed public key passes")
	}
}

func TestSignerRejectsInvalidIdentity(t *testing.T) {
	if _, err := testSigner(t).create("invalid", validInput(), testNow); err == nil {
		t.Fatal("invalid identifier passes")
	}
}

func TestZoneRejectsInvalidNames(t *testing.T) {
	for _, zone := range []string{"", "EXAMPLE.test.", "windows.example", "-a.example.", "a-.example.", "a..example.", "a_b.example.", strings.Repeat("a", 64) + ".example.", strings.Repeat("a", 201) + "."} {
		if validZone(zone) {
			t.Errorf("invalid zone passes: %q", zone)
		}
	}
}

func TestSignerRejectsInvalidPrivateScalars(t *testing.T) {
	if matchingKey(&ecdsa.PrivateKey{}) {
		t.Fatal("unsupported curve passes")
	}
	path, _ := testKeyFiles(t)
	writeTestFile(t, path, "Private-key-format: v1.3\nAlgorithm: 13 (ECDSAP256SHA256)\nPrivateKey: AA==\n")
	if _, err := LoadSigner("windows.example.", path); err == nil {
		t.Fatal("zero scalar passes")
	}
}

func TestStoreRejectsUnsafeStatePaths(t *testing.T) {
	signer := testSigner(t)
	if _, err := Open(t.TempDir(), nil); err == nil {
		t.Fatal("absent signer passes")
	}
	for _, kind := range []string{"file parent", "public database", "directory database", "symlink database", "corrupt database"} {
		t.Run(kind, func(t *testing.T) {
			state := t.TempDir()
			prepareUnsafeState(t, state, kind)
			if store, err := Open(state, signer); err == nil {
				_ = store.Close()
				t.Fatal("unsafe state passes")
			}
		})
	}
}

func prepareUnsafeState(t *testing.T, state, kind string) {
	t.Helper()
	dir := filepath.Join(state, "v1")
	if kind == "file parent" {
		writeTestFile(t, dir, "file")
		return
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "windows.db")
	switch kind {
	case "public database":
		writeTestFile(t, path, "")
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		} // #nosec G302 -- This case tests rejection of public database access.
	case "directory database":
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	case "symlink database":
		target := filepath.Join(state, "target")
		writeTestFile(t, target, "")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	case "corrupt database":
		writeTestFile(t, path, strings.Repeat("invalid", 2048))
	}
}

func TestStoreRejectsAChangedKey(t *testing.T) {
	state := t.TempDir()
	s, err := Open(state, testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if changed, err := Open(state, testSigner(t)); err == nil {
		_ = changed.Close()
		t.Fatal("changed key passes")
	}
}

func TestStoreRejectsClosedDatabase(t *testing.T) {
	s, err := Open(t.TempDir(), testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(validInput(), testNow); err == nil {
		t.Fatal("closed database accepts creation")
	}
	if _, err := s.Get(strings.Repeat("a", 32)); err == nil {
		t.Fatal("closed database accepts read")
	}
	if _, err := s.List(); err == nil {
		t.Fatal("closed database accepts list")
	}
}

func TestStoreRejectsCorruptDocuments(t *testing.T) {
	s, err := Open(t.TempDir(), testSigner(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	w, err := s.Create(validInput(), testNow)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"invalid", `{"version":1}`, strings.Replace(marshalWindow(t, w), `"version":1`, `"version":2`, 1)} {
		if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(windowBucket).Put([]byte(w.ID), []byte(data)) }); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(w.ID); err == nil {
			t.Fatal("corrupt window passes read")
		}
		if _, err := s.List(); err == nil {
			t.Fatal("corrupt window passes list")
		}
	}
}

func marshalWindow(t *testing.T, w Window) string {
	t.Helper()
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

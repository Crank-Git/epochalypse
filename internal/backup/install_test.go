package backup

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func installationSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"state", "config", "keys", "tls"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range []struct{ name, zone string }{{"window", "windows.example."}, {"countdown", "events.example."}} {
		key := dns.NewDNSKEY(pair.zone, dns.ECDSAP256SHA256)
		key.Flags = 257
		key.Hdr.TTL = 60
		private, err := key.Generate(256)
		if err != nil {
			t.Fatal(err)
		}
		writeInstallationFile(t, root, "keys/"+pair.name+".private", []byte(key.PrivateKeyString(private)))
		writeInstallationFile(t, root, "keys/"+pair.name+".key", []byte(key.String()))
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{DNSNames: []string{"main.example"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallationFile(t, root, "tls/fullchain.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	writeInstallationFile(t, root, "tls/private.pem", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}))
	writeInstallationFile(t, root, "config/main.json", []byte(`{"version":1,"zone":"windows.example.","countdown_zone":"events.example.","nameserver":"ns.example.","public_url":"https://main.example"}`))
	o, err := owner.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Initialize([]byte("synthetic backup password")); err != nil {
		t.Fatal(err)
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := installation.Open(filepath.Join(root, "state"), installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := window.LoadSigner("windows.example.", filepath.Join(root, "keys/window.private"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := window.Attach(db, signer); err != nil {
		t.Fatal(err)
	}
	if _, err := capsule.Attach(db, capsule.DefaultPlaintextLimit); err != nil {
		t.Fatal(err)
	}
	if _, err := countdown.Attach(db, "events.example."); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeInstallationFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func installationArchive(t *testing.T, change func(string)) Archive {
	t.Helper()
	root := installationSource(t)
	if change != nil {
		change(root)
	}
	a := Archive{Version: 1, Commit: strings.Repeat("a", 40), CreatedAt: time.Now().UTC(), MaxObjects: 100, QuotaBytes: 1 << 30}
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		a.Files = append(a.Files, File{Path: path, UID: os.Getuid(), GID: os.Getgid(), SHA256: hex.EncodeToString(digest[:]), Size: int64(len(data)), SourcePath: filepath.Join(root, path)})
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext bytes.Buffer
	if err := Seal(context.Background(), &ciphertext, a, identity.Recipient(), testLimits(t)); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(context.Background(), &ciphertext, identity, testLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func restoreDestination(t *testing.T, a Archive) Destination {
	t.Helper()
	return Destination{Path: filepath.Join(t.TempDir(), "restored"), Commit: a.Commit, UID: os.Getuid(), GID: os.Getgid()}
}

func TestInstallPublishesAuthenticatedInstallation(t *testing.T) {
	a := installationArchive(t, nil)
	destination := restoreDestination(t, a)
	if err := a.Install(context.Background(), destination, testLimits(t)); err != nil {
		t.Fatal(err)
	}
	for _, file := range a.Files {
		data, err := os.ReadFile(filepath.Join(destination.Path, file.Path))
		if err != nil || !bytes.Equal(data, readContent(t, file)) {
			t.Fatalf("installed file differs: %v", err)
		}
	}
}

func TestInstallRejectsInvalidStateBeforePublication(t *testing.T) {
	for _, name := range []string{"settings", "key", "tls", "owner", "installation"} {
		t.Run(name, func(t *testing.T) {
			a := installationArchive(t, func(root string) {
				path := map[string]string{"settings": "config/main.json", "key": "keys/window.private", "tls": "tls/private.pem", "owner": "state/v1/owner.db", "installation": "state/v1/windows.db"}[name]
				writeInstallationFile(t, root, path, []byte("synthetic invalid secret"))
			})
			destination := restoreDestination(t, a)
			if err := a.Install(context.Background(), destination, testLimits(t)); err == nil {
				t.Fatal("restore installs invalid authenticated state")
			} else if strings.Contains(err.Error(), "synthetic invalid secret") {
				t.Fatal("restore error exposes stored content")
			}
			if _, err := os.Lstat(destination.Path); !os.IsNotExist(err) {
				t.Fatal("restore leaves invalid destination")
			}
		})
	}
}

func TestInstallRequiresMatchingExecutableCommit(t *testing.T) {
	a := installationArchive(t, nil)
	destination := restoreDestination(t, a)
	destination.Commit = strings.Repeat("b", 40)
	if err := a.Install(context.Background(), destination, testLimits(t)); err == nil {
		t.Fatal("restore accepts a different executable commit")
	}
}

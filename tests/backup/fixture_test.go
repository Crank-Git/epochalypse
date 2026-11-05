package backupfixture

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"filippo.io/age"
	backup "github.com/Crank-Git/epochalypse/internal/backup"
	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
)

type recoveryFixture struct {
	Commit          string           `json:"commit"`
	Password        string           `json:"password"`
	Window          window.Window    `json:"window"`
	Countdown       countdown.Record `json:"countdown"`
	CountdownKeyTag uint16           `json:"countdown_key_tag"`
	CapsuleID       string           `json:"capsule_id"`
	CapsulePlain    string           `json:"capsule_plaintext"`
}

// TestWritesSyntheticRecoveryInstallation creates source files for the Linux recovery exercise.
func TestWritesSyntheticRecoveryInstallation(t *testing.T) {
	base := os.Getenv("EPOCHALYPSE_BACKUP_FIXTURE_DIR")
	if base == "" {
		base = t.TempDir()
	}
	commit := os.Getenv("EPOCHALYPSE_BACKUP_FIXTURE_COMMIT")
	if len(commit) != 40 {
		if commit != "" {
			t.Fatal("fixture requires the full source commit")
		}
		commit = strings.Repeat("a", 40)
	}
	root := filepath.Join(base, "installation")
	for _, name := range []string{"state", "config", "keys", "tls"} {
		backupFixtureDirectory(t, filepath.Join(root, name))
	}
	const windowZone = "windows.backup.invalid."
	const countdownZone = "countdowns.backup.invalid."
	backupFixtureFile(t, root, "config/main.json", []byte(`{"version":1,"zone":"windows.backup.invalid.","countdown_zone":"countdowns.backup.invalid.","nameserver":"ns.backup.invalid.","public_url":"https://localhost:8080"}`))
	backupFixtureDNSKey(t, root, "window", windowZone)
	countdownKeyTag := backupFixtureDNSKey(t, root, "countdown", countdownZone)
	backupFixtureTLS(t, root)
	for _, name := range []string{"lab", "agents"} {
		backupFixtureDirectory(t, filepath.Join(root, name))
	}
	backupFixtureFile(t, root, "lab/owner.initialized", []byte("synthetic setup marker"))
	backupFixtureFile(t, root, "agents/state.db", []byte("synthetic agent state"))

	password := createBackupFixtureOwner(t, root)
	now := time.Now().UTC().Truncate(time.Second)
	windowRecord := createBackupFixtureWindow(t, root, windowZone, now)
	countdownRecord := createBackupFixtureCountdown(t, root, countdownZone, now)
	capsuleRecord, plaintext := createBackupFixtureCapsule(t, root, now)

	fixture := recoveryFixture{Commit: commit, Password: password, Window: windowRecord, Countdown: countdownRecord, CountdownKeyTag: countdownKeyTag, CapsuleID: capsuleRecord.ID, CapsulePlain: plaintext}
	backupFixtureJSON(t, filepath.Join(base, "expected.json"), fixture)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	backupFixtureDirectory(t, filepath.Join(base, "credentials"))
	backupFixtureBytes(t, filepath.Join(base, "credentials/identity"), []byte(identity.String()+"\n"))
	backupFixtureBytes(t, filepath.Join(base, "credentials/recipient"), []byte(identity.Recipient().String()+"\n"))

	archive, err := backup.Snapshot(context.Background(), backup.Source{Root: root, Commit: commit, MaxObjects: 100, QuotaBytes: 1 << 30}, backup.Limits{MaxBytes: backup.DefaultMaxBytes})
	if err != nil {
		t.Fatalf("snapshot the synthetic installation: %v", err)
	}
	if len(archive.Files) != 9 {
		t.Fatalf("snapshot contains %d files, want 9", len(archive.Files))
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close the synthetic snapshot: %v", err)
	}
}

func createBackupFixtureOwner(t *testing.T, root string) string {
	t.Helper()
	store, err := owner.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	password := "synthetic-backup-password"
	if err := store.Initialize([]byte(password)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return password
}

func createBackupFixtureWindow(t *testing.T, root, zone string, now time.Time) window.Window {
	t.Helper()
	store, err := window.Open(filepath.Join(root, "state"), backupFixtureSigner(t, zone, root))
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(window.Input{
		Content:    "synthetic recovery window",
		Inception:  now.Add(time.Hour).Format(time.RFC3339),
		Expiration: now.Add(2 * time.Hour).Format(time.RFC3339),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return record
}

func createBackupFixtureCountdown(t *testing.T, root, zone string, now time.Time) countdown.Record {
	t.Helper()
	store, err := countdown.Open(filepath.Join(root, "state"), installation.DefaultLimits(), zone)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(context.Background(), countdown.Input{
		Title: "Synthetic recovery event", EventAt: now.Add(2 * time.Hour).Format(time.RFC3339),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return record
}

func createBackupFixtureCapsule(t *testing.T, root string, now time.Time) (capsule.Manifest, string) {
	t.Helper()
	store, err := capsule.Open(filepath.Join(root, "state"), installation.DefaultLimits(), capsule.DefaultPlaintextLimit)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := "synthetic released capsule content"
	created := now.Add(-time.Hour)
	record, err := store.Create(capsule.Input{
		Mode: capsule.ServerHeld, Kind: capsule.Text, Title: "Synthetic recovery capsule",
		Filename: "recovery.txt", ReleaseAt: created.Add(30 * time.Minute),
	}, strings.NewReader(plaintext), created)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(record.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return record.Manifest, plaintext
}

func backupFixtureSigner(t *testing.T, zone, root string) *window.Signer {
	t.Helper()
	signer, err := window.LoadSigner(zone, filepath.Join(root, "keys/window.private"))
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func backupFixtureDNSKey(t *testing.T, root, name, zone string) uint16 {
	t.Helper()
	key := dns.NewDNSKEY(zone, dns.ECDSAP256SHA256)
	key.Flags = 257
	key.Hdr.TTL = 60
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	backupFixtureFile(t, root, "keys/"+name+".private", []byte(key.PrivateKeyString(private)))
	backupFixtureFile(t, root, "keys/"+name+".key", []byte(key.String()))
	return key.KeyTag()
}

func backupFixtureTLS(t *testing.T, root string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cert := &x509.Certificate{DNSNames: []string{"localhost"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	backupFixtureFile(t, root, "tls/fullchain.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	backupFixtureFile(t, root, "tls/private.pem", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}))
}

func backupFixtureDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil { // #nosec G703 -- The test runner selects the synthetic fixture root.
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil { // #nosec G302 G703 -- The test runner selects private directories that require owner traversal.
		t.Fatal(err)
	}
}

func backupFixtureFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	backupFixtureBytes(t, filepath.Join(root, name), data)
}

func backupFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	backupFixtureBytes(t, path, append(data, '\n'))
}

func backupFixtureBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil { // #nosec G703 -- The test runner selects the synthetic fixture root.
		t.Fatal(err)
	}
}

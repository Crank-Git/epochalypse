package command

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/owner"
)

func TestBackupCreateReportsMissingRecipient(t *testing.T) {
	var out, errOut bytes.Buffer
	args := []string{"backup", "create", "--source", t.TempDir(), "--output", "unused.age", "--recipient-file", "missing-recipient", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--max-objects", "100", "--quota-bytes", "1073741824"}
	if code := Run(args, &out, &errOut); code != 1 || out.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &errOut)
	}
}

func TestBackupRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"backup"}, {"backup", "unknown"}, {"backup", "create"}, {"backup", "restore"},
		{"backup", "create", "--unknown"}, {"backup", "restore", "--uid", "bad"},
		{"backup", "create", "extra"},
	} {
		if code := Run(args, io.Discard, io.Discard); code != 2 {
			t.Fatalf("%q returns %d", args, code)
		}
	}
}

func backupCommandSource(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	o, err := owner.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Initialize([]byte("synthetic backup password")); err != nil {
		t.Fatal(err)
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := installation.Open(state, installation.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"config/main.json", "keys/window.private", "keys/window.key", "keys/countdown.private", "keys/countdown.key", "tls/fullchain.pem", "tls/private.pem"} {
		file := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("synthetic invalid installation"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestBackupCreateEncryptsStoppedSource(t *testing.T) {
	root := backupCommandSource(t)
	dir := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	recipient := filepath.Join(dir, "recipient")
	if err := os.WriteFile(recipient, []byte(identity.Recipient().String()+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "backup.age")
	args := []string{"backup", "create", "--source", root, "--output", output, "--recipient-file", recipient, "--commit", strings.Repeat("a", 40), "--max-objects", "100", "--quota-bytes", "1073741824", "--temp-dir", dir}
	var out, diagnostic bytes.Buffer
	if code := Run(args, &out, &diagnostic); code != 0 {
		t.Fatalf("create returns %d: %s", code, &diagnostic)
	}
	if !strings.Contains(out.String(), `"max_objects":100`) || !strings.Contains(out.String(), `"quota_bytes":1073741824`) {
		t.Fatal("source limits absent")
	}
	data, err := os.ReadFile(output) // #nosec G304 -- The test creates this private archive path.
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("synthetic")) {
		t.Fatal("plaintext enters encrypted output")
	}
	if code := Run(args, io.Discard, io.Discard); code != 1 {
		t.Fatal("replaces existing archive")
	}
	args[5] = filepath.Join(dir, "output-fails.age")
	if code := Run(args, failedWriter{}, io.Discard); code != 1 {
		t.Fatal("reports output success")
	}
	args[3] = filepath.Join(dir, "missing-source")
	if code := Run(args, io.Discard, io.Discard); code != 1 {
		t.Fatal("accepts missing source")
	}
	checkBackupRestoreFailures(t, dir, output, identity)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	args[3] = root
	if code := runBackup(ctx, args, io.Discard, io.Discard); code != 1 {
		t.Fatal("ignores cancellation")
	}
}

func checkBackupRestoreFailures(t *testing.T, dir, output string, identity *age.X25519Identity) {
	t.Helper()
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(identityFile, []byte(identity.String()), 0600); err != nil {
		t.Fatal(err)
	}
	restore := []string{"backup", "restore", "--input", output, "--destination", filepath.Join(dir, "restored"), "--identity-file", identityFile, "--commit", strings.Repeat("a", 40), "--uid", strconv.Itoa(os.Getuid()), "--gid", strconv.Itoa(os.Getgid())}
	if code := Run(restore, io.Discard, io.Discard); code != 1 {
		t.Fatal("installs invalid configuration")
	}
	if _, err := os.Stat(filepath.Join(dir, "restored")); !os.IsNotExist(err) {
		t.Fatal("publishes invalid installation")
	}
	restore[3] = filepath.Join(dir, "missing.age")
	if code := Run(restore, io.Discard, io.Discard); code != 1 {
		t.Fatal("accepts absent input")
	}
	if err := os.WriteFile(output, []byte("corrupt archive"), 0600); err != nil {
		t.Fatal(err)
	}
	restore[3] = output
	if code := Run(restore, io.Discard, io.Discard); code != 1 {
		t.Fatal("accepts corrupt input")
	}
}

func TestBackupRejectsKeyDisclosure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	secret := "synthetic-secret-that-must-not-enter-errors"
	for _, action := range []string{"create", "restore"} {
		if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"backup", "create", "--source", dir, "--output", "unused.age", "--recipient-file", path, "--commit", strings.Repeat("a", 40), "--max-objects", "1", "--quota-bytes", "1"}
		if action == "restore" {
			args = []string{"backup", "restore", "--input", "unused.age", "--destination", "fresh", "--identity-file", path, "--commit", strings.Repeat("a", 40), "--uid", "0", "--gid", "0"}
		}
		var out, diagnostic bytes.Buffer
		if code := Run(args, &out, &diagnostic); code != 1 || out.Len() != 0 || strings.Contains(diagnostic.String(), secret) {
			t.Fatalf("key error exposes input: code=%d %s %s", code, &out, &diagnostic)
		}
	}
}

func TestBackupKeyRequiresProtectedBoundedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 1025)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBackupKey(path); err == nil {
		t.Fatal("reads oversized key")
	}
	if err := os.Chmod(path, 0644); err != nil { // #nosec G302 -- The test requires an unsafe key mode.
		t.Fatal(err)
	}
	if _, err := readBackupKey(path); err == nil {
		t.Fatal("reads public key file")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{link, dir, filepath.Join(dir, "absent")} {
		if _, err := readBackupKey(name); err == nil {
			t.Fatalf("reads invalid key %s", name)
		}
	}
}

func TestBackupRestoreReportsMissingIdentity(t *testing.T) {
	var out, errOut bytes.Buffer
	args := []string{"backup", "restore", "--input", "missing.age", "--destination", "fresh", "--identity-file", "missing-identity", "--commit", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--uid", "0", "--gid", "0"}
	if code := Run(args, &out, &errOut); code != 1 || out.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &errOut)
	}
}

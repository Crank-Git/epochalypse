package command

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
)

func TestCapsuleCommandsRecoverAfterLocalDeletion(t *testing.T) {
	dir := t.TempDir()
	record := recoveryCommandRecord(t, dir, capsule.Input{Mode: capsule.Public, Kind: capsule.File, Filename: "synthetic.bin", ReleaseAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second)}, []byte{0, 255, 128, 1}, time.Now())
	bundle, output := filepath.Join(dir, "bundle.json"), filepath.Join(dir, "content.bin")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"capsule", "export", "--state-dir", dir, "--id", record.ID, "--output", bundle}, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "released-key") {
		t.Fatal("export omits key distinction", stdout.String())
	}
	stdout.Reset()
	if code := Run([]string{"capsule", "delete", "--state-dir", dir, "--id", record.ID, "--confirm", record.ID}, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if code := Run([]string{"capsule", "open", "--bundle", bundle, "--output", output}, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	data, err := os.ReadFile(output) // #nosec G304 -- The test owns this temporary file.
	if err != nil || !bytes.Equal(data, []byte{0, 255, 128, 1}) {
		t.Fatal("recovered bytes differ", err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("output permissions differ", err)
	}
	if code := Run([]string{"capsule", "open", "--bundle", bundle, "--output", output}, &stdout, &stderr); code == 0 {
		t.Fatal("open replaces an existing output")
	}
	data, err = os.ReadFile(bundle) // #nosec G304 -- The test owns this temporary file.
	if err != nil {
		t.Fatal(err)
	}
	var decoded capsule.RecoveryBundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Record.ObjectSHA256 = strings.Repeat("0", 64)
	data, err = json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, data, 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "must-not-exist")
	if code := Run([]string{"capsule", "open", "--bundle", bad, "--output", missing}, &stdout, &stderr); code == 0 {
		t.Fatal("corrupt bundle succeeds")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("corrupt bundle exposes an output")
	}
}

func TestCapsuleRecoveryCommandsRejectInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"capsule", "export"}, {"capsule", "open"}, {"capsule", "delete"},
		{"capsule", "export", "--bad"},
		{"capsule", "export", "--id", "invalid", "--output", filepath.Join(dir, "output"), "--state-dir", dir},
		{"capsule", "export", "--id", strings.Repeat("a", 32), "--output", filepath.Join(dir, "output"), "--state-dir", dir},
		{"capsule", "export", "--id", "invalid", "--output", "out", "--plaintext-limit", "0"},
		{"capsule", "export", "--id", "invalid", "--output", "out", "--max-objects", "invalid"},
		{"capsule", "delete", "--id", "invalid", "--confirm", "yes"},
		{"capsule", "delete", "--id", "invalid", "--confirm", "invalid", "--state-dir", dir},
		{"capsule", "open", "--bundle", dir + "missing", "--output", filepath.Join(dir, "output")},
		{"capsule", "open", "--bundle", dir, "--output", filepath.Join(dir, "output")},
		{"capsule", "open", "--bundle", dir, "--output", filepath.Join(dir, "output"), "extra"},
	} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code == 0 || out.Len() != 0 {
			t.Fatal("invalid recovery command passes", args, code)
		}
	}
	if err := writeRecoveryFile(filepath.Join(dir, "missing", "output"), []byte("secret")); err == nil {
		t.Fatal("absent output directory passes")
	}
	if _, err := recoveryBeacon(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing signature passes")
	}
	signature := filepath.Join(dir, "signature")
	if err := os.WriteFile(signature, make([]byte, 49), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := recoveryBeacon(signature); err == nil {
		t.Fatal("oversized signature passes")
	}
}

func TestCapsuleCommandOpensOfflineTimelockAfterDeletion(t *testing.T) {
	dir := t.TempDir()
	release := time.Unix(1692806364, 0).UTC()
	record := recoveryCommandRecord(t, dir, capsule.Input{Mode: capsule.Tlock, Kind: capsule.Text, ReleaseAt: release}, []byte("offline recovery"), release.Add(-time.Hour))
	bundle, output := filepath.Join(dir, "bundle.json"), filepath.Join(dir, "output")
	var out, stderr bytes.Buffer
	if code := Run([]string{"capsule", "export", "--state-dir", dir, "--id", record.ID, "--output", bundle}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(out.String(), "beacon-wrapper") {
		t.Fatal("tlock export omits its recovery requirement")
	}
	if code := Run([]string{"capsule", "delete", "--state-dir", dir, "--id", record.ID, "--confirm", record.ID}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	data, err := os.ReadFile("../../tests/compatibility/tlock/testdata/quicknet-1000.json")
	if err != nil {
		t.Fatal(err)
	}
	var beacon struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(data, &beacon); err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(beacon.Signature)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "signature")
	if err := os.WriteFile(path, signature, 0600); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"capsule", "open", "--bundle", bundle, "--output", output, "--beacon-signature-file", path}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	plaintext, err := os.ReadFile(output) // #nosec G304 -- The test owns this temporary file.
	if err != nil || string(plaintext) != "offline recovery" {
		t.Fatal("offline content differs", err)
	}
	signature[0] ^= 1
	if err := os.WriteFile(path, signature, 0600); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"capsule", "open", "--bundle", bundle, "--output", output + "bad", "--beacon-signature-file", path}, &out, &stderr); code == 0 {
		t.Fatal("invalid signature passes")
	}
	if _, err := os.Stat(output + "bad"); !os.IsNotExist(err) {
		t.Fatal("invalid signature exposes output")
	}
}

func recoveryCommandRecord(t *testing.T, dir string, input capsule.Input, content []byte, now time.Time) capsule.Record {
	t.Helper()
	s, err := capsule.Open(dir, installation.DefaultLimits(), 32)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	record, err := s.Create(input, bytes.NewReader(content), now)
	if err != nil {
		t.Fatal(err)
	}
	record, err = s.Publish(record.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

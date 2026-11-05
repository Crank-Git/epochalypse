package command

import (
	"bytes"
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
	s, err := capsule.Open(dir, installation.DefaultLimits(), 32)
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Create(capsule.Input{Mode: capsule.Public, Kind: capsule.File, Filename: "synthetic.bin", ReleaseAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second)}, bytes.NewReader([]byte{0, 255, 128, 1}), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(record.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
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
	data, err := os.ReadFile(output)
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
	data, err = os.ReadFile(bundle)
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

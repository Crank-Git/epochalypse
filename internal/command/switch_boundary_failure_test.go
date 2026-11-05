package command

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/installation"
	"github.com/Crank-Git/epochalypse/internal/switchclient"
	"github.com/Crank-Git/epochalypse/internal/switchcontent"
	"github.com/Crank-Git/epochalypse/internal/switches"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"github.com/Crank-Git/epochalypse/internal/window"
)

func armCommandFixture(t *testing.T) (armOptions, switchclient.Plan, ed25519.PrivateKey) {
	t.Helper()
	dir := t.TempDir()
	draft, err := executeSwitch(filepath.Join(dir, "state"), "", switches.Input{}, window.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	agents := prepareCommandAgents(t)
	material, err := switchcontent.Prepare(strings.NewReader("synthetic protected content"), [3]string{agents[0].Recipient, agents[1].Recipient, agents[2].Recipient})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := switchclient.NewPlan(draft, material, agents, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	options := armOptions{stateDir: filepath.Join(dir, "state"), planPath: filepath.Join(dir, "plan"), keyPath: filepath.Join(dir, "key"), pinsPath: filepath.Join(dir, "pins"), bundlePath: filepath.Join(dir, "bundle")}
	if err := switchclient.WritePlan(options.planPath, plan, key); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, options.keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600)
	writeFixtureFile(t, options.pinsPath, []byte(`{"https://unrelated.invalid":"`+strings.Repeat("01", 32)+`"}`), 0o600)
	return options, plan, key
}

func armCommandArguments(o armOptions) []string {
	return []string{"switch", "arm", "--state-dir", o.stateDir, "--plan", o.planPath, "--owner-key", o.keyPath, "--pins", o.pinsPath, "--bundle", o.bundlePath}
}

func TestSwitchArmRejectsUnavailableOrDifferentDraft(t *testing.T) {
	for _, name := range []string{"missing", "locked", "different interval", "unreachable replicas"} {
		t.Run(name, func(t *testing.T) {
			o, p, key := armCommandFixture(t)
			switch name {
			case "missing":
				o.stateDir = filepath.Join(t.TempDir(), "empty")
			case "locked":
				db, err := installation.Open(o.stateDir, installation.Limits(window.DefaultLimits()))
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
			}
			if name == "different interval" {
				changeArmFixtureInterval(t, o, p, key)
			}
			var stdout, stderr bytes.Buffer
			if code := Run(armCommandArguments(o), &stdout, &stderr); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
				t.Fatalf("failed arm: code=%d out=%q error=%q", code, &stdout, &stderr)
			}
			if bytes.Contains(stderr.Bytes(), p.Ciphertext) || strings.Contains(stderr.String(), base64.StdEncoding.EncodeToString(key)) {
				t.Fatal("arm diagnostic exposes protected input")
			}
			if _, err := os.Stat(o.bundlePath); !os.IsNotExist(err) {
				t.Fatalf("failed prepare creates export: %v", err)
			}
		})
	}
}

func changeArmFixtureInterval(t *testing.T, o armOptions, p switchclient.Plan, key ed25519.PrivateKey) {
	t.Helper()
	enrollment, err := switchwire.AdmitCertificate(p.Certificate, p.CertificateSignature, key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := switchwire.DecodeFrame(p.Prepare.Body)
	if err != nil {
		t.Fatal(err)
	}
	certificate := switchwire.Certificate{Version: switchwire.Version, OwnerKey: enrollment.OwnerKey, Agents: enrollment.Agents, Key: frame.State.Key, Content: frame.State.Content, Grace: frame.State.Grace, Interval: 3600}
	p.Certificate, err = switchwire.EncodeCertificate(certificate)
	if err != nil {
		t.Fatal(err)
	}
	p.CertificateSignature = ed25519.Sign(key, p.Certificate)
	frame.State.Enrollment = sha256.Sum256(p.Certificate)
	p.Prepare, err = switchwire.SignFrame(frame, key)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, o.planPath, data, 0o600)
}

func TestSwitchArmRejectsChangedSignedPlanBeforeTransmission(t *testing.T) {
	for _, name := range []string{"enrollment", "signature", "content", "frame", "missing pins"} {
		t.Run(name, func(t *testing.T) {
			o, p, _ := armCommandFixture(t)
			switch name {
			case "enrollment":
				p.CertificateSignature[0] ^= 1
			case "signature":
				p.Prepare.Signature[0] ^= 1
			case "content":
				p.Ciphertext[0] ^= 1
			case "frame":
				p.Prepare.Body = []byte("malformed")
			case "missing pins":
				o.pinsPath += ".missing"
			}
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, o.planPath, data, 0o600)
			var stdout, stderr bytes.Buffer
			if code := Run(armCommandArguments(o), &stdout, &stderr); code != 1 || stdout.Len() != 0 {
				t.Fatalf("changed plan exit=%d out=%q error=%q", code, &stdout, &stderr)
			}
		})
	}
}

func TestSwitchExportRejectsMissingOrDamagedBundle(t *testing.T) {
	for _, name := range []string{"missing key", "missing bundle", "damaged bundle", "flags", "missing flags"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			public, _, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			keyPath, bundlePath := filepath.Join(dir, "key"), filepath.Join(dir, "bundle")
			writeFixtureFile(t, keyPath, []byte(base64.StdEncoding.EncodeToString(public)), 0o600)
			writeFixtureFile(t, bundlePath, []byte("malformed recovery bundle"), 0o600)
			if name == "missing key" {
				keyPath += ".missing"
			}
			if name == "missing bundle" {
				bundlePath += ".missing"
			}
			args := []string{"switch", "export", "--bundle", bundlePath, "--owner-public-key", keyPath, "--output", filepath.Join(dir, "export")}
			want := 1
			if name == "flags" {
				args = append(args, "--unknown")
				want = 2
			}
			if name == "missing flags" {
				args = []string{"switch", "export"}
				want = 2
			}
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != want || stdout.Len() != 0 {
				t.Fatalf("export exit=%d out=%q error=%q", code, &stdout, &stderr)
			}
		})
	}
}

package service

import (
	"codeberg.org/miekg/dns"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsAcceptSeparateCountdownZone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"countdown_zone":"countdown.example."}`), 0600); err != nil {
		t.Fatal(err)
	}
	if settings, err := ReadSettings(path); err != nil || settings.CountdownZone != "countdown.example." {
		t.Fatalf("the separate countdown zone fails: %v", err)
	}
}

func TestCountdownConfigurationRejectsSharedNamespace(t *testing.T) {
	base := Config{Mode: "server", HTTPAddr: "127.0.0.1:0", DNSAddr: "127.0.0.1:0", StateDir: t.TempDir(), LocalLab: true, Environment: "lab", CountdownZone: "countdown.example.", CountdownKeyFile: "countdown.private"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"countdown.example.", "example.", "nested.countdown.example."} {
		c := base
		c.Zone, c.KeyFile = zone, "windows.private"
		if err := c.Validate(); err == nil {
			t.Fatalf("overlapping zone passes: %s", zone)
		}
	}
	for _, path := range []string{"countdown.private", filepath.Join(".", "countdown.private")} {
		c := base
		c.Zone = "windows.example."
		c.KeyFile, _ = filepath.Abs(path)
		if err := c.Validate(); err == nil {
			t.Fatal("the same key reference passes")
		}
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.CountdownKeyFile = "" },
		func(c *Config) { c.CountdownZone = "" },
		func(c *Config) { c.CountdownZone = "COUNTDOWN.example." },
		func(c *Config) { c.Mode = "dns" },
		func(c *Config) { c.DNSAddr = "" },
		func(c *Config) { c.NameServer = "ns.countdown.example." },
		func(c *Config) { c.Environment = "production" },
		func(c *Config) { c.LocalLab = false },
	} {
		c := base
		change(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("invalid countdown configuration passes: %+v", c)
		}
	}
}

func TestCountdownStateRejectsReusedSigningMaterial(t *testing.T) {
	windowKey := filepath.Join(t.TempDir(), "synthetic.private")
	countdownKey := filepath.Join(t.TempDir(), "synthetic.private")
	key := dns.NewDNSKEY("windows.example.", dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{windowKey: key.PrivateKeyString(private), countdownKey: key.PrivateKeyString(private), strings.TrimSuffix(windowKey, ".private") + ".key": key.String(), strings.TrimSuffix(countdownKey, ".private") + ".key": strings.ReplaceAll(key.String(), "windows.example.", "countdown.example.")} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c := Config{StateDir: t.TempDir(), Zone: "windows.example.", KeyFile: windowKey, CountdownZone: "countdown.example.", CountdownKeyFile: countdownKey, Environment: "lab"}
	if s, err := openMainState(c); err == nil {
		s.close()
		t.Fatal("the countdown authority reuses the window signing material")
	}
}

package service

import "testing"

func TestProductionAuthorityRejectsUnsafeConfiguration(t *testing.T) {
	base := Config{Mode: "server", HTTPAddr: "0.0.0.0:443", DNSAddr: "0.0.0.0:53", StateDir: t.TempDir(), Zone: "windows.operator.tld.", KeyFile: "protected.private", NameServer: "ns.dns.operator.tld.", PublicURL: "https://admin.operator.tld", TLSCert: "cert", TLSKey: "key"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Config){
		"lab nameserver":           func(c *Config) { c.NameServer = "ns.lab.invalid." },
		"origin path":              func(c *Config) { c.PublicURL = "https://admin.operator.tld/path" },
		"missing nameserver":       func(c *Config) { c.NameServer = "" },
		"nameserver in experiment": func(c *Config) { c.NameServer = "ns." + c.Zone },
		"noncanonical nameserver":  func(c *Config) { c.NameServer = "NS.OPERATOR.TLD." },
		"missing origin":           func(c *Config) { c.PublicURL = "" },
		"insecure origin":          func(c *Config) { c.PublicURL = "http://admin.operator.tld" },
		"origin in experiment":     func(c *Config) { c.PublicURL = "https://admin.windows.operator.tld" },
		"origin credentials":       func(c *Config) { c.PublicURL = "https://user:secret@admin.operator.tld" },
		"lab zone":                 func(c *Config) { c.Zone = "windows.lab.invalid." },
		"missing TLS":              func(c *Config) { c.TLSCert = ""; c.TLSKey = "" },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			change(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("unsafe production authority passes")
			}
		})
	}
}

func TestLabClassificationPreservesTheTLSAndLoopbackGuards(t *testing.T) {
	c := Config{Mode: "server", Environment: "lab", HTTPAddr: "0.0.0.0:8080", DNSAddr: "0.0.0.0:1053", StateDir: t.TempDir(), Zone: "windows.lab.invalid.", KeyFile: "synthetic.private", NameServer: "ns.lab.invalid.", PublicURL: "https://localhost:8080"}
	if err := c.Validate(); err == nil {
		t.Fatal("lab classification permits non-loopback plaintext")
	}
	c.TLSCert, c.TLSKey = "cert", "key"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.LocalLab = true
	if err := c.Validate(); err == nil {
		t.Fatal("lab classification weakens the loopback-only HTTP guard")
	}
	c.HTTPAddr = "127.0.0.1:8080"
	c.TLSCert = ""
	c.TLSKey = ""
	c.PublicURL = ""
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"", "production", "unknown"} {
		c.Environment = environment
		if err := c.Validate(); err == nil {
			t.Fatal("non-lab environment admits a lab zone")
		}
	}
}

package service

import (
	"encoding/json"
	"fmt"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/window"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
)

// Config contains the local process configuration.
type Config struct {
	Mode         string
	HTTPAddr     string
	DNSAddr      string
	StateDir     string
	AgentID      string
	LocalLab     bool
	TLSCert      string
	TLSKey       string
	Zone         string
	KeyFile      string
	WindowLimits *window.Limits
	NameServer   string
	PublicURL    string
	Environment  string
}

func (c Config) nameServer() string {
	if c.NameServer != "" {
		return c.NameServer
	}
	return "ns.lab.invalid."
}

// Validate rejects incomplete process configuration.
func (c Config) Validate() error {
	if c.Environment != "" && c.Environment != "lab" && c.Environment != "production" {
		return fmt.Errorf("environment must be lab or production")
	}
	if c.Mode != "server" && c.Mode != "dns" && c.Mode != "agent" {
		return fmt.Errorf("invalid service mode")
	}
	if c.HTTPAddr == "" || c.StateDir == "" {
		return fmt.Errorf("set the HTTP address and state directory")
	}
	if c.Mode == "dns" && c.DNSAddr == "" {
		return fmt.Errorf("set the DNS address")
	}
	if c.Mode == "agent" && c.AgentID == "" {
		return fmt.Errorf("set EPOCHALYPSE_AGENT_ID for agent mode")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("set both TLS certificate and key")
	}
	if err := c.validateWindows(); err != nil {
		return err
	}
	return c.validateLabHTTP()
}

func (c Config) validateLabHTTP() error {
	if c.LocalLab {
		host, _, err := net.SplitHostPort(c.HTTPAddr)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("local lab requires a loopback HTTP address")
		}
	}
	return nil
}

func (c Config) validateWindows() error {
	if err := c.windowLimits().Validate(); err != nil {
		return err
	}
	if (c.Zone == "") != (c.KeyFile == "") {
		return fmt.Errorf("set both the experimental zone and DNSSEC key file")
	}
	if c.Zone != "" && (c.Mode != "server" || c.DNSAddr == "") {
		return fmt.Errorf("signed windows require the main server and DNS address")
	}
	if c.Zone != "" {
		if err := authority.ValidateNameServer(c.Zone, c.nameServer()); err != nil {
			return err
		}
		if !c.labMaterialAllowed() && (c.NameServer == "" || labZone(c.Zone) || labZone(c.NameServer)) {
			return fmt.Errorf("production authority requires an explicit external nameserver and a non-lab zone")
		}
		if !c.LocalLab {
			return c.validateProductionAuthority()
		}
	}
	return nil
}

func (c Config) validateProductionAuthority() error {
	if c.TLSCert == "" {
		return fmt.Errorf("production authority requires an explicit external nameserver, TLS, and a non-lab zone")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return fmt.Errorf("production authority requires a public HTTPS origin without credentials")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("public URL must be an origin")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")) + "."
	if host == c.Zone || strings.HasSuffix(host, "."+c.Zone) {
		return fmt.Errorf("administration and release discovery must use a host outside the experimental zone")
	}
	return nil
}

func (c Config) labMaterialAllowed() bool { return c.Environment == "lab" }

func labZone(zone string) bool {
	for _, suffix := range []string{"example.", "invalid.", "test.", "localhost.", "example.com.", "example.net.", "example.org."} {
		if zone == suffix || strings.HasSuffix(zone, "."+suffix) {
			return true
		}
	}
	return false
}

func (c Config) windowLimits() window.Limits {
	if c.WindowLimits != nil {
		return *c.WindowLimits
	}
	return window.DefaultLimits()
}

// ReadConfig verifies the configuration format before a listener starts.
func ReadConfig(path string) error {
	_, err := ReadSettings(path)
	return err
}

// Settings contains non-secret versioned file configuration.
type Settings struct {
	Version    int    `json:"version"`
	Zone       string `json:"zone,omitempty"`
	NameServer string `json:"nameserver,omitempty"`
	PublicURL  string `json:"public_url,omitempty"`
}

// ReadSettings reads the bounded public configuration before service startup.
func ReadSettings(path string) (Settings, error) {
	// The local operator selects this file. No HTTP request controls its path.
	file, err := os.Open(path) // #nosec G304
	if err != nil {
		return Settings{}, fmt.Errorf("open local configuration: %w", err)
	}
	defer func() { _ = file.Close() }()
	var document Settings
	decoder := json.NewDecoder(io.LimitReader(file, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Settings{}, fmt.Errorf("decode local configuration: %w", err)
	}
	if document.Version != 1 {
		return Settings{}, fmt.Errorf("local configuration requires version 1")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Settings{}, fmt.Errorf("local configuration requires one JSON document")
	}
	return document, nil
}

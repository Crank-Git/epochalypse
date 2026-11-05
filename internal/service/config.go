package service

import (
	"encoding/json"
	"fmt"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/window"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
)

// Config contains the local process configuration.
type Config struct {
	Mode                  string
	HTTPAddr              string
	DNSAddr               string
	StateDir              string
	AgentID               string
	LocalLab              bool
	TLSCert               string
	TLSKey                string
	Zone                  string
	KeyFile               string
	CountdownZone         string
	CountdownKeyFile      string
	WindowLimits          *window.Limits
	NameServer            string
	PublicURL             string
	Environment           string
	Resolvers             []observation.Target
	CapsulePlaintextLimit int
}

func (c Config) nameServer() string {
	if c.NameServer != "" {
		return c.NameServer
	}
	return "ns.lab.invalid."
}

// Validate rejects incomplete process configuration.
func (c Config) Validate() error {
	switch c.Environment {
	case "", "lab", "production":
	default:
		return fmt.Errorf("environment must be lab or production")
	}
	switch c.Mode {
	case "server", "dns", "agent":
	default:
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
	if _, err := capsule.EncodedObjectLimit(c.capsuleLimit()); err != nil {
		return fmt.Errorf("configure a positive bounded capsule plaintext limit: %w", err)
	}
	if err := c.validateWindows(); err != nil {
		return err
	}
	if err := c.validateCountdowns(); err != nil {
		return err
	}
	if err := c.validateResolvers(); err != nil {
		return err
	}
	return c.validateLabHTTP()
}

func (c Config) capsuleLimit() int {
	if c.CapsulePlaintextLimit == 0 {
		return capsule.DefaultPlaintextLimit
	}
	return c.CapsulePlaintextLimit
}

func (c Config) validateResolvers() error {
	if len(c.Resolvers) == 0 {
		return nil
	}
	if c.Mode != "server" || c.Zone == "" {
		return fmt.Errorf("resolver jobs require the main signed-window authority")
	}
	if err := observation.ValidateTargets(c.Resolvers); err != nil {
		return err
	}
	for _, target := range c.Resolvers {
		if target.VersionSource != "configured" && target.VersionSource != "unknown" {
			return fmt.Errorf("resolver configuration requires configured or unknown version provenance")
		}
		if target.VersionSource == "unknown" && target.Version != "unknown" {
			return fmt.Errorf("unknown resolver version provenance requires an unknown version")
		}
		if target.Clock.Source != "configured" && target.Clock.Source != "unknown" {
			return fmt.Errorf("resolver configuration requires configured or unknown clock provenance")
		}
		if target.Clock.Source == "unknown" && (target.Clock.LowerNanoseconds != nil || target.Clock.UpperNanoseconds != nil) {
			return fmt.Errorf("unknown clock provenance cannot claim a measured interval")
		}
	}
	return nil
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
	Version               int                  `json:"version"`
	Zone                  string               `json:"zone,omitempty"`
	CountdownZone         string               `json:"countdown_zone,omitempty"`
	NameServer            string               `json:"nameserver,omitempty"`
	PublicURL             string               `json:"public_url,omitempty"`
	Resolvers             []observation.Target `json:"resolvers,omitempty"`
	CapsulePlaintextLimit int                  `json:"capsule_plaintext_limit,omitempty"`
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

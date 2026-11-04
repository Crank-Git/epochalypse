package service

import (
	"encoding/json"
	"fmt"
	"github.com/Crank-Git/epochalypse/internal/window"
	"io"
	"net"
	"os"
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
}

// Validate rejects incomplete process configuration.
func (c Config) Validate() error {
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
	return nil
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
	Version int    `json:"version"`
	Zone    string `json:"zone,omitempty"`
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

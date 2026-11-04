package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Config contains the local process configuration.
type Config struct {
	Mode     string
	HTTPAddr string
	DNSAddr  string
	StateDir string
	AgentID  string
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
	return nil
}

// ReadConfig verifies the configuration format before a listener starts.
func ReadConfig(path string) error {
	// The local operator selects this file. No HTTP request controls its path.
	file, err := os.Open(path) // #nosec G304
	if err != nil {
		return fmt.Errorf("open local configuration: %w", err)
	}
	defer func() { _ = file.Close() }()
	var document struct {
		Version int `json:"version"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode local configuration: %w", err)
	}
	if document.Version != 1 {
		return fmt.Errorf("local configuration requires version 1")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("local configuration requires one JSON document")
	}
	return nil
}

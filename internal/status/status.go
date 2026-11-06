// Package status verifies local deployment endpoints.
package status

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ValidCommit reports whether value is a full Git SHA-1 identifier.
func ValidCommit(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 40 && err == nil && value == strings.ToLower(value)
}

// Check returns the observed state and commit across all endpoints.
// An unavailable or inconsistent identity returns unknown.
func Check(endpoints []string, requested string) (string, string) {
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	state, commit := "success", ""
	for _, endpoint := range endpoints {
		observed, healthy := inspect(client, endpoint)
		if observed == "" || (commit != "" && commit != observed) {
			return "unknown", "unknown"
		}
		commit = observed
		if !healthy || commit != requested {
			state = "failure"
		}
	}
	if commit == "" {
		return "unknown", "unknown"
	}
	return state, commit
}

func inspect(client *http.Client, endpoint string) (string, bool) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	base := strings.TrimRight(endpoint, "/")
	body, err := fetch(client, base+"/version")
	var v struct {
		Commit string `json:"commit"`
	}
	if err != nil || json.Unmarshal(body, &v) != nil || !ValidCommit(v.Commit) {
		return "", false
	}
	health, err := fetch(client, base+"/healthz")
	return v.Commit, err == nil && string(health) == "ok\n"
}

func fetch(client *http.Client, endpoint string) ([]byte, error) {
	response, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("query deployment: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deployment HTTP status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil {
		return nil, fmt.Errorf("read deployment response: %w", err)
	}
	if len(body) > 4096 {
		return nil, fmt.Errorf("deployment response exceeds 4096 bytes")
	}
	return body, nil
}

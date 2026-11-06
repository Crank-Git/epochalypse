// Package deployment checks operator-supplied service inventories.
package deployment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Service pins one endpoint and records operator-declared failure domains.
type Service struct {
	ID            string            `json:"id"`
	Endpoint      string            `json:"endpoint"`
	TLSPin        string            `json:"tls_spki_sha256"`
	Domains       map[string]string `json:"failure_domains"`
	Evidence      string            `json:"domain_evidence"`
	FailureDomain string            `json:"failure_domain"`
}

// Inventory binds local deployment claims to the signed enrollment.
type Inventory struct {
	Version              int               `json:"version"`
	OwnerKey             ed25519.PublicKey `json:"owner_public_key"`
	Certificate          []byte            `json:"certificate"`
	CertificateSignature []byte            `json:"certificate_signature"`
	Main                 Service           `json:"main"`
	Agents               []Service         `json:"agents"`
}

// DomainIssue identifies shared or absent operator declarations.
type DomainIssue struct {
	Kind   string   `json:"kind"`
	Domain string   `json:"domain"`
	Agents []string `json:"agents"`
}

// ReadInventory decodes a bounded local configuration and checks enrollment pins.
func ReadInventory(r io.Reader) (Inventory, error) {
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil {
		return Inventory{}, errors.New("read deployment inventory")
	}
	if len(data) > 65536 {
		return Inventory{}, errors.New("deployment inventory exceeds limit")
	}
	var inventory Inventory
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&inventory) != nil || decoder.Decode(new(any)) != io.EOF {
		return Inventory{}, errors.New("invalid deployment inventory")
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return Inventory{}, err
	}
	if _, err := inventory.Validate(); err != nil {
		return Inventory{}, err
	}
	return inventory, nil
}

// Validate checks the main service and the exact canonical enrolled agents.
func (i Inventory) Validate() (switchwire.AdmittedEnrollment, error) {
	if i.Version != 1 || len(i.Agents) != 3 || i.Main.ID != "main" {
		return switchwire.AdmittedEnrollment{}, errors.New("inventory requires main and exactly three agents")
	}
	enrollment, err := switchwire.AdmitCertificate(i.Certificate, i.CertificateSignature, i.OwnerKey)
	if err != nil {
		return switchwire.AdmittedEnrollment{}, errors.New("inventory enrollment is invalid")
	}
	if !validService(i.Main) {
		return switchwire.AdmittedEnrollment{}, errors.New("invalid main endpoint or TLS pin")
	}
	endpoints := map[string]bool{i.Main.Endpoint: true}
	for index, a := range i.Agents {
		canonical := enrollment.Agents[index]
		if a.ID != canonical.ID || a.Endpoint != canonical.Endpoint || endpoints[a.Endpoint] || !validService(a) {
			return switchwire.AdmittedEnrollment{}, errors.New("inventory agent does not match canonical enrollment or TLS pin")
		}
		endpoints[a.Endpoint] = true
	}
	return enrollment, nil
}

func validService(s Service) bool {
	u, err := url.Parse(s.Endpoint)
	pin, pinErr := hex.DecodeString(s.TLSPin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Opaque != "" {
		return false
	}
	if pinErr != nil || len(pin) != 32 || s.TLSPin == strings.Repeat("0", 64) {
		return false
	}
	return true
}

// DomainIssues compares declarations without certifying physical independence.
func (i Inventory) DomainIssues() []DomainIssue {
	var issues []DomainIssue
	for _, domain := range []string{"failure_domain", "provider", "region", "host", "network", "power"} {
		values := map[string][]string{}
		var order []string
		for _, a := range i.Agents {
			value := strings.TrimSpace(a.Domains[domain])
			if domain == "failure_domain" {
				value = strings.TrimSpace(a.FailureDomain)
			}
			if domain != "failure_domain" && value == "" {
				continue
			}
			if value == "" || strings.EqualFold(value, "unknown") || strings.TrimSpace(a.Evidence) == "" {
				issues = append(issues, DomainIssue{Kind: "unknown", Domain: domain, Agents: []string{a.ID}})
				continue
			}
			value = strings.ToLower(value)
			if _, exists := values[value]; !exists {
				order = append(order, value)
			}
			values[value] = append(values[value], a.ID)
		}
		for _, value := range order {
			if len(values[value]) > 1 {
				issues = append(issues, DomainIssue{Kind: "shared", Domain: domain, Agents: values[value]})
			}
		}
	}
	return issues
}

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("inventory exceeds nesting limit")
	}
	token, err := d.Token()
	if err != nil {
		return errors.New("invalid inventory JSON")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for d.More() {
		if delimiter == '{' {
			key, err := d.Token()
			if err != nil {
				return errors.New("invalid inventory key")
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return errors.New("duplicate inventory key")
			}
			keys[name] = true
		}
		if err := uniqueJSON(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

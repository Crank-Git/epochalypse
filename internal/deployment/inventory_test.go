package deployment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func inventoryFixture(t *testing.T) Inventory {
	t.Helper()
	owner, key, _ := ed25519.GenerateKey(nil)
	certificate := switchwire.Certificate{Version: 2, Interval: 3600, Grace: 60, Key: [32]byte{1}, Content: [32]byte{2}}
	copy(certificate.OwnerKey[:], owner)
	inventory := Inventory{Version: 1, OwnerKey: owner}
	for index := range 3 {
		public, _, _ := ed25519.GenerateKey(nil)
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			t.Fatal(err)
		}
		a := switchwire.Agent{ID: fmt.Sprintf("agent-%d", index), Recipient: identity.Recipient().String(), Endpoint: fmt.Sprintf("https://agent-%d.example", index)}
		copy(a.SigningKey[:], public)
		certificate.Agents[index] = a
		inventory.Agents = append(inventory.Agents, Service{ID: a.ID, Endpoint: a.Endpoint, TLSPin: strings.Repeat("01", 32), Evidence: "operator host record", Domains: map[string]string{"provider": a.ID, "region": a.ID, "host": a.ID, "network": a.ID, "power": a.ID}})
		inventory.Agents[index].FailureDomain = a.ID
	}
	inventory.Main = Service{ID: "main", Endpoint: "https://main.example", TLSPin: strings.Repeat("02", 32)}
	body, err := switchwire.EncodeCertificate(certificate)
	if err != nil {
		t.Fatal(err)
	}
	inventory.Certificate = body
	inventory.CertificateSignature = ed25519.Sign(key, body)
	return inventory
}

func resignEndpoints(t *testing.T, inventory Inventory, services []Service) Inventory {
	t.Helper()
	enrollment, err := switchwire.AdmitCertificate(inventory.Certificate, inventory.CertificateSignature, inventory.OwnerKey)
	if err != nil {
		t.Fatal(err)
	}
	owner, key, _ := ed25519.GenerateKey(nil)
	certificate := switchwire.Certificate{Version: 2, Interval: 3600, Grace: 60, Key: [32]byte{1}, Content: [32]byte{2}, Agents: enrollment.Agents}
	copy(certificate.OwnerKey[:], owner)
	for index := range services {
		certificate.Agents[index].Endpoint = services[index].Endpoint
	}
	inventory.Certificate, err = switchwire.EncodeCertificate(certificate)
	if err != nil {
		t.Fatal(err)
	}
	inventory.CertificateSignature = ed25519.Sign(key, inventory.Certificate)
	inventory.OwnerKey = owner
	inventory.Agents = services
	return inventory
}

func TestInventoryRequiresExactEnrolledIdentitiesAndEndpoints(t *testing.T) {
	inventory := inventoryFixture(t)
	data, _ := json.Marshal(inventory)
	if _, err := ReadInventory(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Inventory){
		"missing agent":        func(i *Inventory) { i.Agents = i.Agents[:2] },
		"duplicate agent":      func(i *Inventory) { i.Agents[1] = i.Agents[0] },
		"endpoint mismatch":    func(i *Inventory) { i.Agents[0].Endpoint = "https://other.example" },
		"main shares endpoint": func(i *Inventory) { i.Main.Endpoint = i.Agents[0].Endpoint },
		"unknown version":      func(i *Inventory) { i.Version = 2 },
		"absent TLS pin":       func(i *Inventory) { i.Main.TLSPin = "" },
		"invalid enrollment":   func(i *Inventory) { i.CertificateSignature[0] ^= 1 },
	} {
		t.Run(name, func(t *testing.T) {
			value := inventoryFixture(t)
			change(&value)
			if _, err := value.Validate(); err == nil {
				t.Fatal("invalid inventory passes")
			}
		})
	}
	for _, data := range []string{"{}", string(data) + " {}", strings.Replace(string(data), `"version":1`, `"version":1,"extra":1`, 1), strings.Repeat(" ", 65537)} {
		if _, err := ReadInventory(strings.NewReader(data)); err == nil {
			t.Fatal("invalid document passes")
		}
	}
}

func TestDomainReportSeparatesSharedAndUnknownDeclarations(t *testing.T) {
	inventory := inventoryFixture(t)
	if issues := inventory.DomainIssues(); len(issues) != 0 {
		t.Fatalf("distinct: %+v", issues)
	}
	inventory.Agents[1].Domains["host"] = inventory.Agents[0].Domains["host"]
	delete(inventory.Agents[2].Domains, "power")
	inventory.Agents[2].Evidence = ""
	issues := inventory.DomainIssues()
	shared, unknown := false, false
	for _, issue := range issues {
		shared = shared || issue.Kind == "shared"
		unknown = unknown || issue.Kind == "unknown"
	}
	if !shared || !unknown {
		t.Fatalf("domain issues: %+v", issues)
	}
}

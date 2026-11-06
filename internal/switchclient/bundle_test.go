package switchclient

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"testing"
)

func TestBundleRejectsChangedContentOwnerAndReceipts(t *testing.T) {
	p, key, r := armFixture(t)
	if _, err := PrepareAndArm(context.Background(), p, key, r, r.bundle); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(r.bundle)
	if err != nil {
		t.Fatal(err)
	}
	owner := key.Public().(ed25519.PublicKey)
	b, err := ReadBundle(bytes.NewReader(data), owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"content", "owner", "receipts", "version", "trailing"} {
		t.Run(name, func(t *testing.T) {
			var changed Bundle
			if err := json.Unmarshal(data, &changed); err != nil {
				t.Fatal(err)
			}
			pinned := owner
			switch name {
			case "content":
				changed.Plan.Ciphertext[0] ^= 1
			case "owner":
				pinned = make(ed25519.PublicKey, 32)
			case "receipts":
				changed.Prepared = changed.Prepared[:2]
			case "version":
				changed.Version = 2
			}
			encoded, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if name == "trailing" {
				encoded = append(encoded, []byte("{}")...)
			}
			if _, err := ReadBundle(bytes.NewReader(encoded), pinned); err == nil {
				t.Fatal("changed bundle accepted")
			}
		})
	}
	if err := ExportBundle(r.bundle, b, owner); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	b.Plan.Ciphertext[0] ^= 1
	if err := ExportBundle(r.bundle, b, owner); err == nil {
		t.Fatal("different export accepted")
	}
}

func TestArmRejectsWrongOwnerAndChangedContentBeforePrepare(t *testing.T) {
	for _, name := range []string{"owner", "content", "request"} {
		t.Run(name, func(t *testing.T) {
			p, key, r := armFixture(t)
			switch name {
			case "owner":
				_, key, _ = ed25519.GenerateKey(nil)
			case "content":
				p.Ciphertext[0] ^= 1
			case "request":
				p.Prepare.Signature[0] ^= 1
			}
			result, err := PrepareAndArm(context.Background(), p, key, r, r.bundle)
			if err == nil || result.Confirmed || len(result.Prepared) != 0 || r.armCalls != 0 {
				t.Fatalf("invalid plan accepted: %+v %v", result, err)
			}
		})
	}
}

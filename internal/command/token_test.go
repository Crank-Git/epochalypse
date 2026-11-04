package command

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/owner"
)

func TestTokenCommandCreatesAScopedCredential(t *testing.T) {
	dir := t.TempDir()
	s, err := owner.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize([]byte("synthetic")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var out, diagnostic bytes.Buffer
	args := []string{"token", "create", "--state-dir", dir, "--name", "automation", "--scopes", "read,switch:checkin", "--expires-at", "2099-01-01T00:00:00Z"}
	if code := Run(args, &out, &diagnostic); code != 0 {
		t.Fatalf("create exits %d: %s", code, &diagnostic)
	}
	var grant owner.TokenGrant
	if err := json.Unmarshal(out.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	s, err = owner.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.AuthenticateToken(grant.Token, "switch:checkin", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateToken(grant.Token, "switch:write", time.Now()); err == nil {
		t.Fatal("check-in token permits writes")
	}
	if diagnostic.Len() != 0 {
		t.Fatal("success writes diagnostics")
	}
}

func TestTokenCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"token"}, {"token", "unknown"}, {"token", "create", "--unknown"}, {"token", "create"}, {"token", "create", "extra"}, {"token", "create", "--expires-at", "bad"}} {
		if code := Run(args, io.Discard, io.Discard); code != 2 {
			t.Fatalf("%v exits %d", args, code)
		}
	}
}

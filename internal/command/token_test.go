package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	server := httptest.NewServer(s.RequireScope("switch:checkin", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })))
	defer server.Close()
	client := &http.Client{Timeout: time.Second}
	for _, status := range []int{204, 401} {
		request, err := http.NewRequest(http.MethodPost, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+grant.Token)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != status {
			t.Fatalf("scoped HTTP request: %d", response.StatusCode)
		}
		if err := s.RevokeToken(grant.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTokenCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"token"}, {"token", "unknown"}, {"token", "create", "--unknown"}, {"token", "create"}, {"token", "create", "extra"}, {"token", "create", "--expires-at", "bad"}} {
		if code := Run(args, io.Discard, io.Discard); code != 2 {
			t.Fatalf("%v exits %d", args, code)
		}
	}
}

func TestTokenCommandReportsStateAndOutputFailures(t *testing.T) {
	args := []string{"token", "create", "--state-dir", t.TempDir(), "--name", "automation", "--scopes", "read", "--expires-at", "2099-01-01T00:00:00Z"}
	if code := Run(args, io.Discard, io.Discard); code != 1 {
		t.Fatal("missing owner permits token creation")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	args[3] = file
	if code := Run(args, io.Discard, io.Discard); code != 1 {
		t.Fatal("invalid state permits token creation")
	}
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
	args[3] = dir
	if code := Run(args, failedWriter{}, io.Discard); code != 1 {
		t.Fatal("failed output reports success")
	}
}

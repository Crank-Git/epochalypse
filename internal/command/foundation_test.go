package command

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoCreatesSyntheticFixtures(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	if code := Run([]string{"demo", "--state-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d: %s", code, &errOut)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile("demo/fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"synthetic", "capsule", "switch", "countdown", "window"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s: %s", want, data)
		}
	}
	if code := Run([]string{"clean-demo", "--state-dir", dir}, &out, &errOut); code == 0 {
		t.Fatal("cleanup accepts missing namespace")
	}
	if code := Run([]string{"clean-demo", "--state-dir", dir, "--namespace", "../demo"}, &out, &errOut); code == 0 {
		t.Fatal("cleanup accepts traversal")
	}
	if code := Run([]string{"clean-demo", "--state-dir", dir, "--namespace", "demo"}, &out, &errOut); code != 0 {
		t.Fatal(errOut.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "demo")); !os.IsNotExist(err) {
		t.Fatalf("demo remains: %v", err)
	}
}

func TestStatusVerifiesObservedCommit(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct{ name, version, health, want string }{
		{"correct", `{"commit":"` + sha + `"}`, "ok\n", "success local " + sha + "\n"},
		{"wrong", `{"commit":"` + strings.Repeat("b", 40) + `"}`, "ok\n", "failure local " + strings.Repeat("b", 40) + "\n"},
		{"malformed", `{`, "ok\n", "unknown local unknown\n"},
		{"unhealthy", `{"commit":"` + sha + `"}`, "bad", "failure local " + sha + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/version" {
					_, _ = w.Write([]byte(tc.version))
				} else {
					_, _ = w.Write([]byte(tc.health))
				}
			}))
			defer server.Close()
			var out, errOut bytes.Buffer
			code := Run([]string{"status", "--endpoints", server.URL, sha}, &out, &errOut)
			if code != 0 || out.String() != tc.want {
				t.Fatalf("code=%d out=%q err=%s", code, &out, &errOut)
			}
			if !strings.Contains(errOut.String(), "legacy endpoint status") {
				t.Fatalf("legacy scope is absent: %s", &errOut)
			}
		})
	}
}

func TestFoundationRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"demo", "--bad"}, {"demo", "extra"}, {"clean-demo", "--namespace", "other"}, {"status"}, {"status", "short"}, {"status", "--bad"}, {"demo", "--state-dir", ""}} {
		if code := Run(args, &bytes.Buffer{}, &bytes.Buffer{}); code == 0 {
			t.Fatalf("accepts %v", args)
		}
	}
	for _, args := range [][]string{{"demo", "--state-dir", t.TempDir()}, {"status", strings.Repeat("a", 40)}} {
		if code := Run(args, failedWriter{}, &bytes.Buffer{}); code != 1 {
			t.Fatalf("output failure: %v = %d", args, code)
		}
	}
}

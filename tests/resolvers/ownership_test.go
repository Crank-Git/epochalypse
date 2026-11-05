//go:build resolvers

package resolvers_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolverFixtureAccessProcess(t *testing.T) {
	if os.Getenv("EPOCHALYPSE_FIXTURE_DENY") == "1" {
		for _, path := range []string{"/tmp/resolver-owner/synthetic.private", "/tmp/resolver-owner.private"} {
			if _, err := os.ReadFile(path); !errors.Is(err, os.ErrPermission) { // #nosec G304 -- The test supplies both fixed private paths.
				t.Fatalf("another user reads the private fixture: %v", err)
			}
		}
		return
	}
	expected := os.Getenv("EPOCHALYPSE_FIXTURE_USER")
	if expected == "" {
		return
	}
	if actual := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()); actual != expected {
		t.Fatalf("fixture process user is %s; want %s", actual, expected)
	}
	portPolicy, err := os.ReadFile("/proc/sys/net/ipv4/ip_unprivileged_port_start")
	if err != nil {
		t.Fatal(err)
	}
	if string(portPolicy) != "0\n" {
		t.Fatalf("fixture namespace requires unprivileged port 53: %q", portPolicy)
	}
	t.Logf("fixture user=%s ip_unprivileged_port_start=0", expected)
	private, err := os.ReadFile("/fixture/synthetic.private")
	if err != nil {
		t.Fatal(err)
	}
	// Container-local files exercise Linux access rules without Desktop mount translation.
	if err := os.Mkdir("/tmp/resolver-owner", 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/tmp/resolver-owner/synthetic.private", "/tmp/resolver-owner.private"} {
		if err := os.WriteFile(path, private, 0600); err != nil { // #nosec G703 -- The test supplies both fixed private destinations.
			t.Fatal(err)
		}
	}
	// Another user cannot traverse the protected fixture mount on native Linux.
	binary, err := os.ReadFile("/fixture/authority.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/resolver-access.test", binary, 0755); err != nil { // #nosec G303 G306 G703 -- The isolated container exposes this public test executable at a fixed path.
		t.Fatal(err)
	}
}

func TestResolverFixtureOwnership(t *testing.T) {
	lab := newResolverLab(t)
	container := lab.network + "-authority"
	policy := docker(t, "inspect", "--format", "{{json .HostConfig.CapDrop}} {{json .HostConfig.SecurityOpt}}", container)
	if policy != `["ALL"] ["no-new-privileges"]` {
		t.Fatalf("fixture isolation changes: %s", policy)
	}
	user := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	t.Log(docker(t, "exec", "--env", "EPOCHALYPSE_FIXTURE_USER="+user, container,
		"/fixture/authority.test", "-test.v", "-test.run=^TestResolverFixtureAccessProcess$"))
	docker(t, "exec", "--user", fmt.Sprintf("%d:%d", os.Getuid()+1, os.Getgid()),
		"--env", "EPOCHALYPSE_FIXTURE_DENY=1", container,
		"/tmp/resolver-access.test", "-test.run=^TestResolverFixtureAccessProcess$")
	for path, mode := range map[string]os.FileMode{
		"synthetic.private": 0600, "synthetic.key": 0600, "state/v1/windows.db": 0600,
		".": 0700, "state": 0700, "state/v1": 0700,
	} {
		info, err := os.Stat(filepath.Join(lab.directory, path))
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("fixture %s loses owner-only access: %v", path, err)
		}
	}
}

//go:build resolvers

package resolvers_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/observation"
	"github.com/Crank-Git/epochalypse/internal/window"
)

const unboundImage = "mvance/unbound@sha256:76906da36d1806f3387338f15dcf8b357c51ce6897fb6450d6ce010460927e90"
const bindImage = "ubuntu/bind9@sha256:8f62529754757e8cb7bbf27f43a1d280f9c5f92c2d30704565242058d8d24872"

func TestResolverAuthorityProcess(t *testing.T) {
	if os.Getenv("EPOCHALYPSE_RESOLVER_CLOCK") == "1" {
		fmt.Printf("CLOCK=%d\n", time.Now().UnixNano())
		return
	}
	if os.Getenv("EPOCHALYPSE_RESOLVER_FIXTURE") != "1" {
		return
	}
	signer, err := window.LoadSigner("windows.example.", "/fixture/synthetic.private")
	if err != nil {
		t.Fatal(err)
	}
	store, err := window.Open("/fixture/state", signer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	a, err := authority.New(store, signer, "ns.lab.invalid.")
	if err != nil {
		t.Fatal(err)
	}
	handler := dns.HandlerFunc(func(_ context.Context, writer dns.ResponseWriter, request *dns.Msg) {
		udp := writer.RemoteAddr().Network() == "udp"
		reply := &dns.Msg{Data: a.Packet(request.Data, udp, time.Now())}
		if _, err := io.Copy(writer, reply); err != nil {
			t.Error(err)
		}
	})
	go func() {
		if err := dns.ListenAndServe(":53", "tcp", handler); err != nil {
			t.Error(err)
		}
	}()
	if err := dns.ListenAndServe(":53", "udp", handler); err != nil {
		t.Fatal(err)
	}
}

func TestStrictResolverObservesProductionWindows(t *testing.T) {
	directory := t.TempDir()
	key, records := createFixture(t, directory)
	binary := filepath.Join(directory, "authority.test")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// #nosec G204 -- The test controls each compiler argument.
	build := exec.CommandContext(ctx, "go", "test", "-tags=resolvers", "-c", "-o", binary, ".")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build authority: %v: %s", err, output)
	}
	network := fmt.Sprintf("epochalypse-worker9-%d", time.Now().UnixNano())
	docker(t, "network", "create", network)
	t.Cleanup(func() { docker(t, "network", "rm", network) })
	authorityContainer := startContainer(t, network, directory, "authority", "linux/arm64", bindImage, "/fixture/authority.test", "-test.run=^TestResolverAuthorityProcess$", "-test.timeout=3m")
	ip := docker(t, "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", authorityContainer)
	clockLower, clockUpper := sampleClock(t, authorityContainer)
	config := fmt.Sprintf(`server:
 interface: 0.0.0.0
 username: ""
 chroot: ""
 directory: "/tmp"
 logfile: ""
 use-syslog: no
 pidfile: "/tmp/unbound.pid"
 access-control: 0.0.0.0/0 allow
 do-ip6: no
 local-zone: "example." nodefault
 val-sig-skew-min: 0
 val-sig-skew-max: 0
 trust-anchor: "%s"
stub-zone:
 name: "windows.example."
 stub-addr: %s
 stub-first: no
`, key.String(), strings.TrimSpace(ip))
	if err := os.WriteFile(filepath.Join(directory, "strict.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	resolver := startContainer(t, network, directory, "strict", "linux/amd64", unboundImage, "/opt/unbound/sbin/unbound", "-d", "-c", "/fixture/strict.conf")
	version := docker(t, "exec", resolver, "/opt/unbound/sbin/unbound", "-V")
	if !strings.Contains(version, "Version 1.22.0") {
		t.Fatalf("unexpected resolver identity: %s", version)
	}
	address := strings.TrimSpace(docker(t, "port", resolver, "53/udp"))
	offset := int64(0)
	target := observation.Target{ID: "unbound-strict", Address: address, Version: "1.22.0", VersionSource: "executable -V", Clock: observation.Clock{OffsetNanoseconds: &offset, Source: "configured zero; Docker clock sampled from authority", LowerNanoseconds: &clockLower, UpperNanoseconds: &clockUpper}}
	current := awaitResponse(t, records["current"].Name, target)
	if current.Rcode == nil || *current.Rcode != dns.RcodeSuccess || current.Flags == nil || !current.Flags.AD || !current.Flags.RA {
		t.Fatalf("the strict resolver cannot validate the positive control: %+v", current)
	}
	future := observation.Observe(context.Background(), records["future"].Name, target)
	if future.Rcode == nil || *future.Rcode != dns.RcodeServerFailure || future.Flags == nil || future.Flags.AD {
		t.Fatalf("the strict resolver accepts a future signature: %+v", future)
	}
	if future.Resolver.Version != "1.22.0" || future.Resolver.Clock.OffsetNanoseconds == nil || future.Completed.Before(future.Started) {
		t.Fatalf("the observation loses resolver provenance: %+v", future)
	}
	t.Logf("strict production path: current rcode=%d AD=%v; future rcode=%d AD=%v; version=%s; offset interval=[%d,%d]ns", *current.Rcode, current.Flags.AD, *future.Rcode, future.Flags.AD, target.Version, clockLower, clockUpper)
}

func createFixture(t *testing.T, directory string) (*dns.DNSKEY, map[string]window.Window) {
	t.Helper()
	key := dns.NewDNSKEY("windows.example.", dns.ECDSAP256SHA256)
	key.Flags = 257
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "synthetic.private")
	for path, text := range map[string]string{path: key.PrivateKeyString(private), filepath.Join(directory, "synthetic.key"): key.String()} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	signer, err := window.LoadSigner("windows.example.", path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := window.Open(filepath.Join(directory, "state"), signer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	now := time.Now().UTC().Truncate(time.Second)
	records := map[string]window.Window{}
	for label, start := range map[string]time.Time{"current": now.Add(-time.Hour), "future": now.Add(time.Hour)} {
		in := window.Input{Content: "public resolver experiment", Inception: start.Format(time.RFC3339), Expiration: start.Add(2 * time.Hour).Format(time.RFC3339)}
		record, err := store.Create(in, now)
		if err != nil {
			t.Fatal(err)
		}
		records[label] = record
	}
	return key, records
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// #nosec G204 -- The test supplies Docker arguments and isolated resource names.
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func startContainer(t *testing.T, network, directory, name, platform, image, entry string, args ...string) string {
	t.Helper()
	name = network + "-" + name
	command := []string{"run", "--detach", "--name", name, "--network", network, "--platform", platform, "--env", "EPOCHALYPSE_RESOLVER_FIXTURE=1", "--volume", directory + ":/fixture", "--publish", "127.0.0.1::53/udp", "--publish", "127.0.0.1::53/tcp", "--entrypoint", entry, image}
	docker(t, append(command, args...)...)
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(docker(t, "logs", "--tail", "35", name))
		}
		docker(t, "rm", "--force", name)
	})
	return name
}

func awaitResponse(t *testing.T, name string, target observation.Target) observation.Result {
	t.Helper()
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		result := observation.Observe(context.Background(), name, target)
		if result.Rcode != nil && *result.Rcode == dns.RcodeSuccess {
			return result
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the resolver does not answer its positive control")
	return observation.Result{}
}

func sampleClock(t *testing.T, container string) (int64, int64) {
	t.Helper()
	before := time.Now().UnixNano()
	output := docker(t, "exec", "--env", "EPOCHALYPSE_RESOLVER_CLOCK=1", container, "/fixture/authority.test", "-test.run=^TestResolverAuthorityProcess$")
	after := time.Now().UnixNano()
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "CLOCK=") {
			continue
		}
		clock, err := strconv.ParseInt(strings.TrimPrefix(line, "CLOCK="), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		lower, upper := clock-after, clock-before
		if lower > int64(time.Second) || upper < -int64(time.Second) {
			t.Fatalf("Docker clock interval [%d,%d] exceeds one second", lower, upper)
		}
		return lower, upper
	}
	t.Fatal("the fixture omits its clock sample")
	return 0, 0
}

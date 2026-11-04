//go:build resolvers

package resolvers_test

import (
	"context"
	"encoding/json"
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
	"github.com/Crank-Git/epochalypse/internal/resolver"
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

type resolverLab struct {
	directory   string
	network     string
	authorityIP string
	key         *dns.DNSKEY
	records     map[string]window.Window
	clock       observation.Clock
}

func newResolverLab(t *testing.T) resolverLab {
	t.Helper()
	directory := t.TempDir()
	binary := filepath.Join(directory, "authority.test")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// #nosec G204 -- The test controls each compiler argument.
	build := exec.CommandContext(ctx, "go", "test", "-tags=resolvers", "-c", "-o", binary, ".")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build authority: %v: %s", err, output)
	}
	key, records := createFixture(t, directory)
	network := fmt.Sprintf("epochalypse-worker9-%d", time.Now().UnixNano())
	docker(t, "network", "create", network)
	t.Cleanup(func() { docker(t, "network", "rm", network) })
	container := startContainer(t, network, directory, "authority", "linux/arm64", bindImage, "/fixture/authority.test", "-test.run=^TestResolverAuthorityProcess$", "-test.timeout=3m")
	ip := docker(t, "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", container)
	lower, upper := sampleClock(t, container)
	zero := int64(0)
	return resolverLab{directory, network, ip, key, records, observation.Clock{OffsetNanoseconds: &zero, Source: "configured zero; shared Docker clock sample", LowerNanoseconds: &lower, UpperNanoseconds: &upper}}
}

func (lab resolverLab) startResolver(t *testing.T, kind, label string) observation.Target {
	t.Helper()
	config, entry, platform, image := resolverConfiguration(kind, lab.authorityIP, lab.key)
	file := label + ".conf"
	if err := os.WriteFile(filepath.Join(lab.directory, file), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-d", "-c", "/fixture/" + file}
	version := "1.22.0"
	marker := "Version " + version
	if kind == "bind" {
		args = []string{"-g", "-c", "/fixture/" + file, "-n", "1"}
		version = "9.20.24-1ubuntu0.4-Ubuntu"
		marker = version
	}
	container := startContainer(t, lab.network, lab.directory, label, platform, image, entry, args...)
	identity := docker(t, "exec", container, entry, "-V")
	if !strings.Contains(identity, marker) {
		t.Fatalf("unexpected %s identity: %s", kind, identity)
	}
	address := docker(t, "port", container, "53/udp")
	target := observation.Target{ID: kind, Address: address, Version: version, VersionSource: "executable -V", Clock: lab.clock}
	assertResult(t, awaitResponse(t, lab.records["current"].Name, target), dns.RcodeSuccess, true)
	return target
}

func TestResolverMatrixObservesProductionWindows(t *testing.T) {
	lab := newResolverLab(t)
	warm, cold := map[string]observation.Target{}, map[string]observation.Target{}
	for _, kind := range []string{"unbound-strict", "unbound-default", "bind"} {
		warm[kind] = lab.startResolver(t, kind, kind+"-warm")
		cold[kind] = lab.startResolver(t, kind, kind+"-cold")
	}
	results := []observation.Result{}
	for _, kind := range []string{"unbound-strict", "unbound-default", "bind"} {
		target := warm[kind]
		for _, name := range []string{"current", "near", "future", "expired"} {
			accepted := name == "current" || (name == "near" && kind == "unbound-default")
			code := uint16(dns.RcodeServerFailure)
			if accepted {
				code = dns.RcodeSuccess
			}
			result := resolver.Observe(context.Background(), lab.records[name].Name, target)
			assertResult(t, result, code, accepted)
			results = append(results, result)
		}
	}
	results = append(results, assertCacheBoundary(t, lab, warm, cold)...)
	if err := os.MkdirAll("../../.test-artifacts", 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(struct {
		Windows map[string]window.Window `json:"windows"`
		Results []observation.Result     `json:"results"`
	}{lab.records, results}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("../../.test-artifacts/resolvers.json", data, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertCacheBoundary(t *testing.T, lab resolverLab, warm, cold map[string]observation.Target) []observation.Result {
	t.Helper()
	results := []observation.Result{}
	boundary := lab.records["boundary"]
	if !time.Now().Before(boundary.Inception.Add(-2 * time.Second)) {
		t.Fatal("the fixture does not leave time to prime failure caches")
	}
	for _, kind := range []string{"unbound-strict", "unbound-default", "bind"} {
		code := uint16(dns.RcodeServerFailure)
		if kind == "unbound-default" {
			code = dns.RcodeSuccess
		}
		result := resolver.Observe(context.Background(), boundary.Name, warm[kind])
		assertResult(t, result, code, kind == "unbound-default")
		results = append(results, result)
	}
	// The clock interval and scheduling margin prevent an exact-boundary claim.
	time.Sleep(time.Until(boundary.Inception.Add(2 * time.Second)))
	for _, kind := range []string{"unbound-strict", "unbound-default", "bind"} {
		coldResult := resolver.Observe(context.Background(), boundary.Name, cold[kind])
		assertResult(t, coldResult, dns.RcodeSuccess, true)
		warmResult := resolver.Observe(context.Background(), boundary.Name, warm[kind])
		code := uint16(dns.RcodeSuccess)
		if kind == "unbound-strict" {
			code = dns.RcodeServerFailure
		}
		assertResult(t, warmResult, code, kind != "unbound-strict")
		t.Logf("%s boundary=%s cold=%s rcode=%d warm=%s rcode=%d", kind, boundary.Inception.Format(time.RFC3339), coldResult.Completed.Format(time.RFC3339Nano), *coldResult.Rcode, warmResult.Completed.Format(time.RFC3339Nano), *warmResult.Rcode)
		results = append(results, coldResult, warmResult)
	}
	// Strict Unbound retains a bogus entry for its default sixty-second interval.
	until := time.Now().Add(45 * time.Second)
	for {
		result := resolver.Observe(context.Background(), boundary.Name, warm["unbound-strict"])
		if result.Rcode != nil && *result.Rcode == dns.RcodeSuccess {
			assertResult(t, result, dns.RcodeSuccess, true)
			t.Logf("strict warm recovery at %s", result.Completed.Format(time.RFC3339Nano))
			results = append(results, result)
			break
		}
		if time.Now().After(until) {
			t.Fatal("the warm strict resolver does not recover after its failure cache expires")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return results
}

func assertResult(t *testing.T, result observation.Result, rcode uint16, ad bool) {
	t.Helper()
	if result.Error != "" || result.Rcode == nil || result.Flags == nil {
		t.Fatalf("the resolver omits its response: %+v", result)
	}
	if *result.Rcode != rcode || result.Flags.AD != ad || !result.Flags.RA {
		t.Fatalf("%s rcode=%d AD=%v RA=%v; want rcode=%d AD=%v", result.Resolver.ID, *result.Rcode, result.Flags.AD, result.Flags.RA, rcode, ad)
	}
	if result.Version != 1 || result.Resolver.Version == "" || result.Resolver.Clock.OffsetNanoseconds == nil || result.Completed.Before(result.Started) {
		t.Fatalf("the result loses its provenance: %+v", result)
	}
}

func resolverConfiguration(kind, ip string, key *dns.DNSKEY) (string, string, string, string) {
	if kind == "bind" {
		config := fmt.Sprintf(`options {
 directory "/tmp";
 listen-on { any; }; listen-on-v6 { none; };
 recursion yes; allow-query { any; }; dnssec-validation yes;
 pid-file "/tmp/named.pid"; session-keyfile "/tmp/session.key";
};
trust-anchors { "windows.example." static-key %d %d %d "%s"; };
zone "windows.example." { type static-stub; server-addresses { %s; }; };
`, key.Flags, key.Protocol, key.Algorithm, key.PublicKey, ip)
		return config, "/usr/sbin/named", "linux/arm64", bindImage
	}
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
 trust-anchor: "%s"
stub-zone:
 name: "windows.example."
 stub-addr: %s
 stub-first: no
`, key.String(), ip)
	if kind == "unbound-strict" {
		config = strings.Replace(config, "server:\n", "server:\n val-sig-skew-min: 0\n val-sig-skew-max: 0\n", 1)
	}
	return config, "/opt/unbound/sbin/unbound", "linux/amd64", unboundImage
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
	for label, start := range map[string]time.Time{"current": now.Add(-time.Hour), "near": now.Add(20 * time.Minute), "future": now.Add(48 * time.Hour), "expired": now.Add(-48 * time.Hour), "boundary": now.Add(30 * time.Second)} {
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
		result := resolver.Observe(context.Background(), name, target)
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

package compatibility_test

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

const unboundImage = "mvance/unbound@sha256:76906da36d1806f3387338f15dcf8b357c51ce6897fb6450d6ce010460927e90"
const bindImage = "ubuntu/bind9@sha256:8f62529754757e8cb7bbf27f43a1d280f9c5f92c2d30704565242058d8d24872"

func TestAuthorityProcess(t *testing.T) {
	if os.Getenv("COMPAT_CLOCK") == "1" {
		fmt.Printf("CLOCK=%d\n", time.Now().UnixNano())
		return
	}
	if os.Getenv("COMPAT_ZONE") != "/fixture/zone.json" {
		return
	}
	data, err := os.ReadFile("/fixture/zone.json")
	if err != nil {
		t.Fatal(err)
	}
	var records []string
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	zone := make(map[string][]dns.RR)
	for _, text := range records {
		rr := parseModern(t, text)
		zone[rr.Header().Name] = append(zone[rr.Header().Name], rr)
	}
	handler := &fixtureAuthority{t: t, zone: zone}
	go func() {
		if err := dns.ListenAndServe(":53", "tcp", handler); err != nil {
			t.Error(err)
		}
	}()
	if err := dns.ListenAndServe(":53", "udp", handler); err != nil {
		t.Fatal(err)
	}
}

type fixtureAuthority struct {
	t    *testing.T
	zone map[string][]dns.RR
}

func (a *fixtureAuthority) ServeDNS(_ context.Context, w dns.ResponseWriter, request *dns.Msg) {
	if err := request.Unpack(); err != nil {
		a.t.Errorf("decode authority request: %v", err)
		return
	}
	reply := new(dns.Msg)
	dnsutil.SetReply(reply, request)
	reply.Authoritative = true
	question := request.Question[0]
	qtype := dns.RRToType(question)
	for _, rr := range a.zone[question.Header().Name] {
		if dns.RRToType(rr) == qtype {
			reply.Answer = append(reply.Answer, rr)
		}
		if sig, ok := rr.(*dns.RRSIG); ok && request.Security && sig.TypeCovered == qtype {
			reply.Answer = append(reply.Answer, rr)
		}
	}
	if len(reply.Answer) == 0 {
		reply.Rcode = dns.RcodeRefused
	}
	if w.RemoteAddr().Network() == "udp" && reply.Len() > max(int(request.UDPSize), dns.MinMsgSize) {
		reply.Answer = nil
		reply.Truncated = true
	}
	if _, err := io.Copy(w, reply); err != nil {
		a.t.Errorf("write authority response: %v", err)
	}
}

func TestResolverMatrix(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	key, zone := signedZone(t, now)
	writeFixture(t, filepath.Join(dir, "zone.json"), zone)
	binary := filepath.Join(dir, "authority.test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// #nosec G204 -- The test controls the output path and every compiler argument.
	build := exec.CommandContext(ctx, "go", "test", "-c", "-o", binary)
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build authority: %v\n%s", err, output)
	}
	network := fmt.Sprintf("epochalypse-compat-%d", time.Now().UnixNano())
	docker(t, "network", "create", network)
	t.Cleanup(func() { docker(t, "network", "rm", network) })
	authority := startContainer(t, network, dir, "authority", "linux/arm64", bindImage,
		"/fixture/authority.test", "-test.run=^TestAuthorityProcess$", "-test.timeout=10m")
	assertClock(t, authority)
	ip := docker(t, "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", authority)
	for _, transport := range []string{"udp", "tcp"} {
		address := publishedAddress(t, authority, "53/"+transport)
		response := waitResponse(t, address, transport)
		if !response.Authoritative || response.RecursionAvailable || len(response.Answer) != 2 {
			t.Fatalf("direct authority flags or records: %s", response)
		}
		sig := response.Answer[1].(*dns.RRSIG)
		if sig.ValidPeriod(time.Now().UTC()) {
			t.Fatal("future signature is already valid")
		}
		t.Logf("direct %s: rcode=%d AA=%v RA=%v TXT visible before %s", transport, response.Rcode,
			response.Authoritative, response.RecursionAvailable, time.Unix(int64(sig.Inception), 0).UTC())
	}
	assertTruncation(t, authority)
	for _, resolver := range []string{"unbound-default", "unbound-strict", "bind"} {
		t.Run(resolver, func(t *testing.T) {
			for _, wrong := range []bool{false, true} {
				anchor := key
				if wrong {
					anchor, _ = syntheticKey(t)
				}
				config, entry, platform, image := resolverConfig(resolver, ip, anchor)
				file := resolver + fmt.Sprint(wrong) + ".conf"
				if err := os.WriteFile(filepath.Join(dir, file), []byte(config), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"-d", "-c", "/fixture/" + file}
				if resolver == "bind" {
					args = []string{"-g", "-c", "/fixture/" + file, "-n", "1"}
				}
				container := startContainer(t, network, dir, resolver, platform, image, entry, args...)
				versionFlag := "-V"
				t.Logf("executable identity: %s", docker(t, "exec", container, entry, versionFlag))
				address := publishedAddress(t, container, "53/udp")
				waitResponse(t, address, "udp")
				assertResolver(t, resolver, address, wrong)
			}
		})
	}
	t.Logf("fixture UTC=%s; resolver clocks use the same Docker VM; configured clock offset=0s", now.Format(time.RFC3339))
}

func assertClock(t *testing.T, authority string) {
	t.Helper()
	before := time.Now()
	output := docker(t, "exec", "--env", "COMPAT_CLOCK=1", authority,
		"/fixture/authority.test", "-test.run=^TestAuthorityProcess$")
	after := time.Now()
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "CLOCK=") {
			continue
		}
		stamp, err := strconv.ParseInt(strings.TrimPrefix(line, "CLOCK="), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		clock := time.Unix(0, stamp)
		lower, upper := clock.Sub(after), clock.Sub(before)
		if lower > time.Second || upper < -time.Second {
			t.Fatalf("Docker clock offset interval [%s, %s] exceeds one second", lower, upper)
		}
		t.Logf("Docker clock offset from host lies in [%s, %s]; all resolver containers share this clock", lower, upper)
		return
	}
	t.Fatal("authority does not report its clock")
}

func syntheticKey(t *testing.T) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	key := parseModern(t, "demo.example. 60 IN DNSKEY 257 3 13 AA==").(*dns.DNSKEY)
	private, err := key.Generate(256)
	if err != nil {
		t.Fatal(err)
	}
	return key, private.(crypto.Signer)
}

func signedZone(t *testing.T, now time.Time) (*dns.DNSKEY, []string) {
	t.Helper()
	key, signer := syntheticKey(t)
	var records []string
	add := func(rr dns.RR, inception, expiration time.Time) {
		sig := parseModern(t, fmt.Sprintf("%s 60 IN RRSIG %s 13 %d 60 %s %s %d demo.example. AA==",
			rr.Header().Name, dns.TypeToString[dns.RRToType(rr)], strings.Count(rr.Header().Name, "."),
			expiration.Format("20060102150405"), inception.Format("20060102150405"), key.KeyTag())).(*dns.RRSIG)
		if err := sig.Sign(signer, []dns.RR{rr}, &dns.SignOption{}); err != nil {
			t.Fatal(err)
		}
		records = append(records, rr.String(), sig.String())
	}
	add(key, now.Add(-24*time.Hour), now.Add(24*time.Hour))
	large := fmt.Sprintf(`large.demo.example. 60 IN TXT "%s" "%s" "cc"`, strings.Repeat("a", 255), strings.Repeat("b", 255))
	add(parseModern(t, large), now.Add(-time.Hour), now.Add(time.Hour))
	for _, name := range []string{"current", "future", "near", "expired", "tampered"} {
		inception, expiration := now.Add(-time.Hour), now.Add(time.Hour)
		switch name {
		case "future":
			inception, expiration = now.Add(24*time.Hour), now.Add(25*time.Hour)
		case "near":
			inception, expiration = now.Add(20*time.Minute), now.Add(80*time.Minute)
		case "expired":
			inception, expiration = now.Add(-25*time.Hour), now.Add(-24*time.Hour)
		}
		rr := parseModern(t, name+`.demo.example. 60 IN TXT "public synthetic content"`)
		add(rr, inception, expiration)
		if name == "tampered" {
			records[len(records)-2] = `tampered.demo.example. 60 IN TXT "changed content"`
		}
	}
	return key, records
}

func writeFixture(t *testing.T, path string, records []string) {
	t.Helper()
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// #nosec G204 -- Only this fixture supplies Docker arguments.
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func startContainer(t *testing.T, network, dir, label, platform, image, entry string, command ...string) string {
	t.Helper()
	name := network + "-" + label + fmt.Sprint(time.Now().UnixNano())
	args := []string{"run", "--detach", "--name", name, "--network", network, "--platform", platform,
		"--publish", "127.0.0.1::53/udp", "--publish", "127.0.0.1::53/tcp",
		"--mount", "type=bind,src=" + dir + ",dst=/fixture,readonly", "--env", "COMPAT_ZONE=/fixture/zone.json",
		"--entrypoint", entry, image}
	docker(t, append(args, command...)...)
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(docker(t, "logs", "--tail", "35", name))
		}
		docker(t, "rm", "--force", name)
	})
	return name
}

func publishedAddress(t *testing.T, container, port string) string {
	t.Helper()
	address := docker(t, "port", container, port)
	if _, _, err := net.SplitHostPort(address); err != nil {
		t.Fatalf("published address %q: %v", address, err)
	}
	return address
}

func query(address, transport, name string, cd bool) (*dns.Msg, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := dns.NewMsg(name+".demo.example.", dns.TypeTXT)
	request.RecursionDesired, request.Security, request.CheckingDisabled = true, true, cd
	request.UDPSize = 1232
	return dns.Exchange(ctx, request, transport, address)
}

func waitResponse(t *testing.T, address, transport string) *dns.Msg {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, err := query(address, transport, "future", true)
		if err == nil {
			return response
		}
		if time.Now().After(deadline) {
			t.Fatalf("DNS listener %s: %v", address, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func assertResolver(t *testing.T, resolver, address string, wrong bool) {
	t.Helper()
	for _, name := range []string{"current", "future", "near", "expired", "tampered"} {
		for _, cd := range []bool{false, true} {
			response, err := query(address, "udp", name, cd)
			if err != nil {
				t.Fatal(err)
			}
			valid := !wrong && (name == "current" || name == "near" && resolver == "unbound-default")
			wantCode := uint16(dns.RcodeServerFailure)
			if valid || cd {
				wantCode = dns.RcodeSuccess
			}
			t.Logf("%s wrong-anchor=%v name=%s CD=%v rcode=%d AD=%v RA=%v answers=%d", resolver, wrong,
				name, cd, response.Rcode, response.AuthenticatedData, response.RecursionAvailable, len(response.Answer))
			if response.Rcode != wantCode || response.AuthenticatedData != valid {
				t.Errorf("%s CD=%v wrong-anchor=%v: rcode=%d AD=%v; want rcode=%d AD=%v", name, cd, wrong,
					response.Rcode, response.AuthenticatedData, wantCode, valid)
			}
			if (valid || cd) && len(response.Answer) != 2 {
				t.Errorf("%s: want TXT and RRSIG, got %d records", name, len(response.Answer))
			}
		}
	}
}

func assertTruncation(t *testing.T, authority string) {
	t.Helper()
	request := dns.NewMsg("large.demo.example.", dns.TypeTXT)
	request.Security = true
	request.UDPSize = 512
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := dns.Exchange(ctx, request, "udp", publishedAddress(t, authority, "53/udp"))
	if err != nil {
		t.Fatal(err)
	}
	if !response.Truncated {
		t.Fatal("large UDP response does not request TCP fallback")
	}
	response, err = query(publishedAddress(t, authority, "53/tcp"), "tcp", "large", false)
	if err != nil {
		t.Fatal(err)
	}
	if response.Truncated || len(response.Answer) != 2 {
		t.Fatalf("TCP fallback loses records: %s", response)
	}
	txt := response.Answer[0].(*dns.TXT)
	if len(strings.Join(txt.Txt, "")) != 512 {
		t.Fatal("TCP fallback changes TXT content")
	}
	t.Log("512-byte TXT: UDP TC=true; explicit TCP fallback returns TXT and RRSIG")
}

func resolverConfig(resolver, authority string, key *dns.DNSKEY) (string, string, string, string) {
	if resolver == "bind" {
		config := fmt.Sprintf(`options {
 directory "/tmp";
 listen-on { any; }; listen-on-v6 { none; };
 recursion yes; allow-query { any; }; dnssec-validation yes;
 pid-file "/tmp/named.pid"; session-keyfile "/tmp/session.key";
};
trust-anchors { "demo.example." static-key %d %d %d "%s"; };
zone "demo.example." { type static-stub; server-addresses { %s; }; };
`, key.Flags, key.Protocol, key.Algorithm, key.PublicKey, authority)
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
 name: "demo.example."
 stub-addr: %s
 stub-first: no
`, key.String(), authority)
	if resolver == "unbound-strict" {
		config = strings.Replace(config, "server:\n", "server:\n val-sig-skew-min: 0\n val-sig-skew-max: 0\n", 1)
	}
	return config, "/opt/unbound/sbin/unbound", "linux/amd64", unboundImage
}

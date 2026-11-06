package atom

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"runtime"
	"strings"
	"sync/atomic"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/cmd/atomdns/atom/atomhttp"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/empty"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/global"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/metrics"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/refuse"
	"codeberg.org/miekg/dns/cmd/atomdns/handlers/unpack"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/conffile"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnslog"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/dnsserver"
	"codeberg.org/miekg/dns/cmd/atomdns/internal/zlog"
	"codeberg.org/miekg/dns/dnsutil"
	"github.com/caddyserver/certmagic"
	"golang.org/x/net/netutil"
)

type Server struct {
	global  *global.Global
	config  []byte
	version string // atomdns version

	mux     *dns.ServeMux
	servers []*dns.Server
	started chan error

	tlsservers []*dns.Server
	tlsstarted chan error

	httpservers []*atomhttp.Server
	httpstarted chan error

	unixservers []*dns.Server
	unixstarted chan error
}

// Start starts a server.
func (s *Server) Start() error {
	if err := s.global.Startup(); err != nil {
		return fmt.Errorf("global: %w", err)
	}

	for i := range s.servers {
		go Serve(s.started, s.servers[i])
	}
	// drain the channel, we either get a nil for success or otherwise an error _for each server_ started
	for range s.servers {
		err := <-s.started
		if err != nil {
			return fmt.Errorf("dns: %w", err)
		}
	}

	for i := range s.tlsservers {
		if !s.global.HasTls() {
			return fmt.Errorf("dot requires a tls block")
		}
		opt := func(srv *dns.Server) error {
			if x := s.global.TlsLimits.MaxInflight; x > 0 {
				srv.ListenFunc = func(s *dns.Server) {
					s.Listener = netutil.LimitListener(s.Listener, x)
				}
			}
			return nil
		}
		go Serve(s.tlsstarted, s.tlsservers[i], opt)
	}
	for range s.tlsservers {
		err := <-s.tlsstarted
		if err != nil {
			return fmt.Errorf("dot: %w", err)
		}
	}

	for i := range s.httpservers {
		if !s.global.HasTls() {
			return fmt.Errorf("doh requires a tls block")
		}
		go atomhttp.Serve(s.httpstarted, s.httpservers[i], s.global)
	}
	for range s.httpservers {
		if err := <-s.httpstarted; err != nil {
			return fmt.Errorf("doh: %w", err)
		}
	}

	for i := range s.unixservers {
		opt := func(srv *dns.Server) (err error) {
			srv.Net = "unix"
			srv.Listener, err = net.Listen("unix", s.unixservers[i].Addr)
			return err
		}
		go Serve(s.unixstarted, s.unixservers[i], opt)
	}
	for range s.unixservers {
		err := <-s.unixstarted
		if err != nil {
			return fmt.Errorf("dou: %w", err)
		}
	}

	attrs := []slog.Attr{}
	for _, addr := range s.Addr() {
		attrs = append(attrs, slog.String("dns", addr))
	}

	if s.global.TlsConfig != nil || s.global.TlsCertConfig != nil {
		if s.global.HttpLimits.Servers > 0 {
			for _, addr := range s.HttpAddr() {
				attrs = append(attrs, slog.String("doh", addr))
			}
		}
		if s.global.TlsLimits.Servers > 0 {
			for _, addr := range s.TlsAddr() {
				attrs = append(attrs, slog.String("dot", addr))
			}
		}
	}
	if s.global.UnixLimits.Servers > 0 {
		for _, addr := range s.UnixAddr() {
			attrs = append(attrs, slog.String("dou", addr))
		}
	}

	// bit of a cop out, but if roles is empty, we have nothing to run
	if len(attrs) == 0 {
		return fmt.Errorf("no servers defined, nothing to do")
	}

	if bi := builtinfo(); len(bi) == 4 {
		slog.Info("Build", bi[0], bi[1], bi[2], bi[3])
	}
	slog.Info("Listening", "total", len(attrs), "listen", attrs)
	slog.Info("Launched", "config", s.global.Config, "PID", os.Getpid(), "version", "v"+s.version, "dns", dns.Version, "zones", len(s.global.Registered))
	return nil
}

func Serve(ch chan error, srv *dns.Server, opts ...func(s *dns.Server) error) {
	for _, opt := range opts {
		if err := opt(srv); err != nil {
			ch <- err
			return
		}
	}
	if err := srv.ListenAndServe(); err != nil {
		ch <- err
		return
	}
}

// Shutdown shuts down a server.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.global.Shutdown(); err != nil {
		slog.Warn("Failed to run shutdown", dnslog.Error(err))
	}
	for _, srv := range s.servers {
		srv.Shutdown(ctx)
	}
	for _, srv := range s.tlsservers {
		srv.Shutdown(ctx)
	}
	for _, srv := range s.httpservers {
		srv.Shutdown(ctx)
	}
	for _, srv := range s.unixservers {
		srv.Shutdown(ctx)
	}
	return nil
}

// New returns a new server that has parsed the config in and r. If conf start with < and ends with > it's
// considered "not a file" and the contents of r is also stored in s.config.
func New(conf string, r io.Reader) (*Server, error) {
	s := &Server{mux: dns.NewServeMux()}
	w := &bytes.Buffer{}

	if builtin(conf) {
		r = io.TeeReader(r, w)
	}

	global, err := s.parse(conf, r)
	if err != nil {
		return nil, err
	}
	if builtin(conf) {
		s.config = w.Bytes()
	}
	s.global = global

	// dns servers
	for _, net := range []string{"udp", "tcp"} {
		for k := range global.Addrs {
			server := &dns.Server{
				ReuseAddr: true, ReusePort: true,
				Handler: s.mux, Net: net, Addr: global.Addrs[k], MaxTCPQueries: global.Limits.MaxTCPQueries,
			}
			msgInvalidFunc(server, global.MetricsN)
			server.NotifyStartedFunc = func(_ context.Context) { s.started <- nil }
			s.servers = append(s.servers, server)
		}
	}
	s.started = make(chan error, len(s.servers))

	// dot server
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if global.TlsConfig != nil {
		tlsConfig = global.TlsConfig.Clone()
	}
	if global.TlsCertConfig != nil {
		tlsConfig = global.TlsCertConfig.TLSConfig().Clone()
	}
	tlsConfig.NextProtos = dns.NextProtos
	for k := range global.TlsAddrs {
		server := &dns.Server{
			ReuseAddr:     true,
			ReusePort:     true,
			TLSConfig:     tlsConfig,
			Handler:       s.mux,
			Net:           "tcp",
			Addr:          global.TlsAddrs[k],
			MaxTCPQueries: global.TlsLimits.MaxTCPQueries,
		}
		msgInvalidFunc(server, global.MetricsN)
		server.NotifyStartedFunc = func(_ context.Context) { s.tlsstarted <- nil }
		s.tlsservers = append(s.tlsservers, server)
	}
	s.tlsstarted = make(chan error, len(s.tlsservers))

	// doh servers
	for k := range global.HttpAddrs {
		var i atomic.Uint64
		N := global.MetricsN
		server := atomhttp.New(global.HttpAddrs[k], s.mux, func(_ *dns.Msg, _ error) {
			if N == 0 {
				return
			}
			if (i.Add(1)-1)%N == 0 {
				metrics.Dropped.Inc()
			}
		})
		s.httpservers = append(s.httpservers, server)
	}
	s.httpstarted = make(chan error, len(s.httpservers))

	// dou servers
	for k := range global.UnixAddrs {
		server := &dns.Server{
			Handler:       s.mux,
			Net:           "unix",
			Addr:          global.UnixAddrs[k],
			MaxTCPQueries: global.UnixLimits.MaxTCPQueries,
		}
		msgInvalidFunc(server, global.MetricsN)
		server.NotifyStartedFunc = func(_ context.Context) { s.unixstarted <- nil }
		s.unixservers = append(s.unixservers, server)
	}
	s.unixstarted = make(chan error, len(s.unixservers))

	// Check if we need something else running on 443 to do the challenge for TLS certs.
	if global.TlsCertConfig != nil {
		host := ""
		for k := range global.HttpAddrs {
			h, p, _ := net.SplitHostPort(global.HttpAddrs[k])
			host = h
			if p == "443" {
				host = ""
				break
			}
		}

		if host != "" {
			addr := net.JoinHostPort(host, "443")
			slog.With("handler", "global").Info("Startup", "ACME", "[::]:443", "run", 1)
			s.httpservers = append(
				s.httpservers,
				atomhttp.New(addr, s.mux, func(_ *dns.Msg, _ error) {}),
			)
			s.httpstarted = make(chan error, len(s.httpservers))
		}
	}

	return s, nil
}

func (s *Server) parse(conf string, r io.Reader) (*global.Global, error) {
	blocks, err := conffile.Parse(conf, r)
	if err != nil {
		return nil, err
	}

	certmagic.Default.Logger = zlog.New(false)
	global := &global.Global{
		Registered: make(map[conffile.ZoneClass]struct{}),
		Config:     conf,
		Root:       func() string { wd, _ := os.Getwd(); return wd }(),
		Addrs:      []string{"[::]:53"},
		Limits:     global.Limits{MaxTCPQueries: dns.MaxTCPQueries, Servers: runtime.NumCPU() * 3},
	}
	global.OnReset(func() { global.Reset() })

	return global, s.Setup(conf, global, blocks)
}

func (s *Server) Setup(conf string, global *global.Global, blocks []conffile.HandlerBlock) error {
	for _, b := range blocks {
		if b.Keys != nil {
			continue
		}
		for _, dir := range b.Directives {
			d := conffile.NewDispenser(conf, nil, b.Tokens[dir], nil)
			err := global.Setup(d)
			if err != nil {
				return fmt.Errorf("could not parse global config: %w", err)
			}
		}
		global.OnStartup(func() error {
			slog.With("handler", "global").Info("Startup", "signal", "HUP")
			return nil
		})
		break
	}
	// reset for reload, s.mux is lock guarded, global.Registered is used in a non-concurrent way
	for zc := range global.Registered {
		s.mux.HandleRemove(zc.Zone, zc.Class)
	}
	global.Registered = map[conffile.ZoneClass]struct{}{}

	type teardown struct {
		name string
		fn   func(*dnsserver.Controller) error
	}

	const Unpack = 0
	for _, b := range blocks {
		if b.Keys == nil {
			continue
		}
		hs := []handlers.Handler{new(unpack.Unpack)}

		teardowns := []teardown{}
		names := []string{}

		// prepend unpack to start the chain
		for _, name := range b.Directives {
			names = append(names, name)
			newFn, ok := handlers.StringToHandler[name]
			if !ok {
				slog.With("handler", name).Warn("Unknown handler, skipping config")
				newFn = func() handlers.Handler { return new(empty.Empty) }
			}
			handler := newFn()
			if s, ok := handler.(handlers.Setupper); ok {
				co := &dnsserver.Controller{
					Dispenser: conffile.NewDispenser(conf, b.Keys, b.Tokens[name], names),
					Global:    global,
				}
				err := s.Setup(co)
				if err != nil {
					return handler.Err(fmt.Errorf("%w for '%s'", err, strings.Join(b.Keys, ",")))
				}
			}
			if t, ok := handler.(handlers.Teardowner); ok {
				teardowns = append(teardowns, teardown{name, t.Teardown})
			}

			if fn := handler.HandlerFunc(nil); fn != nil {
				// Do not add noop handler funcs.
				hs = append(hs, handler)
			} else {
				// Noop handler, check again if its a setupper, otherwise it isn't doing anything.
				if _, ok := handler.(handlers.Setupper); !ok {
					return fmt.Errorf("handler: %s, is a noop handler, but has no setup", name)
				}
			}
		}

		for _, teardown := range teardowns {
			co := &dnsserver.Controller{
				Dispenser: conffile.NewDispenser(conf, b.Keys, nil, nil),
				Global:    global,
			}
			err := teardown.fn(co)
			if err != nil {
				newFn := handlers.StringToHandler[teardown.name]
				handler := newFn()
				return handler.Err(fmt.Errorf("%w for '%s'", err, strings.Join(b.Keys, ",")))
			}
		}

		hs = append(hs, new(refuse.Refuse)) // add refuse guard

		for _, k := range b.Keys {
			class := uint16(dns.ClassINET)
			switch {
			case strings.HasSuffix(k, IN):
				k = strings.TrimSuffix(k, IN)
			case strings.HasSuffix(k, CH):
				k = strings.TrimSuffix(k, CH)
				class = dns.ClassCHAOS
			}

			k = dnsutil.Canonical(k)
			if _, ok := global.Registered[conffile.ZoneClass{Zone: k, Class: class}]; ok {
				return fmt.Errorf("origin already registered: %s", k)
			}

			if !global.Quiet {
				attrs := make([]slog.Attr, len(names))
				for i := range names {
					attrs[i] = slog.String("handler", names[i])
				}

				chaos := ""
				if class == dns.ClassCHAOS {
					chaos = CH
				}
				slog.Info(k+chaos, "handlers", slog.GroupValue(attrs...))
			}
			s.mux.HandleFunc(k, handlers.Compile(hs), class)
			global.Registered[conffile.ZoneClass{Zone: k, Class: class}] = struct{}{}
		}
	}
	return nil
}

// When a server is started on the wildcard port, this method can be used to get the actual address and
// listening port. Note that with a wildcard port the servers will all run on a different port. For all
// returned address the first half are the UDP listening port, the other half is TCP.
// See [Server.HttpAddr] for getting the addresss of the DOH server.
func (s *Server) Addr() []string {
	addr := make([]string, len(s.servers))
	for i, srv := range s.servers {
		if x := srv.PacketConn; x != nil {
			addr[i] = x.LocalAddr().String()
		}
		if x := srv.Listener; x != nil {
			addr[i] = x.Addr().String()
		}
	}
	return addr
}

// HttpAddr return the addresses of the DOH servers. See [Server.Addr].
func (s *Server) HttpAddr() []string {
	addr := make([]string, len(s.httpservers))
	for i, srv := range s.httpservers {
		addr[i] = srv.Listener.Addr().String()
	}
	return addr
}

// TlsAddr returns the addreses of the DOT servers. See [Server.Addr].
func (s *Server) TlsAddr() []string {
	addr := make([]string, len(s.tlsservers))
	for i, srv := range s.tlsservers {
		addr[i] = srv.Listener.Addr().String()
	}
	return addr
}

func (s *Server) UnixAddr() []string {
	addr := make([]string, len(s.unixservers))
	for i, srv := range s.unixservers {
		addr[i] = srv.Listener.Addr().String()
	}
	return addr
}

func builtin(conf string) bool { return strings.HasPrefix(conf, "<") && strings.HasSuffix(conf, ">") }

// msgInvalidFunc sets up the MsgInvalidFunc for a server.
func msgInvalidFunc(s *dns.Server, N uint64) {
	var i atomic.Uint64
	s.MsgInvalidFunc = func(_ *dns.Msg, _ error) {
		if N == 0 {
			return
		}
		if (i.Add(1)-1)%N == 0 {
			metrics.Dropped.Inc()
		}
	}
}

const (
	IN = "/IN"
	CH = "/CH"
)

package service

import (
	"errors"
	"html/template"
	"net"
	"net/http"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/Crank-Git/epochalypse/internal/authority"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/internal/window"
	"github.com/Crank-Git/epochalypse/web"
)

var publicWindowTemplate = template.Must(template.New("window").Parse(web.WindowHTML))

type directEvidence struct {
	State     string
	At        time.Time
	Transport string
	Records   []string
}

func (s *mainState) setDirectAddress(address net.Addr) {
	if s == nil {
		return
	}
	host, port, _ := net.SplitHostPort(address.String())
	// A wildcard listener needs a loopback destination for the owned probe.
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
		if ip.To4() == nil {
			host = "::1"
		}
	}
	s.directAddress = net.JoinHostPort(host, port)
}

func (s *mainState) publicWindowRoutes(mux *http.ServeMux, lab bool) {
	mux.Handle("GET /windows/{id}", owner.SecureHandler(lab, http.HandlerFunc(s.publicWindow)))
	for path, asset := range map[string]struct{ media, body string }{
		"/windows/style.css":  {"text/css; charset=utf-8", web.WindowCSS},
		"/windows/results.js": {"text/javascript; charset=utf-8", web.WindowJS},
	} {
		mux.Handle("GET "+path, owner.SecureHandler(lab, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", asset.media)
			_, _ = w.Write([]byte(asset.body))
		})))
	}
}

func (s *mainState) publicWindow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !observationID(id) {
		observationInvalid(w)
		return
	}
	if s.windows == nil {
		observationUnavailable(w)
		return
	}
	record, err := s.windows.Get(id)
	if errors.Is(err, window.ErrNotFound) {
		observationError(w, err)
		return
	}
	if err != nil {
		observationUnavailable(w)
		return
	}
	evidence := s.probeAuthority(r, record)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Public scripts need same-origin access. Owner routes retain their stricter policy.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	_ = publicWindowTemplate.Execute(w, struct {
		Window window.Window
		Direct directEvidence
	}{record, evidence})
}

func (s *mainState) probeAuthority(r *http.Request, record window.Window) directEvidence {
	evidence := directEvidence{State: "unavailable", At: time.Now().UTC()}
	if s.directAddress == "" {
		return evidence
	}
	select {
	case s.directSlots <- struct{}{}:
		defer func() { <-s.directSlots }()
	default:
		return evidence
	}
	request := dns.NewMsg(record.Name, dns.TypeTXT)
	request.Security = true
	request.UDPSize = 1232
	response, transport, err := authority.Query(r.Context(), s.directAddress, request)
	if err != nil || response.Rcode != dns.RcodeSuccess || !response.Authoritative || response.Truncated || len(response.Answer) > 16 {
		return evidence
	}
	var txt, sig bool
	for _, rr := range response.Answer {
		switch value := rr.(type) {
		case *dns.TXT:
			txt = value.String() == record.TXT
		case *dns.RRSIG:
			sig = value.String() == record.RRSIG
		}
		evidence.Records = append(evidence.Records, rr.String())
	}
	if txt && sig {
		evidence.State = "observed"
		evidence.Transport = transport
	}
	return evidence
}

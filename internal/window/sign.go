package window

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"codeberg.org/miekg/dns"
)

// Signer holds the configured zone key. It never creates a production key.
type Signer struct {
	key     *dns.DNSKEY
	private crypto.Signer
}

// LoadSigner reads a protected BIND private key and its sibling .key file.
func LoadSigner(zone, privatePath string) (*Signer, error) {
	if !validZone(zone) || !strings.HasSuffix(privatePath, ".private") {
		return nil, errors.New("set a dedicated canonical subdomain and a .private key file")
	}
	public, err := readKeyFile(strings.TrimSuffix(privatePath, ".private")+".key", false)
	if err != nil {
		return nil, err
	}
	rr, err := dns.New(strings.TrimSpace(string(public)))
	if err != nil {
		return nil, errors.New("invalid public DNSKEY")
	}
	key, ok := rr.(*dns.DNSKEY)
	if !ok || key.Hdr.Name != zone || key.Algorithm != dns.ECDSAP256SHA256 || key.Flags != 257 || key.Protocol != 3 {
		return nil, errors.New("require a zone-matched ECDSAP256SHA256 DNSKEY with flags 257 and protocol 3")
	}
	private, err := readKeyFile(privatePath, true)
	if err != nil {
		return nil, err
	}
	material, err := key.NewPrivate(string(private))
	if err != nil {
		// Parser errors can contain private key fields.
		return nil, errors.New("invalid private DNSSEC key")
	}
	p, ok := material.(*ecdsa.PrivateKey)
	if !ok || !matchingKey(p) || key.KeyTag() == 0 {
		return nil, errors.New("private DNSSEC key does not match the public DNSKEY")
	}
	key.Hdr.TTL = 60
	return &Signer{key: key, private: p}, nil
}

func readKeyFile(path string, private bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect DNSSEC key file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 16384 || (private && info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("require a bounded regular DNSSEC key file with owner-only private access")
	}
	// The host administrator selects the key reference.
	file, err := os.Open(path) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("open DNSSEC key file: %w", err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("DNSSEC key file changes during access")
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(data) > 16384 {
		return nil, errors.New("cannot read bounded DNSSEC key file")
	}
	return data, nil
}

func matchingKey(p *ecdsa.PrivateKey) bool {
	if p.Curve != elliptic.P256() {
		return false
	}
	raw, err := p.Bytes()
	if err != nil {
		return false
	}
	derived, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	return err == nil && derived.PublicKey.Equal(&p.PublicKey)
}

func validZone(zone string) bool {
	// The generated 33-byte label and wire terminator must fit the 255-byte name limit.
	if len(zone) > 220 || !strings.HasSuffix(zone, ".") || zone != strings.ToLower(zone) {
		return false
	}
	labels := strings.Split(strings.TrimSuffix(zone, "."), ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !validLabel(label) {
			return false
		}
	}
	return true
}

func validLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func validID(id string) bool {
	_, err := hex.DecodeString(id)
	return len(id) == 32 && id == strings.ToLower(id) && err == nil
}

// Window contains immutable public records and their versioned boundaries.
type Window struct {
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Zone       string    `json:"zone"`
	Name       string    `json:"name"`
	Inception  time.Time `json:"inception"`
	Expiration time.Time `json:"expiration"`
	DNSKEY     string    `json:"dnskey"`
	TXT        string    `json:"txt"`
	RRSIG      string    `json:"rrsig"`
}

func (s *Signer) create(id string, in Input, now time.Time) (Window, error) {
	if err := in.Validate(now); err != nil {
		return Window{}, err
	}
	if !validID(id) {
		return Window{}, errors.New("invalid window identifier")
	}
	inception, expiration, _ := in.boundaries()
	w := Window{Version: 1, ID: id, Content: in.Content, Zone: s.key.Hdr.Name,
		Name: "w" + id + "." + s.key.Hdr.Name, Inception: inception, Expiration: expiration, DNSKEY: s.key.String()}
	txt := makeTXT(w.Name, in.Content)
	// The experiment stores exactly one signature with the owner's boundaries.
	sig := dns.NewRRSIG(w.Zone, s.key.Algorithm, s.key.KeyTag(), uint32(inception.Unix()), uint32(expiration.Unix())) // #nosec G115 -- Validate bounds both timestamps.
	if err := sig.Sign(s.private, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		return Window{}, fmt.Errorf("sign public TXT: %w", err)
	}
	w.TXT, w.RRSIG = txt.String(), sig.String()
	return w, nil
}

func makeTXT(name, content string) *dns.TXT {
	txt := &dns.TXT{Hdr: dns.Header{Name: name, TTL: 60, Class: dns.ClassINET}}
	for len(content) > 255 {
		txt.Txt = append(txt.Txt, escapeTXT(content[:255]))
		content = content[255:]
	}
	txt.Txt = append(txt.Txt, escapeTXT(content))
	return txt
}

func escapeTXT(content string) string {
	// The adapter consumes presentation escapes, including backslashes in content.
	var escaped strings.Builder
	for i := range len(content) {
		_, _ = fmt.Fprintf(&escaped, "\\%03d", content[i])
	}
	return escaped.String()
}

// Records returns independent records after a local signature check.
func (w Window) Records() (*dns.TXT, *dns.RRSIG, *dns.DNSKEY, error) {
	if err := w.validateMetadata(); err != nil {
		return nil, nil, nil, err
	}
	text, textErr := dns.New(w.TXT)
	signature, sigErr := dns.New(w.RRSIG)
	public, keyErr := dns.New(w.DNSKEY)
	txt, textOK := text.(*dns.TXT)
	sig, sigOK := signature.(*dns.RRSIG)
	key, keyOK := public.(*dns.DNSKEY)
	if textErr != nil || sigErr != nil || keyErr != nil || !textOK || !sigOK || !keyOK {
		return nil, nil, nil, errors.New("invalid window records")
	}
	if err := w.verifyRecords(txt, sig, key); err != nil {
		return nil, nil, nil, err
	}
	return txt, sig, key, nil
}

func (w Window) validateMetadata() error {
	if w.Version != 1 || !validID(w.ID) || !validZone(w.Zone) || w.Name != "w"+w.ID+"."+w.Zone {
		return errors.New("invalid window identity")
	}
	in := Input{w.Content, w.Inception.Format(time.RFC3339Nano), w.Expiration.Format(time.RFC3339Nano)}
	// Old records remain readable after their validity interval ends.
	return in.Validate(w.Inception)
}

func (w Window) verifyRecords(txt *dns.TXT, sig *dns.RRSIG, key *dns.DNSKEY) error {
	if txt.String() != makeTXT(w.Name, w.Content).String() || key.Hdr.Name != w.Zone || key.Algorithm != dns.ECDSAP256SHA256 || key.Flags != 257 || key.Protocol != 3 {
		return errors.New("window records differ from the public metadata")
	}
	if sig.Hdr.Name != w.Name || sig.Inception != uint32(w.Inception.Unix()) || sig.Expiration != uint32(w.Expiration.Unix()) { // #nosec G115 -- validateMetadata bounds both timestamps.
		return errors.New("window signature differs from the explicit boundaries")
	}
	if err := sig.Verify(key, []dns.RR{txt}, &dns.SignOption{}); err != nil {
		return errors.New("invalid public TXT signature")
	}
	return nil
}

// Package countdown retains absolute public event times independently of DNS signatures.
package countdown

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"codeberg.org/miekg/dns"
)

// MaxTTL bounds admission and the signed Original TTL.
const MaxTTL = 2147483647

var (
	ErrInput       = errors.New("invalid countdown input")
	ErrNotFound    = errors.New("countdown does not exist")
	ErrIntegrity   = errors.New("invalid retained countdown")
	ErrUnavailable = errors.New("countdown creation is unavailable")
)

// Input contains the owner's absolute UTC event time.
type Input struct {
	Title   string `json:"title"`
	EventAt string `json:"event_at"`
}

// Validate returns the future event in whole UTC seconds within the supported horizon.
func (in Input) Validate(now time.Time) (time.Time, error) {
	event, err := time.Parse(time.RFC3339, in.EventAt)
	if err != nil || event.Format(time.RFC3339) != in.EventAt || len(in.EventAt) != 20 || !strings.HasSuffix(in.EventAt, "Z") {
		return time.Time{}, ErrInput
	}
	if !validTitle(in.Title) || now.IsZero() || !event.After(now) || event.Sub(now) > MaxTTL*time.Second {
		return time.Time{}, ErrInput
	}
	return event.UTC(), nil
}

func validTitle(title string) bool {
	return strings.TrimSpace(title) != "" && len(title) <= 128 && utf8.ValidString(title)
}

// Record preserves immutable event metadata after the event completes.
type Record struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	EventAt   time.Time `json:"event_at"`
	CreatedAt time.Time `json:"created_at"`
	Zone      string    `json:"zone"`
	Name      string    `json:"name"`
}

// TXT returns an independent static RRset member with the maximum signed Original TTL.
// The authoritative adapter lowers only the emitted TTL after signature creation.
func (r Record) TXT() *dns.TXT {
	return &dns.TXT{Hdr: dns.Header{Name: r.Name, Class: dns.ClassINET, TTL: MaxTTL}, Txt: []string{"v=epochalypse1 event=" + r.EventAt.UTC().Format(time.RFC3339)}}
}

func (r Record) validate() error {
	if r.Version != 1 || !validID(r.ID) || !ValidZone(r.Zone) || r.Name != "c"+r.ID+"."+r.Zone {
		return ErrIntegrity
	}
	if r.CreatedAt.Location() != time.UTC || r.CreatedAt.Nanosecond() != 0 || r.EventAt.Location() != time.UTC || r.EventAt.Nanosecond() != 0 {
		return ErrIntegrity
	}
	// Admission uses the precise clock; truncation can add less than one second to the retained interval.
	if !validTitle(r.Title) || r.CreatedAt.IsZero() || !r.EventAt.After(r.CreatedAt) || r.EventAt.Sub(r.CreatedAt) > MaxTTL*time.Second {
		return ErrIntegrity
	}
	return nil
}

func validID(id string) bool {
	_, err := hex.DecodeString(id)
	return len(id) == 32 && id == strings.ToLower(id) && err == nil
}

// ValidZone requires a canonical subdomain with room for the opaque event label.
func ValidZone(zone string) bool {
	if len(zone) > 220 || zone != strings.ToLower(zone) || !strings.HasSuffix(zone, ".") {
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

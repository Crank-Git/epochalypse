package dns

import (
	"context"
	"fmt"
	"testing"
)

func TestServeMux(t *testing.T) {
	mux := NewServeMux()
	noopHandler := func(ctx context.Context, w ResponseWriter, req *Msg) {}
	mux.Handle("grand.child.miek.nl.", HandlerFunc(noopHandler))
	mux.Handle("child.miek.nl.", HandlerFunc(noopHandler))
	mux.Handle("miek.nl.", HandlerFunc(noopHandler))
	mux.Handle(".", HandlerFunc(noopHandler))

	testcases := []struct {
		zone string
		typ  uint16
		exp  string
	}{
		{"child.miek.nl.", TypeDS, "miek.nl."},
		{"grand.child.miek.nl.", TypeDS, "child.miek.nl."},
		{"miek.nl.", TypeDS, "miek.nl."},
		{".", TypeDS, "."},
		// Names below an apex are not redirected, their DS (or the proof it does not exist) is in the child.
		{"foo.grand.child.miek.nl.", TypeDS, "grand.child.miek.nl."},
		{"foo.child.miek.nl.", TypeDS, "child.miek.nl."},
		{"foo.miek.nl.", TypeDS, "miek.nl."},
		//
		{"child.miek.nl.", TypeA, "child.miek.nl."},
		{"foo.child.miek.nl.", TypeA, "child.miek.nl."},
		{"foo.miek.nl.", TypeA, "miek.nl."},
		{"miek.nl.", TypeA, "miek.nl."},
		{".", TypeA, "."},
		{"bla.nl.", TypeA, ""},
	}
	for _, tc := range testcases {
		t.Run(fmt.Sprintf("%s/%s", tc.zone, typeToString(tc.typ)), func(t *testing.T) {
			_, zone := mux.match(tc.zone, tc.typ, ClassINET)
			if zone != tc.exp {
				t.Errorf("expected %s for %d, got %s", tc.exp, tc.typ, zone)
			}
		})
	}
}

func BenchmarkServeMux(b *testing.B) {
	mux := NewServeMux()
	noopHandler := func(ctx context.Context, w ResponseWriter, req *Msg) {}
	mux.Handle("_udp.example.org.", HandlerFunc(noopHandler))

	for b.Loop() {
		mux.match("_dns._udp.example.com.", TypeSRV, ClassINET)
		mux.match("miek.nl.", TypeSRV, ClassINET)
	}
}

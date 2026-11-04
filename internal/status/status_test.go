package status

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckRejectsUnconfirmedIdentity(t *testing.T) {
	sha := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_, _ = w.Write([]byte(`{"commit":"` + sha + `"}`))
		default:
			_, _ = w.Write([]byte("ok\n"))
		}
	}))
	defer server.Close()
	for _, endpoints := range [][]string{nil, {"http://127.0.0.1:0"}, {"%"}, {"file:///tmp"}, {"http://user:secret@localhost"}, {server.URL + "?q=1"}, {server.URL + "#fragment"}, {server.URL + "/path"}, {"http:///"}} {
		state, commit := Check(endpoints, sha)
		if state != "unknown" || commit != "unknown" {
			t.Fatalf("%v: %s %s", endpoints, state, commit)
		}
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"commit":"` + strings.Repeat("b", 40) + `"}`))
	}))
	defer other.Close()
	state, commit := Check([]string{server.URL, other.URL}, sha)
	if state != "unknown" || commit != "unknown" {
		t.Fatalf("mixed versions: %s %s", state, commit)
	}
}

func TestCheckRejectsMalformedResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"commit":"unknown"}`, `{"commit":"` + strings.Repeat("A", 40) + `"}`, `{"commit":"` + strings.Repeat("z", 40) + `"}`, strings.Repeat("x", 4097), `{} {}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		state, _ := Check([]string{server.URL}, strings.Repeat("a", 40))
		server.Close()
		if state != "unknown" {
			t.Fatalf("body %q: %s", body, state)
		}
	}
	for _, code := range []int{http.StatusServiceUnavailable, http.StatusFound} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "/version")
			w.WriteHeader(code)
		}))
		state, _ := Check([]string{server.URL}, strings.Repeat("a", 40))
		server.Close()
		if state != "unknown" {
			t.Fatal(state)
		}
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

type responseTransport struct{}

func (responseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(brokenReader{})}, nil
}

func TestFetchReportsReadFailure(t *testing.T) {
	if _, err := fetch(&http.Client{Transport: responseTransport{}}, "http://localhost"); err == nil {
		t.Fatal("read failure accepted")
	}
}

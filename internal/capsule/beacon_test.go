package capsule

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type beaconTransport func(*http.Request) (*http.Response, error)

func (f beaconTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBoundsPinnedBeaconFetch(t *testing.T) {
	for _, name := range []string{"valid", "network", "missing", "oversize", "round", "hex", "json", "read"} {
		t.Run(name, func(t *testing.T) {
			relay := beaconRelay{client: &http.Client{Transport: beaconTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != "https://api.drand.sh/"+QuicknetChainHash+"/public/1000" {
					t.Fatal("relay identity differs")
				}
				if _, ok := request.Context().Deadline(); !ok {
					t.Fatal("request lacks deadline")
				}
				if name == "network" {
					return nil, errors.New("private relay error")
				}
				return beaconResponse(name), nil
			})}}
			signature, err := relay.Fetch(context.Background(), 1000)
			if name == "valid" {
				if err != nil || len(signature) != 48 {
					t.Fatalf("valid response: %v", err)
				}
				return
			}
			if err == nil || len(signature) != 0 {
				t.Fatal("invalid response returns a signature")
			}
		})
	}
	if _, err := QuicknetRelay().Fetch(context.Background(), 0); !errors.Is(err, ErrInput) {
		t.Fatal("zero round succeeds")
	}
}

func beaconResponse(name string) *http.Response {
	body := `{"round":1000,"signature":"` + strings.Repeat("ab", 48) + `","randomness":"ignored"}`
	status := http.StatusOK
	switch name {
	case "missing":
		status = http.StatusNotFound
	case "oversize":
		body = strings.Repeat("x", beaconResponseLimit+1)
	case "round":
		body = strings.Replace(body, "1000", "1001", 1)
	case "hex":
		body = strings.Replace(body, "ab", "zz", 1)
	case "json":
		body = "{}"
	case "read":
		return &http.Response{StatusCode: status, Body: io.NopCloser(privateReaderFailure{})}
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

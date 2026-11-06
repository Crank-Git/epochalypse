package switchclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

// Metadata retrieves bounded public metadata through the existing TLS identity pins.
func (h *HTTPSReplica) Metadata(ctx context.Context, endpoint, path string) ([]byte, int, error) {
	if path != "/healthz" && path != "/readyz" && path != "/version" {
		return nil, 0, errors.New("invalid metadata path")
	}
	transport, err := h.transport(switchwire.Agent{Endpoint: endpoint})
	if err != nil {
		return nil, 0, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("metadata redirect refused") }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+path, nil)
	if err != nil {
		return nil, 0, errors.New("invalid metadata endpoint")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, errors.New("metadata HTTPS request failed")
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(data) > 4096 {
		return nil, response.StatusCode, errors.New("invalid metadata response")
	}
	return data, response.StatusCode, nil
}

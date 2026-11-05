package capsule

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const beaconResponseLimit = 8192

type beaconRelay struct{ client *http.Client }

// QuicknetRelay uses one fixed HTTPS relay with no redirect or fallback.
func QuicknetRelay() BeaconSource {
	return beaconRelay{client: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Fetch returns one bounded signature without trusting the relay's authenticity.
func (r beaconRelay) Fetch(ctx context.Context, round uint64) ([]byte, error) {
	if round == 0 {
		return nil, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url := "https://api.drand.sh/" + QuicknetChainHash + "/public/" + strconv.FormatUint(round, 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, beaconResponseLimit+1))
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(data) > beaconResponseLimit {
		return nil, ErrIntegrity
	}
	return decodeBeacon(data, round)
}

func decodeBeacon(data []byte, round uint64) ([]byte, error) {
	var beacon struct {
		Round     uint64 `json:"round"`
		Signature string `json:"signature"`
	}
	if json.Unmarshal(data, &beacon) != nil || beacon.Round != round || len(beacon.Signature) != 96 {
		return nil, ErrIntegrity
	}
	signature, err := hex.DecodeString(beacon.Signature)
	if err != nil {
		return nil, ErrIntegrity
	}
	return signature, nil
}

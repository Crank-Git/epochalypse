package capsule

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/drand/tlock"
)

func TestSelectsCeilingQuicknetRound(t *testing.T) {
	for _, test := range []struct {
		requested int64
		round     uint64
		effective int64
	}{
		{1692803367, 1, 1692803367}, {1692806364, 1000, 1692806364},
		{1692806365, 1001, 1692806367}, {1692806367, 1001, 1692806367},
	} {
		round, effective, err := SelectRound(time.Unix(test.requested, 0).UTC())
		if err != nil || round != test.round || effective.Unix() != test.effective {
			t.Fatalf("request %d: got %d, %v, %v", test.requested, round, effective, err)
		}
	}
	if _, _, err := SelectRound(time.Unix(1692806364, 1).UTC()); !errors.Is(err, ErrInput) {
		t.Fatal("subsecond request succeeds")
	}
	if _, _, err := SelectRound(time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)); !errors.Is(err, ErrInput) {
		t.Fatal("effective date exceeds supported time")
	}
}

type savedBeacon struct {
	signature []byte
	err       error
	calls     int
}

func (s *savedBeacon) Fetch(_ context.Context, round uint64) ([]byte, error) {
	s.calls++
	if round != 1000 {
		return nil, ErrUnavailable
	}
	return bytes.Clone(s.signature), s.err
}

func fixtureBeacon(t *testing.T) *savedBeacon {
	t.Helper()
	data, err := os.ReadFile("../../tests/compatibility/tlock/testdata/quicknet-1000.json")
	if err != nil {
		t.Fatal(err)
	}
	var beacon struct{ Signature string }
	if err := json.Unmarshal(data, &beacon); err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(beacon.Signature)
	if err != nil {
		t.Fatal(err)
	}
	return &savedBeacon{signature: signature}
}

func TestWrapsFreshKeyForPinnedRound(t *testing.T) {
	key := [32]byte{42}
	requested := time.Unix(1692806364, 0).UTC()
	wrapped, err := WrapKey(key, requested)
	if err != nil {
		t.Fatal(err)
	}
	other, err := WrapKey(key, requested)
	if err != nil || wrapped.WrappedKey == other.WrappedKey {
		t.Fatal("wrapper is not fresh")
	}
	opened, err := wrapped.OpenKey(context.Background(), fixtureBeacon(t), requested)
	if err != nil || opened != key {
		t.Fatalf("key differs: %v", err)
	}
	if wrapped.ChainHash != QuicknetChainHash || wrapped.Round != 1000 || wrapped.PublicKey != QuicknetPublicKey {
		t.Fatal("identity differs")
	}
}

func TestRejectsInvalidTimelockWithoutKey(t *testing.T) {
	requested := time.Unix(1692806364, 0).UTC()
	for _, name := range []string{"early", "missing", "signature", "chain", "public key", "round", "version", "corrupt", "oversize", "canceled"} {
		t.Run(name, func(t *testing.T) {
			wrapped, err := WrapKey([32]byte{42}, requested)
			if err != nil {
				t.Fatal(err)
			}
			source := fixtureBeacon(t)
			now := requested
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := ErrIntegrity
			switch name {
			case "early":
				now = now.Add(-time.Second)
				want = ErrTooEarly
			case "missing":
				source.err = errors.New("private relay error")
				want = ErrUnavailable
			case "signature":
				source.signature[0] ^= 1
			case "chain":
				wrapped.ChainHash = "other"
			case "public key":
				wrapped.PublicKey = "other"
			case "canceled":
				cancel()
				want = context.Canceled
			default:
				alterTimelock(&wrapped, name)
			}
			key, err := wrapped.OpenKey(ctx, source, now)
			if !errors.Is(err, want) || key != [32]byte{} {
				t.Fatalf("got key presence %t, error %v; want %v", key != [32]byte{}, err, want)
			}
			if name == "early" && source.calls != 0 {
				t.Fatal("early open fetches beacon")
			}
		})
	}
}

func alterTimelock(wrapped *Timelock, name string) {
	switch name {
	case "round":
		wrapped.Round++
	case "version":
		wrapped.Version++
	case "corrupt":
		wrapped.WrappedKey = wrapped.WrappedKey[:len(wrapped.WrappedKey)-2] + "aa"
	case "oversize":
		wrapped.WrappedKey = string(bytes.Repeat([]byte("a"), 4097))
	}
}

func TestPreservesLegacyAgeCompatibility(t *testing.T) {
	data, err := os.ReadFile("../../tests/compatibility/tlock/testdata/synthetic.age")
	if err != nil {
		t.Fatal(err)
	}
	network := newPinnedNetwork(1000)
	network.signature = fixtureBeacon(t).signature
	var output bytes.Buffer
	if err := tlock.New(network).Strict().Decrypt(&output, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if output.String() != "epochalypse synthetic capsule fixture\n" {
		t.Fatal("legacy plaintext differs")
	}
}

func TestRejectsWrongWrapperChainAndExcessiveKey(t *testing.T) {
	requested := time.Unix(1692806364, 0).UTC()
	wrapped, err := WrapKey([32]byte{42}, requested)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.RawURLEncoding.DecodeString(wrapped.WrappedKey)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(QuicknetChainHash), bytes.Repeat([]byte("0"), 64), 1)
	wrapped.WrappedKey = base64.RawURLEncoding.EncodeToString(data)
	source := fixtureBeacon(t)
	if key, err := wrapped.OpenKey(context.Background(), source, requested); !errors.Is(err, ErrIntegrity) || key != [32]byte{} || source.calls != 0 {
		t.Fatal("wrong wrapper chain succeeds or fetches a beacon")
	}
	var buffer keyBuffer
	if _, err := buffer.Write(make([]byte, 33)); !errors.Is(err, ErrIntegrity) || buffer.used != 0 {
		t.Fatal("excessive key enters the buffer")
	}
}

package capsule

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestSelectsCeilingQuicknetRound(t *testing.T) {
	for _, test := range []struct{ requested int64; round uint64; effective int64 }{
		{1692803367, 1, 1692803367}, {1692806364, 1000, 1692806364},
		{1692806365, 1001, 1692806367}, {1692806367, 1001, 1692806367},
	} {
		round, effective, err := SelectRound(time.Unix(test.requested, 0).UTC())
		if err != nil || round != test.round || effective.Unix() != test.effective {
			t.Fatalf("request %d: got %d, %v, %v", test.requested, round, effective, err)
		}
	}
	if _, _, err := SelectRound(time.Unix(1692806364, 1).UTC()); !errors.Is(err, ErrInput) { t.Fatal("subsecond request succeeds") }
}

type savedBeacon struct { signature []byte; err error; calls int }
func (s *savedBeacon) Fetch(_ context.Context, round uint64) ([]byte, error) {
	s.calls++
	if round != 1000 { return nil, ErrUnavailable }
	return bytes.Clone(s.signature), s.err
}

func fixtureBeacon(t *testing.T) *savedBeacon {
	t.Helper()
	data, err := os.ReadFile("../../tests/compatibility/tlock/testdata/quicknet-1000.json")
	if err != nil { t.Fatal(err) }
	var beacon struct { Signature string }
	if err := json.Unmarshal(data, &beacon); err != nil { t.Fatal(err) }
	signature, err := hex.DecodeString(beacon.Signature)
	if err != nil { t.Fatal(err) }
	return &savedBeacon{signature: signature}
}

func TestWrapsFreshKeyForPinnedRound(t *testing.T) {
	key := [32]byte{42}
	requested := time.Unix(1692806364, 0).UTC()
	wrapped, err := WrapKey(key, requested)
	if err != nil { t.Fatal(err) }
	other, err := WrapKey(key, requested)
	if err != nil || wrapped.WrappedKey == other.WrappedKey { t.Fatal("wrapper is not fresh") }
	opened, err := wrapped.OpenKey(context.Background(), fixtureBeacon(t), requested)
	if err != nil || opened != key { t.Fatalf("key differs: %v", err) }
	if wrapped.ChainHash != QuicknetChainHash || wrapped.Round != 1000 || wrapped.PublicKey != QuicknetPublicKey { t.Fatal("identity differs") }
}

func TestRejectsInvalidTimelockWithoutKey(t *testing.T) {
	requested := time.Unix(1692806364, 0).UTC()
	for _, name := range []string{"early", "missing", "signature", "chain", "public key", "round", "version", "corrupt", "oversize", "canceled"} {
		t.Run(name, func(t *testing.T) {
			wrapped, err := WrapKey([32]byte{42}, requested)
			if err != nil { t.Fatal(err) }
			source := fixtureBeacon(t)
			now := requested
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := ErrIntegrity
			switch name {
			case "early": now = now.Add(-time.Second); want = ErrTooEarly
			case "missing": source.err = errors.New("private relay error"); want = ErrUnavailable
			case "signature": source.signature[0] ^= 1
			case "chain": wrapped.ChainHash = "other"
			case "public key": wrapped.PublicKey = "other"
			case "round": wrapped.Round++
			case "version": wrapped.Version++
			case "corrupt": wrapped.WrappedKey = wrapped.WrappedKey[:len(wrapped.WrappedKey)-2] + "aa"
			case "oversize": wrapped.WrappedKey = string(bytes.Repeat([]byte("a"), 4097))
			case "canceled": cancel(); want = context.Canceled
			}
			key, err := wrapped.OpenKey(ctx, source, now)
			if !errors.Is(err, want) || key != [32]byte{} { t.Fatalf("got key presence %t, error %v; want %v", key != [32]byte{}, err, want) }
			if name == "early" && source.calls != 0 { t.Fatal("early open fetches beacon") }
		})
	}
}

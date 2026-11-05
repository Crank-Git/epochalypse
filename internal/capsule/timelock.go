package capsule

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	chain "github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/kyber"
	"github.com/drand/tlock"
)

const QuicknetChainHash = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
const QuicknetPublicKey = "83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a"

const quicknetGenesis int64 = 1692803367
const quicknetPeriod int64 = 3
const quicknetScheme = "bls-unchained-g1-rfc9380"
const wrappedKeyLimit = 2048

// Timelock retains the exact release identity and the bounded age key wrapper.
type Timelock struct {
	Version     int       `json:"version"`
	ChainHash   string    `json:"chain_hash"`
	PublicKey   string    `json:"public_key"`
	Scheme      string    `json:"scheme"`
	Genesis     int64     `json:"genesis"`
	Period      int64     `json:"period"`
	Round       uint64    `json:"round"`
	RequestedAt time.Time `json:"requested_at"`
	EffectiveAt time.Time `json:"effective_at"`
	WrappedKey  string    `json:"wrapped_key"`
}

// BeaconSource retrieves a signature for one exact quicknet round.
type BeaconSource interface {
	Fetch(context.Context, uint64) ([]byte, error)
}

// SelectRound returns the first quicknet round at or after a UTC second.
func SelectRound(requested time.Time) (uint64, time.Time, error) {
	if !validStoredTime(requested) || requested.Unix() < 0 {
		return 0, time.Time{}, ErrInput
	}
	offset := max(int64(0), requested.Unix()-quicknetGenesis)
	steps := offset / quicknetPeriod
	if offset%quicknetPeriod != 0 {
		steps++
	}
	effective := time.Unix(quicknetGenesis+steps*quicknetPeriod, 0).UTC()
	if !validStoredTime(effective) {
		return 0, time.Time{}, ErrInput
	}
	return uint64(steps) + 1, effective, nil // #nosec G115 -- The bounded UTC timestamp keeps steps nonnegative.
}

type pinnedNetwork struct {
	key       kyber.Point
	scheme    crypto.Scheme
	round     uint64
	signature []byte
}

func newPinnedNetwork(round uint64) pinnedNetwork {
	scheme, _ := crypto.SchemeFromName(quicknetScheme)
	key := scheme.KeyGroup.Point()
	encoded, _ := hex.DecodeString(QuicknetPublicKey)
	// The pinned fixture tests the complete public key before this adapter ships.
	_ = key.UnmarshalBinary(encoded)
	return pinnedNetwork{key: key, scheme: *scheme, round: round}
}

func (n pinnedNetwork) ChainHash() string            { return QuicknetChainHash }
func (n pinnedNetwork) PublicKey() kyber.Point       { return n.key }
func (n pinnedNetwork) Scheme() crypto.Scheme        { return n.scheme }
func (n pinnedNetwork) Current(time.Time) uint64     { return n.round }
func (n pinnedNetwork) SwitchChainHash(string) error { return ErrIntegrity }
func (n pinnedNetwork) Signature(round uint64) ([]byte, error) {
	if round != n.round || len(n.signature) != 48 {
		return nil, ErrIntegrity
	}
	return n.signature, nil
}

// WrapKey encrypts a fresh content key without a network request.
func WrapKey(key [32]byte, requested time.Time) (Timelock, error) {
	round, effective, err := SelectRound(requested)
	if err != nil {
		return Timelock{}, err
	}
	var output bytes.Buffer
	if err := tlock.New(newPinnedNetwork(round)).Strict().Encrypt(&output, bytes.NewReader(key[:]), round); err != nil {
		return Timelock{}, ErrIntegrity
	}
	if output.Len() > wrappedKeyLimit {
		return Timelock{}, ErrLimit
	}
	return Timelock{Version: 1, ChainHash: QuicknetChainHash, PublicKey: QuicknetPublicKey, Scheme: quicknetScheme, Genesis: quicknetGenesis, Period: quicknetPeriod, Round: round, RequestedAt: requested, EffectiveAt: effective, WrappedKey: base64.RawURLEncoding.EncodeToString(output.Bytes())}, nil
}

func (t Timelock) validate() ([]byte, error) {
	round, effective, err := SelectRound(t.RequestedAt)
	if err != nil || t.Version != 1 || t.ChainHash != QuicknetChainHash || t.PublicKey != QuicknetPublicKey || t.Scheme != quicknetScheme || t.Genesis != quicknetGenesis || t.Period != quicknetPeriod || t.Round != round || !t.EffectiveAt.Equal(effective) {
		return nil, ErrIntegrity
	}
	if len(t.WrappedKey) > base64.RawURLEncoding.EncodedLen(wrappedKeyLimit) {
		return nil, ErrIntegrity
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(t.WrappedKey)
	if err != nil || base64.RawURLEncoding.EncodeToString(data) != t.WrappedKey {
		return nil, ErrIntegrity
	}
	prefix := fmt.Sprintf("age-encryption.org/v1\n-> tlock %d %s\n", round, QuicknetChainHash)
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return nil, ErrIntegrity
	}
	return data, nil
}

// OpenKey authenticates the complete key wrapper after local beacon verification.
func (t Timelock) OpenKey(ctx context.Context, source BeaconSource, now time.Time) ([32]byte, error) {
	data, err := t.validate()
	if err != nil {
		return [32]byte{}, err
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	if now.Before(t.EffectiveAt) {
		return [32]byte{}, ErrTooEarly
	}
	if source == nil {
		return [32]byte{}, ErrUnavailable
	}
	signature, err := source.Fetch(ctx, t.Round)
	if err != nil {
		if errors.Is(err, ErrIntegrity) {
			return [32]byte{}, ErrIntegrity
		}
		return [32]byte{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	network := newPinnedNetwork(t.Round)
	if len(signature) != 48 || network.scheme.VerifyBeacon(&chain.Beacon{Round: t.Round, Signature: signature}, network.key) != nil {
		return [32]byte{}, ErrIntegrity
	}
	network.signature = signature
	var output keyBuffer
	if err := tlock.New(network).Strict().Decrypt(&output, bytes.NewReader(data)); err != nil || output.used != 32 {
		return [32]byte{}, ErrIntegrity
	}
	return output.key, nil
}

type keyBuffer struct {
	key  [32]byte
	used int
}

func (b *keyBuffer) Write(data []byte) (int, error) {
	if len(data) > len(b.key)-b.used {
		return 0, ErrIntegrity
	}
	copy(b.key[b.used:], data)
	b.used += len(data)
	return len(data), nil
}

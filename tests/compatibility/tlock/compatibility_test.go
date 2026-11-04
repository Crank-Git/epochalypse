package compatibility

import (
	"bytes"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	chain "github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/kyber"
	"github.com/drand/tlock"
)

const chainHash = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
const publicKey = "83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a"

//go:embed testdata/*
var fixtures embed.FS

type fixtureNetwork struct {
	key         kyber.Point
	scheme      crypto.Scheme
	signature   []byte
	hash        string
	unavailable bool
	switches    int
}

func (n *fixtureNetwork) ChainHash() string        { return n.hash }
func (n *fixtureNetwork) Current(time.Time) uint64 { return 1000 }
func (n *fixtureNetwork) PublicKey() kyber.Point   { return n.key }
func (n *fixtureNetwork) Scheme() crypto.Scheme    { return n.scheme }
func (n *fixtureNetwork) SwitchChainHash(string) error {
	n.switches++
	return errors.New("chain switch forbidden")
}
func (n *fixtureNetwork) Signature(round uint64) ([]byte, error) {
	if n.unavailable || round != 1000 {
		return nil, errors.New("beacon unavailable")
	}
	return n.signature, nil
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := fixtures.ReadFile("testdata/" + path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func network(t *testing.T) *fixtureNetwork {
	t.Helper()
	var info struct {
		PublicKey string `json:"public_key"`
		Period    int64  `json:"period"`
		Genesis   int64  `json:"genesis_time"`
		Hash      string `json:"hash"`
		Scheme    string `json:"schemeID"`
	}
	if err := json.Unmarshal(readFixture(t, "quicknet-info.json"), &info); err != nil {
		t.Fatal(err)
	}
	if info.PublicKey != publicKey || info.Hash != chainHash || info.Period != 3 || info.Genesis != 1692803367 || info.Scheme != "bls-unchained-g1-rfc9380" {
		t.Fatal("quicknet identity differs from the official pin")
	}
	scheme, err := crypto.SchemeFromName(info.Scheme)
	if err != nil {
		t.Fatal(err)
	}
	key := scheme.KeyGroup.Point()
	keyBytes, err := hex.DecodeString(info.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.UnmarshalBinary(keyBytes); err != nil {
		t.Fatal(err)
	}
	var beacon struct {
		Round     uint64
		Signature string
	}
	if err := json.Unmarshal(readFixture(t, "quicknet-1000.json"), &beacon); err != nil {
		t.Fatal(err)
	}
	signature, err := hex.DecodeString(beacon.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if beacon.Round != 1000 {
		t.Fatal("fixture round differs")
	}
	return &fixtureNetwork{key: key, scheme: *scheme, signature: signature, hash: chainHash}
}

func TestVerifiesBeaconLocally(t *testing.T) {
	n := network(t)
	if err := n.scheme.VerifyBeacon(&chain.Beacon{Round: 1000, Signature: n.signature}, n.key); err != nil {
		t.Fatal(err)
	}
	if err := n.scheme.VerifyBeacon(&chain.Beacon{Round: 1001, Signature: n.signature}, n.key); err == nil {
		t.Fatal("altered round passes verification")
	}
	n.signature[0] ^= 1
	if err := n.scheme.VerifyBeacon(&chain.Beacon{Round: 1000, Signature: n.signature}, n.key); err == nil {
		t.Fatal("altered signature passes verification")
	}
}

func TestEncryptsAndDecryptsSyntheticContent(t *testing.T) {
	n := network(t)
	lock := tlock.New(n).Strict()
	plain := []byte("epochalypse synthetic capsule fixture\n")
	var ciphertext, recovered bytes.Buffer
	if err := lock.Encrypt(&ciphertext, bytes.NewReader(plain), 1000); err != nil {
		t.Fatal(err)
	}
	if err := lock.Decrypt(&recovered, bytes.NewReader(ciphertext.Bytes())); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, recovered.Bytes()) {
		t.Fatal("plaintext differs")
	}
}

func TestDecryptsVersionedCiphertextFixture(t *testing.T) {
	var recovered bytes.Buffer
	if err := tlock.New(network(t)).Strict().Decrypt(&recovered, bytes.NewReader(readFixture(t, "synthetic.age"))); err != nil {
		t.Fatal(err)
	}
	if recovered.String() != "epochalypse synthetic capsule fixture\n" {
		t.Fatal("fixture plaintext differs")
	}
}

func TestRejectsInvalidRecoveryInputs(t *testing.T) {
	n := network(t)
	var encrypted bytes.Buffer
	if err := tlock.New(n).Strict().Encrypt(&encrypted, bytes.NewReader([]byte("synthetic content")), 1000); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"wrong chain", "altered signature", "missing beacon", "corrupt ciphertext", "unsupported format"} {
		t.Run(name, func(t *testing.T) {
			n := network(t)
			data := bytes.Clone(encrypted.Bytes())
			switch name {
			case "wrong chain":
				n.hash = "different-chain"
			case "altered signature":
				n.signature[0] ^= 1
			case "missing beacon":
				n.unavailable = true
			case "corrupt ciphertext":
				data[len(data)-1] ^= 1
			case "unsupported format":
				data = bytes.Replace(data, []byte("age-encryption.org/v1"), []byte("age-encryption.org/v2"), 1)
			}
			var output bytes.Buffer
			err := tlock.New(n).Strict().Decrypt(&output, bytes.NewReader(data))
			if err == nil {
				t.Fatal("invalid recovery input succeeds")
			}
			if name == "wrong chain" && (!errors.Is(err, tlock.ErrWrongChainhash) || n.switches != 0) {
				t.Fatalf("strict chain check: switches=%d, error=%v", n.switches, err)
			}
			if name == "missing beacon" && !errors.Is(err, tlock.ErrTooEarly) {
				t.Fatalf("availability error = %v", err)
			}
			if output.Len() != 0 {
				t.Fatal("invalid fixture exposes plaintext")
			}
		})
	}
}

func TestSelectsQuicknetRound(t *testing.T) {
	round, effective, err := roundAtOrAfter(1692806365, 1692803367, 3)
	if err != nil || round != 1001 || effective != 1692806367 {
		t.Fatalf("got %d, %d, %v", round, effective, err)
	}
}

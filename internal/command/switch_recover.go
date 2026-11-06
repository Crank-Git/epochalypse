package command

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Crank-Git/epochalypse/internal/switchclient"
)

func runSwitchRecover(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("switch recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bundlePath := flags.String("bundle", "", "Public recovery bundle file")
	keyPath := flags.String("owner-public-key", "", "Base64 Ed25519 public key file")
	pinsPath := flags.String("pins", "", "JSON map of HTTPS endpoints to TLS SPKI SHA256 pins")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *bundlePath == "" || *keyPath == "" || *pinsPath == "" {
		_, _ = fmt.Fprintln(stderr, "provide --bundle, --owner-public-key, and --pins")
		return 2
	}
	plaintext, err := recoverSwitchFiles(*bundlePath, *keyPath, *pinsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer clear(plaintext)
	if _, err := stdout.Write(plaintext); err != nil {
		return 1
	}
	return 0
}

func recoverSwitchFiles(bundlePath, keyPath, pinsPath string) ([]byte, error) {
	keyData, err := readBoundedFile(keyPath, 4096)
	if err != nil {
		return nil, fmt.Errorf("read owner public key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid owner public key file")
	}
	data, err := readBoundedFile(bundlePath, 16<<20)
	if err != nil {
		return nil, fmt.Errorf("read recovery bundle: %w", err)
	}
	bundle, err := switchclient.ReadBundle(bytes.NewReader(data), ed25519.PublicKey(key))
	if err != nil {
		return nil, err
	}
	pins, err := readEndpointPins(pinsPath)
	if err != nil {
		return nil, err
	}
	return switchclient.Recover(context.Background(), bundle, ed25519.PublicKey(key), &switchclient.HTTPSReplica{Pins: pins})
}

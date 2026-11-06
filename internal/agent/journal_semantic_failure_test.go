package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRejectsChecksummedInvalidAuthority(t *testing.T) {
	for name, change := range map[string]func(*diskRecord){
		"version":          func(r *diskRecord) { r.Version++ },
		"generation":       func(r *diskRecord) { r.Generation++ },
		"state generation": func(r *diskRecord) { r.State.Generation++ },
		"chain":            func(r *diskRecord) { r.PreviousSHA256 = "unrelated head" },
		"phase":            func(r *diskRecord) { r.State.Phase = 4 },
		"disclosure latch": func(r *diskRecord) { r.State.PossibleDisclosure = false },
		"clock latch":      func(r *diskRecord) { r.State.UnsafeClock = false },
	} {
		t.Run(name, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			first := authorityTestState()
			first.PossibleDisclosure, first.UnsafeClock = true, true
			record := diskRecord{Version: authorityFormatVersion, Generation: 1, PreviousSHA256: hex.EncodeToString(make([]byte, sha256.Size)), State: first}
			data, hash := encodeJournalTestRecord(t, record)
			record.Generation, record.State.Generation = 2, 2
			record.PreviousSHA256 = hex.EncodeToString(hash[:])
			change(&record)
			second, _ := encodeJournalTestRecord(t, record)
			assertInvalidJournalRetained(t, config, append(data, second...))
		})
	}
}

func TestOpenRejectsChecksummedMalformedAuthority(t *testing.T) {
	for _, payload := range []string{`{`, `{"unknown":true}`, `{} {}`} {
		t.Run(payload, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(payload))
			data := append(frameHeader(len(payload)), []byte(payload)...)
			data = append(data, hash[:]...)
			data = append(data, authorityCommitMarker...)
			assertInvalidJournalRetained(t, config, data)
		})
	}
}

func encodeJournalTestRecord(t *testing.T, record diskRecord) ([]byte, [sha256.Size]byte) {
	t.Helper()
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	data := append(frameHeader(len(payload)), payload...)
	data = append(data, hash[:]...)
	return append(data, authorityCommitMarker...), hash
}

func assertInvalidJournalRetained(t *testing.T, config Config, data []byte) {
	t.Helper()
	path := filepath.Join(config.Directory, authorityJournalName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(config)
	if journal != nil {
		_ = journal.Close()
		t.Fatal("invalid committed authority opens")
	}
	if !errors.Is(err, ErrQuarantined) {
		t.Fatalf("invalid committed authority error = %v", err)
	}
	after, err := os.ReadFile(path) // #nosec G304 -- The test controls the temporary authority path.
	if err != nil || !bytes.Equal(after, data) {
		t.Fatalf("rejected open changes authority: %v", err)
	}
}

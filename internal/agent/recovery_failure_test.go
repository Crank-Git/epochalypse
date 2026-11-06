package agent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
)

func TestServiceRejectsDamagedAcceptedMaterialBeforeClock(t *testing.T) {
	for _, armed := range []bool{false, true} {
		for name, damage := range map[string]func(*State){
			"request":           func(s *State) { s.AcceptedRequest = []byte("truncated") },
			"signature":         func(s *State) { s.AcceptedRequest[len(s.AcceptedRequest)-1] ^= 1 },
			"prepare":           func(s *State) { s.PrepareRequest = []byte("truncated") },
			"share":             func(s *State) { s.EncryptedShare = []byte("unrelated share") },
			"receipt":           func(s *State) { s.Receipt = []byte("truncated") },
			"receipt signature": func(s *State) { s.Receipt[len(s.Receipt)-1] ^= 1 },
			"manifest":          func(s *State) { s.Manifest = []byte("unexpected") },
			"release":           func(s *State) { s.ReleaseEnvelope = []byte("unexpected") },
			"enrollment":        func(s *State) { s.Enrollment = []byte("unrelated enrollment") },
		} {
			t.Run(name+map[bool]string{false: "/prepared", true: "/armed"}[armed], func(t *testing.T) {
				fixture := newTransitionFixture(t)
				transitions, err := NewTransitions(fixture.config(0))
				if err != nil {
					t.Fatal(err)
				}
				if armed {
					prepareAndArmFixture(t, fixture, transitions)
				} else if _, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), fixture.material.Ciphertext, nil); err != nil {
					t.Fatal(err)
				}
				state, err := fixture.journal.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				damage(&state)
				state, err = transitions.commit(state)
				if err != nil {
					t.Fatal(err)
				}
				if err := fixture.journal.Close(); err != nil {
					t.Fatal(err)
				}
				calls := 0
				config := ServiceConfig{Authority: fixture.authorityConfig, Transition: fixture.config(0), LabOnlySyntheticClock: true}
				config.Transition.ReadClock = func() (Reading, error) { calls++; return Reading{lower: 1000}, nil }
				service, err := OpenService(config)
				if service != nil {
					_ = service.Close()
				}
				if err == nil || calls != 0 {
					t.Fatalf("damaged authority reaches readiness: error=%v clock=%d", err, calls)
				}
				journal, err := Open(fixture.authorityConfig)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = journal.Close() }()
				reopened, err := journal.Snapshot()
				if err != nil || reopened.Generation != state.Generation {
					t.Fatalf("failed readiness mutates authority: %v", err)
				}
			})
		}
	}
}

func TestAuthorityRejectsUnsafeVolumeFiles(t *testing.T) {
	for _, name := range []string{authorityMarkerName, authorityJournalName, authorityLockName} {
		for _, kind := range []string{"directory", "public", "symlink"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				config := authorityTestConfig(t)
				if err := Initialize(config); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(config.Directory, name)
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				switch kind {
				case "directory":
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				case "public":
					if err := os.WriteFile(path, []byte(config.ExpectedVolumeID+"\n"), 0o644); err != nil { // #nosec G306 -- The test requires a public authority file.
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Symlink(config.CheckpointDirectory, path); err != nil {
						t.Fatal(err)
					}
				}
				journal, err := Open(config)
				if journal != nil {
					_ = journal.Close()
				}
				if err == nil {
					t.Fatal("unsafe authority file opens")
				}
			})
		}
	}
}

func TestAuthorityRejectsCorruptCommittedRecords(t *testing.T) {
	for name, damage := range map[string]func([]byte) []byte{
		"length":         func(data []byte) []byte { copy(data[:8], "ffffffff"); return data },
		"payload":        func(data []byte) []byte { data[8] ^= 1; return data },
		"checksum":       func(data []byte) []byte { data[len(data)-len(authorityCommitMarker)-1] ^= 1; return data },
		"marker":         func(data []byte) []byte { data[len(data)-1] ^= 1; return data },
		"short checksum": func(data []byte) []byte { return data[:len(data)-len(authorityCommitMarker)-1] },
		"short payload":  func(data []byte) []byte { return data[:10] },
	} {
		t.Run(name, func(t *testing.T) {
			config := authorityTestConfig(t)
			if err := Initialize(config); err != nil {
				t.Fatal(err)
			}
			journal, err := Open(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := journal.Commit(authorityTestState()); err != nil {
				t.Fatal(err)
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(config.Directory, authorityJournalName)
			data, err := os.ReadFile(path) // #nosec G304 -- The test controls the temporary journal path.
			if err != nil {
				t.Fatal(err)
			}
			data = damage(data)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			journal, err = Open(config)
			if journal != nil {
				_ = journal.Close()
			}
			if !errors.Is(err, ErrQuarantined) {
				t.Fatalf("corrupt journal error = %v", err)
			}
			retained, err := os.ReadFile(path) // #nosec G304 -- The test controls the temporary journal path.
			if err != nil || !bytes.Equal(data, retained) {
				t.Fatalf("failed open modifies corrupt evidence: %v", err)
			}
		})
	}
}

func TestTransitionsRefuseClosedAuthority(t *testing.T) {
	fixture := newTransitionFixture(t)
	transitions, err := NewTransitions(fixture.config(0))
	if err != nil {
		t.Fatal(err)
	}
	prepareAndArmFixture(t, fixture, transitions)
	state, err := fixture.journal.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	arm, err := switchwire.ReadEnvelope(bytes.NewReader(state.AcceptedRequest))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := transitions.Arm(arm); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("arm on closed authority: %v", err)
	}
	if _, err := transitions.Prepare(fixture.prepareRequest(t, fixture.material.Ciphertext), fixture.material.Ciphertext, nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("prepare on closed authority: %v", err)
	}
	if _, err := fixture.journal.Commit(State{Phase: switchwire.PhaseArmed}); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("commit on closed authority: %v", err)
	}
}

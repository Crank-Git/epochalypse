package switchclient

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Crank-Git/epochalypse/internal/switchwire"
	"golang.org/x/sys/unix"
)

func lockRenewal(path string) (*os.File, error) {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("client journal requires an owner-only directory")
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if err := errors.Join(syncDirectory(path, parent), parent.Close()); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(path, "lock"), os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- The owner chooses the private journal directory.
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock client journal: %w", err)
	}
	return f, nil
}

func unlockRenewal(f *os.File) { _ = f.Close() }

func persistEnvelope(path string, envelope switchwire.Envelope) error {
	data, err := switchwire.MarshalEnvelope(envelope)
	if err != nil {
		return err
	}
	return persistExact(path, data)
}

func readJournalEnvelope(path string) (switchwire.Envelope, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return switchwire.Envelope{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return switchwire.Envelope{}, errors.New("client journal requires owner-only regular files")
	}
	f, err := os.Open(path) // #nosec G304 -- The owner chooses the private journal directory.
	if err != nil {
		return switchwire.Envelope{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, switchwire.MaxEnvelope+5))
	if err != nil {
		return switchwire.Envelope{}, err
	}
	return switchwire.ReadEnvelope(bytes.NewReader(data))
}

func readRenewal(path string, p Plan, e switchwire.AdmittedEnrollment) (Renewal, switchwire.Frame, error) {
	initial, err := switchwire.VerifyOwnerRequest(p.Prepare, e)
	if err != nil || initial.Operation != switchwire.OperationPrepare {
		return Renewal{}, switchwire.Frame{}, errors.New("invalid original owner intent")
	}
	var result Renewal
	prior := initial.State
	for sequence := uint64(1); ; sequence++ {
		request, err := readJournalEnvelope(requestPath(path, sequence))
		if errors.Is(err, os.ErrNotExist) && sequence > 1 {
			f, decodeErr := switchwire.DecodeFrame(result.Request.Body)
			return result, f, decodeErr
		}
		if err != nil {
			return result, switchwire.Frame{}, fmt.Errorf("read client intent: %w", err)
		}
		frame, err := validateJournalRequest(request, e, prior, sequence)
		if err != nil {
			return result, frame, err
		}
		if sequence > 1 && !result.Confirmed {
			return result, frame, errors.New("client journal extends unresolved intent")
		}
		receipts, err := readRenewalReceipts(path, frame.State, e)
		if err != nil {
			return result, frame, err
		}
		result = renewalView(result.B0, frame, request, receipts, e)
		if sequence == 1 && !result.Confirmed {
			return result, frame, errors.New("client journal lacks three initial arm receipts")
		}
		prior = frame.State
		if sequence == ^uint64(0) {
			return result, frame, errors.New("client journal exhausts the sequence space")
		}
	}
}

func validateJournalRequest(request switchwire.Envelope, e switchwire.AdmittedEnrollment, prior switchwire.State, sequence uint64) (switchwire.Frame, error) {
	f, err := switchwire.VerifyOwnerRequest(request, e)
	if err != nil {
		return f, err
	}
	expected := switchwire.OperationCheckin
	if sequence == 1 {
		expected = switchwire.OperationArm
	}
	base := f.State
	base.Sequence, base.Deadline = prior.Sequence, prior.Deadline
	if f.Operation != expected || f.Phase != switchwire.PhaseArmed || f.Released || f.State.Sequence != sequence || base != prior || f.State.Deadline < prior.Deadline {
		return f, errors.New("client journal contains conflicting signed intent")
	}
	if sequence == 1 && f.State != prior {
		return f, errors.New("client journal changes initial authority")
	}
	return f, nil
}

func readRenewalReceipts(path string, state switchwire.State, e switchwire.AdmittedEnrollment) (map[string]switchwire.Envelope, error) {
	receipts := map[string]switchwire.Envelope{}
	for _, a := range e.Agents {
		receipt, err := readJournalEnvelope(receiptPath(path, state.Sequence, a.ID))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !validRenewalReceipt(receipt, state, e, a.ID) {
			return nil, errors.New("client journal contains invalid receipt evidence")
		}
		receipts[a.ID] = receipt
	}
	return receipts, nil
}

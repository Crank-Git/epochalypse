package agent

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	authorityMarkerName     = "authority.meta"
	authorityJournalName    = "authority.journal"
	authorityLockName       = "authority.lock"
	authorityCheckpointName = "checkpoint.dat"
	authorityFormatVersion  = 1
	authorityCommitMarker   = "AUTHCOM1"
	authorityDefaultMax     = 32 << 20
	authorityMaxAllowed     = 64 << 20
)

var (
	ErrIrreversibleState = errors.New("agent authority cannot clear an irreversible state")
	ErrQuarantined       = errors.New("agent authority is quarantined")
	ErrGeneration        = errors.New("agent authority generation is invalid")
	ErrVolumeIdentity    = errors.New("agent authority volume identity does not match")
	ErrRecordLimit       = errors.New("agent authority record exceeds its configured limit")
	ErrLocked            = errors.New("agent authority is already open")
)

type Config struct {
	Directory           string
	CheckpointDirectory string
	ExpectedVolumeID    string
	MaxRecordBytes      uint32
}

type State struct {
	Generation         uint64
	Phase              uint8
	PossibleDisclosure bool
	UnsafeClock        bool
	AcceptedRequest    []byte
	PrepareRequest     []byte
	Enrollment         []byte
	Ciphertext         []byte
	Manifest           []byte
	EncryptedShare     []byte
	Receipt            []byte
	ReleaseEnvelope    []byte
}

type Journal struct {
	mu          sync.Mutex
	config      Config
	journal     *os.File
	lock        *os.File
	state       State
	headHash    [sha256.Size]byte
	closed      bool
	quarantined bool
	afterStep   func(string)
}

type diskRecord struct {
	Version        uint8  `json:"version"`
	Generation     uint64 `json:"generation"`
	PreviousSHA256 string `json:"previous_sha256"`
	State          State  `json:"state"`
}

type checkpoint struct {
	Version    uint8  `json:"version"`
	Generation uint64 `json:"generation"`
	HeadSHA256 string `json:"head_sha256"`
	State      State  `json:"state"`
	Checksum   string `json:"checksum"`
}

// Initialize creates the journal authority on an empty, pinned volume.
func Initialize(config Config) error {
	config, err := normalizeAuthorityConfig(config)
	if err != nil {
		return err
	}
	if err := validateAuthorityPaths(config); err != nil {
		return err
	}
	directoryInfo, err := os.Lstat(config.Directory)
	if err != nil {
		return fmt.Errorf("inspect authority volume: %w", err)
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("authority volume must be a real directory")
	}
	if !ownerOnlyDirectory(directoryInfo) {
		return errors.New("authority volume must use owner-only permissions")
	}
	entries, err := os.ReadDir(config.Directory)
	if err != nil {
		return fmt.Errorf("read authority volume: %w", err)
	}
	if len(entries) != 0 {
		return errors.New("authority volume must be empty before initialization")
	}

	root, err := os.OpenRoot(config.Directory)
	if err != nil {
		return fmt.Errorf("open authority volume: %w", err)
	}
	defer func() { _ = root.Close() }()
	marker := []byte(config.ExpectedVolumeID + "\n")
	if err := writeNewSyncedFile(root, authorityMarkerName, marker); err != nil {
		return fmt.Errorf("write authority volume identity: %w", err)
	}
	journal, err := root.OpenFile(authorityJournalName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create authority journal: %w", err)
	}
	if err := journal.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync authority journal: %w", err), journal.Close())
	}
	if err := journal.Close(); err != nil {
		return fmt.Errorf("close authority journal: %w", err)
	}
	if err := syncRoot(root); err != nil {
		return fmt.Errorf("sync authority volume: %w", err)
	}
	return nil
}

// Open validates and locks an existing authority volume.
func Open(config Config) (*Journal, error) {
	config, err := normalizeAuthorityConfig(config)
	if err != nil {
		return nil, err
	}
	if err := validateAuthorityPaths(config); err != nil {
		return nil, err
	}
	if err := verifyVolumeMarker(config); err != nil {
		return nil, err
	}
	lock, journal, err := openLockedAuthorityFiles(config)
	if err != nil {
		return nil, err
	}

	instance := &Journal{config: config, journal: journal, lock: lock}
	if err := instance.replay(); err != nil {
		return nil, errors.Join(err, instance.Close())
	}
	if err := instance.validateOrRepairCheckpoint(); err != nil {
		return nil, errors.Join(err, instance.Close())
	}
	return instance, nil
}

func verifyVolumeMarker(config Config) error {
	if err := checkPrivateDirectory(config.Directory); err != nil {
		return fmt.Errorf("validate authority volume: %w", err)
	}
	root, err := os.OpenRoot(config.Directory)
	if err != nil {
		return fmt.Errorf("open authority volume: %w", err)
	}
	defer func() { _ = root.Close() }()
	markerInfo, err := root.Lstat(authorityMarkerName)
	if err != nil {
		return fmt.Errorf("read authority volume identity: %w", err)
	}
	if !ownerOnlyFile(markerInfo) {
		return errors.New("authority volume identity must be a private regular file")
	}
	marker, err := root.Open(authorityMarkerName)
	if err != nil {
		return fmt.Errorf("read authority volume identity: %w", err)
	}
	defer func() { _ = marker.Close() }()
	info, err := marker.Stat()
	if err != nil {
		return fmt.Errorf("inspect authority volume identity: %w", err)
	}
	if !ownerOnlyFile(info) {
		return errors.New("authority volume identity must be a private regular file")
	}
	data, err := readBoundedFile(root, authorityMarkerName, 512)
	if err != nil {
		return fmt.Errorf("read authority volume identity: %w", err)
	}
	if string(data) != config.ExpectedVolumeID+"\n" {
		return ErrVolumeIdentity
	}
	return nil
}

func openLockedAuthorityFiles(config Config) (*os.File, *os.File, error) {
	root, err := os.OpenRoot(config.Directory)
	if err != nil {
		return nil, nil, fmt.Errorf("open authority volume: %w", err)
	}
	defer func() { _ = root.Close() }()
	lock, err := openAuthorityLock(root)
	if err != nil {
		return nil, nil, err
	}
	journal, err := openAuthorityJournal(root)
	if err != nil {
		if closeErr := releaseAuthorityLock(lock); closeErr != nil {
			return nil, nil, errors.Join(err, closeErr)
		}
		return nil, nil, err
	}
	return lock, journal, nil
}

func openAuthorityLock(root *os.Root) (*os.File, error) {
	if info, err := root.Lstat(authorityLockName); err == nil {
		if !ownerOnlyFile(info) {
			return nil, errors.New("authority lock must be a private regular file")
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect authority lock: %w", err)
	}
	lock, err := root.OpenFile(authorityLockName, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open authority lock: %w", err)
	}
	if err := lock.Chmod(0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("set authority lock permissions: %w", err), lock.Close())
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		closeErr := lock.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errors.Join(fmt.Errorf("%w: %w", ErrLocked, err), closeErr)
		}
		return nil, errors.Join(fmt.Errorf("lock authority volume: %w", err), closeErr)
	}
	return lock, nil
}

func openAuthorityJournal(root *os.Root) (*os.File, error) {
	info, err := root.Lstat(authorityJournalName)
	if err != nil {
		return nil, fmt.Errorf("inspect authority journal: %w", err)
	}
	if !ownerOnlyFile(info) {
		return nil, errors.New("authority journal must be a private regular file")
	}
	journal, err := root.OpenFile(authorityJournalName, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open authority journal: %w", err)
	}
	info, err = journal.Stat()
	if err == nil && ownerOnlyFile(info) {
		return journal, nil
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("inspect authority journal: %w", err), journal.Close())
	}
	return nil, errors.Join(errors.New("authority journal must be a private regular file"), journal.Close())
}

// Snapshot returns a copy of the complete accepted state.
func (j *Journal) Snapshot() (State, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return State{}, os.ErrClosed
	}
	if j.quarantined {
		return State{}, ErrQuarantined
	}
	return cloneAuthorityState(j.state), nil
}

// Commit appends one complete state before it updates the disposable checkpoint.
func (j *Journal) Commit(next State) (State, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return State{}, os.ErrClosed
	}
	if j.quarantined {
		return State{}, ErrQuarantined
	}
	if err := j.validateSuccessor(next); err != nil {
		return State{}, err
	}
	stateCopy := cloneAuthorityState(next)
	record := diskRecord{
		Version:        authorityFormatVersion,
		Generation:     stateCopy.Generation,
		PreviousSHA256: hex.EncodeToString(j.headHash[:]),
		State:          stateCopy,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return State{}, fmt.Errorf("encode authority record: %w", err)
	}
	if len(payload) > int(j.config.MaxRecordBytes) {
		return State{}, ErrRecordLimit
	}
	checksum := sha256.Sum256(payload)
	if err := j.appendRecord(payload, checksum); err != nil {
		return State{}, err
	}
	hash := sha256.Sum256(payload)
	j.state = stateCopy
	j.headHash = hash
	j.step("after-journal-sync")
	if err := j.writeCheckpoint(); err != nil {
		return State{}, fmt.Errorf("write authority checkpoint after journal commit: %w", err)
	}
	j.quarantined = false
	j.step("after-checkpoint-sync")
	return cloneAuthorityState(j.state), nil
}

func (j *Journal) validateSuccessor(next State) error {
	if j.state.Generation == ^uint64(0) || next.Generation != j.state.Generation+1 {
		return ErrGeneration
	}
	if next.Phase > 3 {
		return errors.New("authority phase is unsupported")
	}
	if (j.state.PossibleDisclosure && !next.PossibleDisclosure) || (j.state.UnsafeClock && !next.UnsafeClock) {
		return ErrIrreversibleState
	}
	return nil
}

func (j *Journal) appendRecord(payload []byte, checksum [sha256.Size]byte) error {
	if _, err := j.journal.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek authority journal: %w", err)
	}
	j.quarantined = true
	if err := writeAll(j.journal, frameHeader(len(payload))); err != nil {
		return fmt.Errorf("append authority record length: %w", err)
	}
	if err := writeAll(j.journal, payload); err != nil {
		return fmt.Errorf("append authority record: %w", err)
	}
	if err := writeAll(j.journal, checksum[:]); err != nil {
		return fmt.Errorf("append authority checksum: %w", err)
	}
	if err := j.journal.Sync(); err != nil {
		return fmt.Errorf("sync authority record body: %w", err)
	}
	j.step("before-commit-marker")
	if err := writeAll(j.journal, []byte(authorityCommitMarker)); err != nil {
		return fmt.Errorf("append authority commit marker: %w", err)
	}
	if err := j.journal.Sync(); err != nil {
		return fmt.Errorf("sync authority commit marker: %w", err)
	}
	return nil
}

// Close releases the exclusive process lock and closes the authority files.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	var result error
	if err := j.journal.Close(); err != nil {
		result = fmt.Errorf("close authority journal: %w", err)
	}
	if err := releaseAuthorityLock(j.lock); err != nil && result == nil {
		result = err
	}
	return result
}

func (j *Journal) replay() error {
	if _, err := j.journal.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek authority journal: %w", err)
	}
	reader := io.NewSectionReader(j.journal, 0, 1<<63-1)
	state := State{}
	var previous [sha256.Size]byte
	for {
		payload, checksum, err := readAuthorityRecord(reader, j.config.MaxRecordBytes)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		state, previous, err = applyAuthorityRecord(payload, checksum, state, previous)
		if err != nil {
			return err
		}
	}
	if _, err := j.journal.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("seek authority journal end: %w", err)
	}
	j.state = state
	j.headHash = previous
	return nil
}

func readAuthorityRecord(reader io.Reader, maximum uint32) ([]byte, [sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	var lengthBytes [8]byte
	count, err := io.ReadFull(reader, lengthBytes[:])
	if err == io.EOF && count == 0 {
		return nil, empty, io.EOF
	}
	if err != nil {
		return nil, empty, fmt.Errorf("%w: incomplete authority record length", ErrQuarantined)
	}
	length, err := strconv.ParseUint(string(lengthBytes[:]), 16, 64)
	if err != nil || length == 0 || length > uint64(maximum) {
		return nil, empty, fmt.Errorf("%w: invalid authority record length", ErrQuarantined)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, empty, fmt.Errorf("%w: incomplete authority record", ErrQuarantined)
	}
	var checksum [sha256.Size]byte
	if _, err := io.ReadFull(reader, checksum[:]); err != nil {
		return nil, empty, fmt.Errorf("%w: incomplete authority checksum", ErrQuarantined)
	}
	var marker [len(authorityCommitMarker)]byte
	if _, err := io.ReadFull(reader, marker[:]); err != nil || string(marker[:]) != authorityCommitMarker {
		return nil, empty, fmt.Errorf("%w: authority record has no commit marker", ErrQuarantined)
	}
	actual := sha256.Sum256(payload)
	if !bytes.Equal(checksum[:], actual[:]) {
		return nil, empty, fmt.Errorf("%w: authority record checksum does not match", ErrQuarantined)
	}
	return payload, checksum, nil
}

func applyAuthorityRecord(payload []byte, checksum [sha256.Size]byte, state State, previous [sha256.Size]byte) (State, [sha256.Size]byte, error) {
	record, err := decodeAuthorityRecord(payload)
	if err != nil {
		return State{}, previous, fmt.Errorf("%w: invalid authority record: %w", ErrQuarantined, err)
	}
	if record.Version != authorityFormatVersion || record.Generation != state.Generation+1 || record.Generation == 0 {
		return State{}, previous, fmt.Errorf("%w: authority record generation is not consecutive", ErrQuarantined)
	}
	if record.PreviousSHA256 != hex.EncodeToString(previous[:]) || record.State.Generation != record.Generation || record.State.Phase > 3 {
		return State{}, previous, fmt.Errorf("%w: authority record chain or state is invalid", ErrQuarantined)
	}
	if (state.PossibleDisclosure && !record.State.PossibleDisclosure) || (state.UnsafeClock && !record.State.UnsafeClock) {
		return State{}, previous, fmt.Errorf("%w: authority record clears an irreversible latch", ErrQuarantined)
	}
	return cloneAuthorityState(record.State), checksum, nil
}

func (j *Journal) validateOrRepairCheckpoint() error {
	if err := os.MkdirAll(j.config.CheckpointDirectory, 0o700); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}
	if err := checkPrivateDirectory(j.config.CheckpointDirectory); err != nil {
		return fmt.Errorf("validate checkpoint directory: %w", err)
	}
	root, err := os.OpenRoot(j.config.CheckpointDirectory)
	if err != nil {
		return fmt.Errorf("open checkpoint directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	saved, err := readAuthorityCheckpoint(root, j.config.MaxRecordBytes)
	if os.IsNotExist(err) {
		return j.writeCheckpoint()
	}
	if err != nil {
		return err
	}
	return j.reconcileCheckpoint(saved)
}

func readAuthorityCheckpoint(root *os.Root, maximum uint32) (checkpoint, error) {
	info, err := root.Lstat(authorityCheckpointName)
	if err != nil {
		return checkpoint{}, err
	}
	if !ownerOnlyFile(info) {
		return checkpoint{}, fmt.Errorf("%w: checkpoint is not a private regular file", ErrQuarantined)
	}
	data, err := readBoundedFile(root, authorityCheckpointName, maximum)
	if err != nil {
		return checkpoint{}, fmt.Errorf("%w: read checkpoint: %w", ErrQuarantined, err)
	}
	var saved checkpoint
	if err := decodeStrictJSON(data, &saved); err != nil {
		return checkpoint{}, fmt.Errorf("%w: decode checkpoint: %w", ErrQuarantined, err)
	}
	checksum, err := checkpointChecksum(saved)
	if err != nil || saved.Checksum != checksum {
		return checkpoint{}, fmt.Errorf("%w: checkpoint checksum does not match", ErrQuarantined)
	}
	return saved, nil
}

func (j *Journal) reconcileCheckpoint(saved checkpoint) error {
	if saved.Version != authorityFormatVersion || saved.Generation != j.state.Generation || saved.HeadSHA256 != hex.EncodeToString(j.headHash[:]) || !equalAuthorityState(saved.State, j.state) {
		if saved.Version != authorityFormatVersion || saved.Generation > j.state.Generation || !j.matchesJournalPrefix(saved) {
			return fmt.Errorf("%w: checkpoint does not match an intact journal prefix", ErrQuarantined)
		}
		return j.writeCheckpoint()
	}
	return nil
}

func (j *Journal) matchesJournalPrefix(saved checkpoint) bool {
	if saved.Generation == 0 {
		var emptyHash [sha256.Size]byte
		return saved.HeadSHA256 == hex.EncodeToString(emptyHash[:]) && equalAuthorityState(saved.State, State{})
	}
	reader := io.NewSectionReader(j.journal, 0, 1<<63-1)
	state := State{}
	var previous [sha256.Size]byte
	for state.Generation < saved.Generation {
		payload, checksum, err := readAuthorityRecord(reader, j.config.MaxRecordBytes)
		if err != nil {
			return false
		}
		state, previous, err = applyAuthorityRecord(payload, checksum, state, previous)
		if err != nil {
			return false
		}
	}
	return state.Generation == saved.Generation && saved.HeadSHA256 == hex.EncodeToString(previous[:]) && equalAuthorityState(saved.State, state)
}

func (j *Journal) writeCheckpoint() error {
	saved := checkpoint{
		Version:    authorityFormatVersion,
		Generation: j.state.Generation,
		HeadSHA256: hex.EncodeToString(j.headHash[:]),
		State:      cloneAuthorityState(j.state),
	}
	checksum, err := checkpointChecksum(saved)
	if err != nil {
		return fmt.Errorf("encode checkpoint checksum: %w", err)
	}
	saved.Checksum = checksum
	data, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	if len(data) > int(j.config.MaxRecordBytes) {
		return ErrRecordLimit
	}
	if err := os.MkdirAll(j.config.CheckpointDirectory, 0o700); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}
	root, err := os.OpenRoot(j.config.CheckpointDirectory)
	if err != nil {
		return fmt.Errorf("open checkpoint directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	var randomName [16]byte
	if _, err := rand.Read(randomName[:]); err != nil {
		return fmt.Errorf("create checkpoint name: %w", err)
	}
	temporaryName := ".checkpoint-" + hex.EncodeToString(randomName[:])
	temporary, err := root.OpenFile(temporaryName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary checkpoint: %w", err)
	}
	if err := temporary.Chmod(0o600); err != nil {
		return errors.Join(fmt.Errorf("set temporary checkpoint permissions: %w", err), temporary.Close(), root.Remove(temporaryName))
	}
	if err := writeAll(temporary, data); err != nil {
		return errors.Join(fmt.Errorf("write checkpoint: %w", err), temporary.Close(), root.Remove(temporaryName))
	}
	if err := temporary.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync checkpoint: %w", err), temporary.Close(), root.Remove(temporaryName))
	}
	if err := temporary.Close(); err != nil {
		return errors.Join(fmt.Errorf("close checkpoint: %w", err), root.Remove(temporaryName))
	}
	if err := root.Rename(temporaryName, authorityCheckpointName); err != nil {
		return errors.Join(fmt.Errorf("replace checkpoint: %w", err), root.Remove(temporaryName))
	}
	if err := syncRoot(root); err != nil {
		return fmt.Errorf("sync checkpoint directory: %w", err)
	}
	return nil
}

func (j *Journal) step(point string) {
	if j.afterStep != nil {
		j.afterStep(point)
	}
}

func normalizeAuthorityConfig(config Config) (Config, error) {
	if config.Directory == "" || config.CheckpointDirectory == "" {
		return Config{}, errors.New("authority and checkpoint directories are required")
	}
	if config.ExpectedVolumeID == "" || len(config.ExpectedVolumeID) > 256 || strings.ContainsAny(config.ExpectedVolumeID, "\r\n\x00") {
		return Config{}, errors.New("a bounded pinned volume identity is required")
	}
	var err error
	config.Directory, err = filepath.Abs(config.Directory)
	if err != nil {
		return Config{}, fmt.Errorf("resolve authority directory: %w", err)
	}
	config.CheckpointDirectory, err = filepath.Abs(config.CheckpointDirectory)
	if err != nil {
		return Config{}, fmt.Errorf("resolve checkpoint directory: %w", err)
	}
	if config.MaxRecordBytes == 0 {
		config.MaxRecordBytes = authorityDefaultMax
	}
	if config.MaxRecordBytes < 1024 || config.MaxRecordBytes > authorityMaxAllowed {
		return Config{}, errors.New("authority record limit is outside the supported range")
	}
	return config, nil
}

func validateAuthorityPaths(config Config) error {
	volumePath, err := filepath.EvalSymlinks(config.Directory)
	if err != nil {
		return fmt.Errorf("resolve authority volume: %w", err)
	}
	checkpointPath, err := resolveExistingOrParent(config.CheckpointDirectory)
	if err != nil {
		return fmt.Errorf("resolve checkpoint directory: %w", err)
	}
	relative, err := filepath.Rel(volumePath, checkpointPath)
	if err != nil {
		return fmt.Errorf("compare authority paths: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))) {
		return errors.New("checkpoint directory must remain outside the authority volume")
	}
	return nil
}

func resolveExistingOrParent(path string) (string, error) {
	current := path
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func checkPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !ownerOnlyDirectory(info) {
		return errors.New("path is not a real directory")
	}
	return nil
}

func ownerOnlyDirectory(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm()&0o700 == 0o700 && info.Mode().Perm()&0o077 == 0
}

func ownerOnlyFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0o600 == 0o600 && info.Mode().Perm()&0o077 == 0
}

func writeNewSyncedFile(root *os.Root, name string, content []byte) error {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := writeAll(file, content); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

func frameHeader(size int) []byte {
	return []byte(fmt.Sprintf("%08x", size))
}

func decodeAuthorityRecord(data []byte) (diskRecord, error) {
	var record diskRecord
	if err := decodeStrictJSON(data, &record); err != nil {
		return diskRecord{}, err
	}
	return record, nil
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("JSON has trailing data")
		}
		return err
	}
	return nil
}

func checkpointChecksum(saved checkpoint) (string, error) {
	saved.Checksum = ""
	data, err := json.Marshal(saved)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func equalAuthorityState(left, right State) bool {
	return left.Generation == right.Generation && left.Phase == right.Phase &&
		left.PossibleDisclosure == right.PossibleDisclosure && left.UnsafeClock == right.UnsafeClock &&
		bytes.Equal(left.AcceptedRequest, right.AcceptedRequest) && bytes.Equal(left.PrepareRequest, right.PrepareRequest) && bytes.Equal(left.Enrollment, right.Enrollment) &&
		bytes.Equal(left.Ciphertext, right.Ciphertext) && bytes.Equal(left.Manifest, right.Manifest) &&
		bytes.Equal(left.EncryptedShare, right.EncryptedShare) && bytes.Equal(left.Receipt, right.Receipt) &&
		bytes.Equal(left.ReleaseEnvelope, right.ReleaseEnvelope)
}

func cloneAuthorityState(state State) State {
	state.AcceptedRequest = cloneBytes(state.AcceptedRequest)
	state.PrepareRequest = cloneBytes(state.PrepareRequest)
	state.Enrollment = cloneBytes(state.Enrollment)
	state.Ciphertext = cloneBytes(state.Ciphertext)
	state.Manifest = cloneBytes(state.Manifest)
	state.EncryptedShare = cloneBytes(state.EncryptedShare)
	state.Receipt = cloneBytes(state.Receipt)
	state.ReleaseEnvelope = cloneBytes(state.ReleaseEnvelope)
	return state
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte{}, value...)
}

func readBoundedFile(root *os.Root, name string, limit uint32) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular file")
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, ErrRecordLimit
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) > uint64(limit) {
		return nil, ErrRecordLimit
	}
	return data, nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func releaseAuthorityLock(lock *os.File) error {
	if lock == nil {
		return nil
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_UN); err != nil {
		return errors.Join(fmt.Errorf("unlock authority volume: %w", err), lock.Close())
	}
	if err := lock.Close(); err != nil {
		return fmt.Errorf("close authority lock: %w", err)
	}
	return nil
}

func syncRoot(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return errors.Join(err, directory.Close())
	}
	return directory.Close()
}

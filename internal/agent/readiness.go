package agent

import (
	"github.com/Crank-Git/epochalypse/internal/runtimeinfo"
	"os"
	"path/filepath"
)

// Readiness reads current authority and clock evidence without a durable decision.
func (s *Service) Readiness() runtimeinfo.Report {
	report := runtimeinfo.New("agent", s.transitions.agentID)
	report.Synthetic = s.synthetic
	report.ConfigurationReady = true
	state, err := s.journal.Snapshot()
	if err != nil {
		return report
	}
	report.StateReady = s.journal.readinessStorage() && !state.UnsafeClock
	if s.transitions.clock != nil {
		_, err = s.transitions.clock()
		report.ClockReady = err == nil
	}
	// A released manifest needs no new clock decision.
	report.RecoveryAvailable = report.StateReady && state.PossibleDisclosure
	report.Ready = report.StateReady && report.ClockReady
	report.ProductionReady = report.Ready && !report.Synthetic
	return report
}

func (j *Journal) readinessStorage() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || j.quarantined || !j.storageReady {
		return false
	}
	if err := checkPrivateDirectory(j.config.Directory); err != nil {
		return false
	}
	if err := checkPrivateDirectory(j.config.CheckpointDirectory); err != nil {
		return false
	}
	return verifyVolumeMarker(j.config) == nil && j.readinessFiles()
}

func (j *Journal) readinessFiles() bool {
	journal, err := os.Lstat(filepath.Join(j.config.Directory, authorityJournalName))
	active, statErr := j.journal.Stat()
	if err != nil || statErr != nil || !os.SameFile(journal, active) || !ownerOnlyFile(journal) {
		return false
	}
	if !j.readinessReservation(j.journal, j.config.MaxJournalBytes) {
		return false
	}
	var prior os.FileInfo
	for _, name := range []string{authorityCheckpointName, checkpointSpareName} {
		path := filepath.Join(j.config.CheckpointDirectory, name)
		info, err := os.Lstat(path)
		if err != nil || !ownerOnlyFile(info) || (prior != nil && os.SameFile(prior, info)) {
			return false
		}
		file, err := os.Open(path) // #nosec G304 -- The validated operator checkpoint directory fixes both slot paths.
		if err != nil {
			return false
		}
		ready := j.readinessReservation(file, int64(j.config.MaxRecordBytes))
		closeErr := file.Close()
		if !ready || closeErr != nil {
			return false
		}
		prior = info
	}
	return true
}

func (j *Journal) readinessReservation(file *os.File, size int64) bool {
	info, err := file.Stat()
	if err != nil || info.Size() > size {
		return false
	}
	return j.config.LabOnlySyntheticStorage || inspectPhysicalReservation(file, size) == nil
}

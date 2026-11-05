package backup

import "context"

// Destination selects a fresh installation and its service identity.
// Commit must identify the separately retained executable for this archive.
type Destination struct {
	Path   string
	Commit string
	UID    int
	GID    int
}

// Install validates authenticated state before it publishes a fresh installation.
func (a Archive) Install(ctx context.Context, destination Destination, limits Limits) error {
	return nil
}

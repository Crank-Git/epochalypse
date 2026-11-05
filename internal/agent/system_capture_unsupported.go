//go:build !linux

package agent

// NewSystemCapture rejects platforms without the Linux clock evidence source.
func NewSystemCapture() (func() (Capture, error), error) {
	return nil, ErrClock
}

const (
	clockBoottime  int32 = 7
	clockMonotonic int32 = 1
)

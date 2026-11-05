package capsule

import (
	"io"
	"time"
)

// ImportPrepared persists a structurally validated tlock draft without an unwrapped key.
func (s *Store) ImportPrepared(content io.Reader, now time.Time) (Record, error) {
	return Record{}, ErrInput
}

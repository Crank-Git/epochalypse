package capsule

import "context"

// Delete removes local content only after exact identifier confirmation.
func (s *Store) Delete(ctx context.Context, id, confirmation string) error {
	return ErrUnsupportedMode
}

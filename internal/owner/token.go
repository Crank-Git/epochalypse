package owner

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

var tokenBucket = []byte("tokens-v1")

var (
	ErrTokenRequest = errors.New("invalid token request")
	ErrScope        = errors.New("insufficient scope")
)

// TokenRequest specifies the access and expiry of a new token.
type TokenRequest struct {
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

type tokenRecord struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	OwnerID string `json:"owner_id"`
	Digest  string `json:"digest"`
	TokenRequest
	CreatedAt time.Time `json:"created_at"`
}

// TokenGrant contains the token value for its sole display at creation.
type TokenGrant struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

func validScope(scope string) bool {
	switch scope {
	case "read", "capsule:write", "switch:write", "switch:checkin", "countdown:write":
		return true
	}
	return false
}

func (r TokenRequest) valid(now time.Time) bool {
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 128 || !utf8.ValidString(r.Name) || len(r.Scopes) == 0 || len(r.Scopes) > 5 || !r.ExpiresAt.After(now) {
		return false
	}
	seen := make(map[string]bool, len(r.Scopes))
	for _, scope := range r.Scopes {
		if !validScope(scope) || seen[scope] {
			return false
		}
		seen[scope] = true
	}
	return true
}

// CreateToken persists only the digest of a new token.
// The caller must authorize owner administration before this call.
func (s *Store) CreateToken(request TokenRequest, now time.Time) (TokenGrant, error) {
	if !request.valid(now) {
		return TokenGrant{}, ErrTokenRequest
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	digest := sha256.Sum256(raw)
	grant := TokenGrant{hex.EncodeToString(id), base64.RawURLEncoding.EncodeToString(raw)}
	err := s.db.Update(func(tx *bolt.Tx) error {
		identity, err := readIdentity(tx)
		if err != nil {
			return err
		}
		record := tokenRecord{1, grant.ID, identity.ID, hex.EncodeToString(digest[:]), request, now.UTC()}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if err := tx.Bucket(tokenBucket).Put(digest[:], encoded); err != nil {
			return err
		}
		return appendAudit(tx, tokenCreated, now)
	})
	if err != nil {
		return TokenGrant{}, err
	}
	return grant, nil
}

func validTokenID(id string) bool {
	if len(id) != 32 {
		return false
	}
	data, err := hex.DecodeString(id)
	return err == nil && hex.EncodeToString(data) == id
}

// RevokeToken deletes a token. Repeated calls succeed.
// The caller must authorize owner administration before this call.
func (s *Store) RevokeToken(id string) error {
	if !validTokenID(id) {
		return ErrTokenRequest
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(tokenBucket)
		var key []byte
		if err := bucket.ForEach(func(k, data []byte) error {
			var record tokenRecord
			if err := json.Unmarshal(data, &record); err != nil {
				return errors.New("invalid token state")
			}
			if record.ID == id {
				key = append([]byte(nil), k...)
			}
			return nil
		}); err != nil {
			return err
		}
		if key == nil {
			return nil
		}
		if err := bucket.Delete(key); err != nil {
			return err
		}
		return appendAudit(tx, tokenRevoked, time.Now())
	})
}

// AuthenticateToken checks the current scope and expiry without an idle extension.
// The caller supplies a trusted clock. Invalid credentials return ErrCredentials.
func (s *Store) AuthenticateToken(value, scope string, now time.Time) (string, error) {
	if !validScope(scope) {
		return "", ErrScope
	}
	digest, err := sessionKey(value)
	if err != nil {
		return "", err
	}
	var ownerID string
	err = s.db.View(func(tx *bolt.Tx) error {
		identity, err := readIdentity(tx)
		if err != nil {
			return err
		}
		data := tx.Bucket(tokenBucket).Get(digest)
		if data == nil {
			return ErrCredentials
		}
		var record tokenRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return errors.New("invalid token state")
		}
		if record.Digest != hex.EncodeToString(digest) || !record.valid(identity.ID, now) {
			return ErrCredentials
		}
		for _, allowed := range record.Scopes {
			if scope == allowed {
				ownerID = identity.ID
				return nil
			}
		}
		return ErrScope
	})
	if err != nil {
		return "", err
	}
	return ownerID, nil
}

func (r tokenRecord) valid(ownerID string, now time.Time) bool {
	return r.Version == 1 && validTokenID(r.ID) && r.OwnerID == ownerID && !r.CreatedAt.IsZero() &&
		!now.Before(r.CreatedAt) && r.TokenRequest.valid(r.CreatedAt) && now.Before(r.ExpiresAt)
}

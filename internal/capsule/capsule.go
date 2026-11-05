package capsule

import (
	"errors"
	"time"
)

// Mode identifies the capsule's release trust model.
type Mode string

const (
	Public     Mode = "public"
	ServerHeld Mode = "server-held"
	Tlock      Mode = "tlock"
)

// Kind identifies UTF-8 text or opaque file content.
type Kind string

const (
	Text Kind = "text"
	File Kind = "file"
)

// Input supplies preparation metadata separately from the bounded content reader.
type Input struct {
	Mode      Mode
	Kind      Kind
	Title     string
	Filename  string
	ReleaseAt time.Time
}

// Manifest contains public metadata without a content key.
type Manifest struct {
	Version            int        `json:"version"`
	ID                 string     `json:"id"`
	Mode               Mode       `json:"mode"`
	Kind               Kind       `json:"kind"`
	Title              string     `json:"title"`
	Filename           string     `json:"filename"`
	CreatedAt          time.Time  `json:"created_at"`
	RequestedReleaseAt time.Time  `json:"requested_release_at"`
	EffectiveReleaseAt *time.Time `json:"effective_release_at"`
	PublishedAt        *time.Time `json:"published_at"`
	PlaintextBytes     int        `json:"plaintext_bytes"`
	ObjectSHA256       string     `json:"object_sha256"`
}

// Record contains the manifest and authenticated object without a content key.
type Record struct {
	Manifest
	Object string `json:"object"`
}

var (
	ErrNotFound        = errors.New("capsule does not exist")
	ErrInput           = errors.New("invalid capsule input")
	ErrUnsupportedMode = errors.New("capsule mode is unavailable")
	ErrTooEarly        = errors.New("capsule key is not released")
	ErrUnavailable     = errors.New("capsule key is unavailable")
	ErrIntegrity       = errors.New("capsule integrity check fails")
)

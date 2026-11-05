package service

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

func TestSwitchHTTPWithholdsRecordWhenStorageIsClosed(t *testing.T) {
	h, token := newSwitchHTTP(t)
	created := h.send(countdownRequest("POST", "/api/v1/switches", `{}`, token))
	if created.Code != 201 {
		t.Fatalf("draft creation: %d %s", created.Code, created.Body)
	}
	var record struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if err := h.state.content.Close(); err != nil {
		t.Fatal(err)
	}
	response := h.send(countdownRequest("GET", "/api/v1/switches/"+record.ID, "", token))
	if response.Code != 503 || strings.Contains(response.Body.String(), record.ID) || strings.Contains(response.Body.String(), `"state"`) {
		t.Fatalf("closed-store record response: %d %s", response.Code, response.Body)
	}
}

func corruptCapsuleRecord(t *testing.T, h capsuleHTTPFixture, id string) {
	t.Helper()
	if err := h.state.content.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(installation.CapsulesBucket))
		return bucket.Put([]byte(id), []byte("synthetic-corrupt-record"))
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCapsulePublishRejectsCorruptRetainedRecord(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "server-held")
	corruptCapsuleRecord(t, h, draft.ID)
	response := h.publish(draft.ID)
	if response.Code != 503 || strings.Contains(response.Body.String(), "1234") || strings.Contains(response.Body.String(), `"manifest"`) {
		t.Fatalf("corrupt publish response: %d %s", response.Code, response.Body)
	}
	var stored []byte
	if err := h.state.content.View(func(tx *bolt.Tx) error {
		stored = bytes.Clone(tx.Bucket([]byte(installation.CapsulesBucket)).Get([]byte(draft.ID)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, []byte("synthetic-corrupt-record")) {
		t.Fatal("failed publish changes the corrupt record")
	}
}

func TestPublicCapsuleBundleRejectsCorruptRetainedRecord(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "server-held")
	if response := h.publish(draft.ID); response.Code != 200 {
		t.Fatalf("publish fixture: %d %s", response.Code, response.Body)
	}
	corruptCapsuleRecord(t, h, draft.ID)
	path := "https://localhost/api/v1/public/capsules/" + draft.ID + "/bundle"
	response := h.send(httptest.NewRequest("GET", path, nil))
	if response.Code != 503 || strings.Contains(response.Body.String(), "1234") || strings.Contains(response.Body.String(), `"record"`) {
		t.Fatalf("corrupt bundle response: %d %s", response.Code, response.Body)
	}
	var stored []byte
	if err := h.state.content.View(func(tx *bolt.Tx) error {
		stored = bytes.Clone(tx.Bucket([]byte(installation.CapsulesBucket)).Get([]byte(draft.ID)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, []byte("synthetic-corrupt-record")) {
		t.Fatal("failed bundle request changes the corrupt record")
	}
}

func TestPublicCapsulePageRejectsCorruptRetainedRecord(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "public")
	if response := h.publish(draft.ID); response.Code != 200 {
		t.Fatalf("publish fixture: %d %s", response.Code, response.Body)
	}
	corruptCapsuleRecord(t, h, draft.ID)
	response := h.send(httptest.NewRequest("GET", "https://localhost/c/"+draft.ID, nil))
	if response.Code != 503 || strings.Contains(response.Body.String(), "1234") || strings.Contains(response.Body.String(), "uploadRan") {
		t.Fatalf("corrupt page response: %d %s", response.Code, response.Body)
	}
}

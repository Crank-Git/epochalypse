package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/installation"
	bolt "go.etcd.io/bbolt"
)

type cancelingCapsuleBeacon struct {
	cancel context.CancelFunc
}

func (b cancelingCapsuleBeacon) Fetch(context.Context, uint64) ([]byte, error) {
	b.cancel()
	return nil, context.Canceled
}

func TestPublicCapsuleCancellationWithholdsPlaintext(t *testing.T) {
	for _, pathType := range []string{"page", "download"} {
		t.Run(pathType, func(t *testing.T) {
			h, ids, _ := publicCapsuleFixtures(t)
			*h.clock = h.clock.Add(time.Hour)
			ctx, cancel := context.WithCancel(context.Background())
			h.api.beacon = cancelingCapsuleBeacon{cancel: cancel}
			path := "/c/" + ids["tlock"]
			if pathType == "download" {
				path = "/api/v1/public/capsules/" + ids["tlock"] + "/download"
			}

			request := httptest.NewRequest("GET", "https://localhost"+path, nil).WithContext(ctx)
			response := h.send(request)
			if response.Code != 408 {
				t.Fatalf("canceled %s: %d %s", pathType, response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), "uploadRan") || strings.Contains(response.Body.String(), "Verified and available") {
				t.Fatalf("canceled %s exposes capsule content: %s", pathType, response.Body)
			}
		})
	}
}

type cancelAtEndReader struct {
	reader io.Reader
	cancel context.CancelFunc
}

func (r cancelAtEndReader) Read(data []byte) (int, error) {
	count, err := r.reader.Read(data)
	if err == io.EOF {
		r.cancel()
	}
	return count, err
}

func capsuleCounts(t *testing.T, h capsuleHTTPFixture) (int, int) {
	t.Helper()
	var records, keys int
	if err := h.state.content.View(func(tx *bolt.Tx) error {
		records = tx.Bucket([]byte(installation.CapsulesBucket)).Stats().KeyN
		keys = tx.Bucket([]byte(installation.CapsuleKeysBucket)).Stats().KeyN
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return records, keys
}

func TestCapsuleImportCancellationDoesNotRetainDraft(t *testing.T) {
	h := newCapsuleHTTP(t)
	local, err := capsule.Open(t.TempDir(), installation.DefaultLimits(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = local.Close() }()
	draft, err := local.Create(capsule.Input{Mode: capsule.Tlock, Kind: capsule.Text, ReleaseAt: h.clock.Add(time.Hour)}, strings.NewReader("1234"), *h.clock)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords, beforeKeys := capsuleCounts(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest("POST", "https://localhost/api/v1/capsules/prepared", nil).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+h.bearer)
	request.Body = io.NopCloser(cancelAtEndReader{reader: bytes.NewReader(data), cancel: cancel})
	response := h.send(request)
	if response.Code != 408 {
		t.Fatalf("canceled import: %d %s", response.Code, response.Body)
	}
	afterRecords, afterKeys := capsuleCounts(t, h)
	if afterRecords != beforeRecords || afterKeys != beforeKeys {
		t.Fatalf("canceled import changes retained state: records %d to %d, keys %d to %d", beforeRecords, afterRecords, beforeKeys, afterKeys)
	}
}

func TestCapsulePublishCancellationLeavesDraftUnpublished(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "public")
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest("POST", "https://localhost/api/v1/capsules/"+draft.ID+"/publish", nil).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+h.bearer)
	request.Body = io.NopCloser(cancelAtEndReader{reader: strings.NewReader("{}"), cancel: cancel})
	response := h.send(request)
	if response.Code != 408 {
		t.Fatalf("canceled publish: %d %s", response.Code, response.Body)
	}
	record, err := h.store.Get(draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.PublishedAt != nil {
		t.Fatal("canceled publish changes the draft")
	}
}

func TestCapsuleKeyCancellationWithholdsReleasedKey(t *testing.T) {
	h := newCapsuleHTTP(t)
	draft := createHTTPCapsule(t, h, "public")
	if response := h.publish(draft.ID); response.Code != 200 {
		t.Fatalf("publish fixture: %d %s", response.Code, response.Body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.api.now = func() time.Time {
		cancel()
		return *h.clock
	}
	request := httptest.NewRequest("GET", "https://localhost/api/v1/public/capsules/"+draft.ID+"/key", nil).WithContext(ctx)
	response := h.send(request)
	if response.Code != 408 || strings.Contains(response.Body.String(), `"key"`) {
		t.Fatalf("canceled key request: %d %s", response.Code, response.Body)
	}
	record, err := h.store.Get(draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.PublishedAt == nil {
		t.Fatal("key request changes publication state")
	}
}

func TestPublicCapsuleStylesheetServesWithoutChangingCapsules(t *testing.T) {
	h := newCapsuleHTTP(t)
	beforeRecords, beforeKeys := capsuleCounts(t, h)
	response := h.send(httptest.NewRequest("GET", "https://localhost/c/style.css", nil))
	if response.Code != 200 || response.Header().Get("Content-Type") != "text/css; charset=utf-8" || response.Body.Len() == 0 {
		t.Fatalf("stylesheet response: %d %q %s", response.Code, response.Header().Get("Content-Type"), response.Body)
	}
	afterRecords, afterKeys := capsuleCounts(t, h)
	if afterRecords != beforeRecords || afterKeys != beforeKeys {
		t.Fatal("stylesheet request changes retained capsule state")
	}
}

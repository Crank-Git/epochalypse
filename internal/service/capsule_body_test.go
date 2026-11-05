package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
)

type capsuleTestPart struct {
	name, value, filename string
	extra                 textproto.MIMEHeader
}

func standardCapsuleParts(h capsuleHTTPFixture) []capsuleTestPart {
	return []capsuleTestPart{{name: "mode", value: "server-held"}, {name: "kind", value: "text"}, {name: "release_at", value: h.clock.Add(time.Hour).Format(time.RFC3339)}, {name: "title", value: "Synthetic capsule"}, {name: "content", value: "1234"}}
}

func multipartCapsuleRequest(t *testing.T, h capsuleHTTPFixture, parts []capsuleTestPart) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, spec := range parts {
		params := map[string]string{"name": spec.name}
		if spec.filename != "" {
			params["filename"] = spec.filename
		}
		headers := textproto.MIMEHeader{"Content-Disposition": {mime.FormatMediaType("form-data", params)}}
		for name, values := range spec.extra {
			headers[name] = values
		}
		part, err := writer.CreatePart(headers)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, spec.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://localhost/api/v1/capsules", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+h.bearer)
	return r
}

func TestCapsuleMultipartRejectsMalformedFields(t *testing.T) {
	h := newCapsuleHTTP(t)
	for _, test := range []struct {
		name   string
		change func([]capsuleTestPart) []capsuleTestPart
		status int
	}{
		{"duplicate field", func(p []capsuleTestPart) []capsuleTestPart { return append(p, p[0]) }, 400},
		{"unknown field", func(p []capsuleTestPart) []capsuleTestPart {
			return append(p, capsuleTestPart{name: "unknown", value: "value"})
		}, 400},
		{"missing mode", func(p []capsuleTestPart) []capsuleTestPart { return p[1:] }, 400},
		{"missing content", func(p []capsuleTestPart) []capsuleTestPart { return p[:4] }, 400},
		{"invalid time", func(p []capsuleTestPart) []capsuleTestPart { p[2].value = "invalid"; return p }, 400},
		{"fractional time", func(p []capsuleTestPart) []capsuleTestPart {
			p[2].value = h.clock.Add(time.Hour + time.Nanosecond).Format(time.RFC3339Nano)
			return p
		}, 400},
		{"unsupported mode", func(p []capsuleTestPart) []capsuleTestPart { p[0].value = "tlock"; return p }, 400},
		{"invalid UTF-8 text", func(p []capsuleTestPart) []capsuleTestPart { p[4].value = string([]byte{255}); return p }, 400},
		{"large title", func(p []capsuleTestPart) []capsuleTestPart { p[3].value = strings.Repeat("x", 257); return p }, 413},
		{"large mode", func(p []capsuleTestPart) []capsuleTestPart { p[0].value = strings.Repeat("x", 33); return p }, 413},
		{"file metadata", func(p []capsuleTestPart) []capsuleTestPart { p[3].filename = "title.txt"; return p }, 400},
		{"transfer encoding", func(p []capsuleTestPart) []capsuleTestPart {
			p[4].extra = textproto.MIMEHeader{"Content-Transfer-Encoding": {"quoted-printable"}}
			return p
		}, 400},
		{"extra headers", func(p []capsuleTestPart) []capsuleTestPart {
			p[4].extra = textproto.MIMEHeader{"Content-Type": {"application/octet-stream"}, "X-Unknown": {"value"}}
			return p
		}, 400},
		{"duplicate header", func(p []capsuleTestPart) []capsuleTestPart {
			p[4].extra = textproto.MIMEHeader{"Content-Type": {"text/plain", "text/plain"}}
			return p
		}, 400},
		{"large header", func(p []capsuleTestPart) []capsuleTestPart {
			p[4].extra = textproto.MIMEHeader{"Content-Type": {strings.Repeat("x", 1025)}}
			return p
		}, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := multipartCapsuleRequest(t, h, test.change(standardCapsuleParts(h)))
			if w := h.send(r); w.Code != test.status {
				t.Fatalf("malformed multipart: %d %s", w.Code, w.Body)
			}
			if capsuleHTTPCount(t, h) != 0 {
				t.Fatal("malformed metadata retains a draft")
			}
		})
	}
}

func TestCapsuleMultipartInfersTextAndOpaqueFiles(t *testing.T) {
	h := newCapsuleHTTP(t)
	for _, file := range []bool{false, true} {
		parts := standardCapsuleParts(h)
		parts = append(parts[:1], parts[2:]...)
		if file {
			parts[3].filename = "synthetic.bin"
			parts[3].value = string([]byte{255, 0})
		}
		w := h.send(multipartCapsuleRequest(t, h, parts))
		if w.Code != 201 {
			t.Fatalf("inferred kind: %d %s", w.Code, w.Body)
		}
		var manifest capsule.Manifest
		if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		if file && (manifest.Kind != capsule.File || manifest.Filename != "synthetic.bin" || manifest.PlaintextBytes != 2) {
			t.Fatal("file inference changes opaque bytes or metadata")
		}
		if !file && manifest.Kind != capsule.Text {
			t.Fatal("text inference selects another kind")
		}
	}
}

func TestCapsuleMultipartRejectsInvalidContentType(t *testing.T) {
	h := newCapsuleHTTP(t)
	for _, contentType := range []string{"", "application/json", "invalid", "multipart/form-data", "multipart/form-data; boundary=" + strings.Repeat("x", 71), "multipart/form-data; boundary=x; charset=utf-8", strings.Repeat("x", 257)} {
		r := h.upload(t, "server-held", "1234")
		r.Header.Set("Content-Type", contentType)
		if w := h.send(r); w.Code != 400 {
			t.Fatalf("invalid content type: %d %s", w.Code, w.Body)
		}
	}
	r := h.upload(t, "server-held", "1234")
	r.Header.Add("Content-Type", r.Header.Get("Content-Type"))
	if w := h.send(r); w.Code != 400 {
		t.Fatalf("duplicate content type: %d", w.Code)
	}
}

func TestCapsuleMultipartBoundsUnknownLengthAndEpilogue(t *testing.T) {
	h := newCapsuleHTTP(t)
	r := h.upload(t, "server-held", "1234")
	r.ContentLength = capsuleUploadOverhead + 5
	if w := h.send(r); w.Code != 413 {
		t.Fatalf("declared body limit: %d", w.Code)
	}
	r = h.upload(t, "server-held", "1234")
	r.Body = io.NopCloser(io.MultiReader(r.Body, strings.NewReader(strings.Repeat("x", capsuleUploadOverhead+5))))
	r.ContentLength = -1
	r.TransferEncoding = []string{"chunked"}
	if w := h.send(r); w.Code != 413 {
		t.Fatalf("unknown-length epilogue limit: %d %s", w.Code, w.Body)
	}
	r = h.upload(t, "server-held", "1234")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	r.Body = io.NopCloser(bytes.NewReader(data[:len(data)/2]))
	if w := h.send(r); w.Code != 400 {
		t.Fatalf("truncated body: %d", w.Code)
	}
	if capsuleHTTPCount(t, h) != 0 {
		t.Fatal("a rejected body retains a draft")
	}
}

type capsuleBodyFailure struct{}

func (capsuleBodyFailure) Read([]byte) (int, error) {
	return 0, errors.New("synthetic private upload failure")
}

func TestCapsuleBodyFailureExcludesPrivateErrors(t *testing.T) {
	h := newCapsuleHTTP(t)
	r := h.upload(t, "server-held", "1234")
	r.Body = io.NopCloser(capsuleBodyFailure{})
	r.ContentLength = -1
	if w := h.send(r); w.Code != 400 || strings.Contains(w.Body.String(), "synthetic private upload failure") {
		t.Fatalf("body failure: %d %s", w.Code, w.Body)
	}
}

type cancelCapsuleReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelCapsuleReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

func TestCapsuleCancellationAfterBodyPreventsPreparation(t *testing.T) {
	h := newCapsuleHTTP(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := h.upload(t, "server-held", "1234").WithContext(ctx)
	r.Body = io.NopCloser(cancelCapsuleReader{r.Body, cancel})
	if w := h.send(r); w.Code != 408 {
		t.Fatalf("canceled body: %d %s", w.Code, w.Body)
	}
	if capsuleHTTPCount(t, h) != 0 {
		t.Fatal("body cancellation retains a draft")
	}
}

func TestCapsulePublishRejectsNonemptyOrUnboundedBodies(t *testing.T) {
	h := newCapsuleHTTP(t)
	manifest := createHTTPCapsule(t, h, "server-held")
	for _, test := range []struct {
		contentType, body string
		length            int64
		status            int
	}{
		{"application/json", "{\"unknown\":true}", -1, 400},
		{"application/json", "{}{}", -1, 400},
		{"application/json", "null", -1, 400},
		{"text/plain", "{}", -1, 400},
		{"application/json", "{}", 1025, 413},
		{"application/json", strings.Repeat(" ", 1025), -1, 413},
	} {
		r := httptest.NewRequest("POST", "https://localhost/api/v1/capsules/"+manifest.ID+"/publish", strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer "+h.bearer)
		r.Header.Set("Content-Type", test.contentType)
		r.ContentLength = test.length
		if w := h.send(r); w.Code != test.status {
			t.Fatalf("publication body: %d %s", w.Code, w.Body)
		}
	}
	if _, err := h.store.Published(manifest.ID); !errors.Is(err, capsule.ErrNotFound) {
		t.Fatalf("invalid publication body publishes a draft: %v", err)
	}
}

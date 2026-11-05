package service

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/Crank-Git/epochalypse/internal/capsule"
)

const capsuleUploadOverhead = 65536

func capsuleBody(w http.ResponseWriter, r *http.Request, limit int) (capsule.Input, []byte, error) {
	maximum := int64(limit) + capsuleUploadOverhead
	if r.ContentLength > maximum {
		return capsule.Input{}, nil, &http.MaxBytesError{Limit: maximum}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximum)
	if len(r.Header.Values("Content-Type")) != 1 || len(r.Header.Get("Content-Type")) > 256 {
		return capsule.Input{}, nil, capsule.ErrInput
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" || len(params) != 1 || len(params["boundary"]) == 0 || len(params["boundary"]) > 70 {
		return capsule.Input{}, nil, capsule.ErrInput
	}
	input, content, err := readCapsuleParts(multipart.NewReader(r.Body, params["boundary"]), limit)
	if err != nil {
		return capsule.Input{}, nil, err
	}
	// Chunked requests must finish within the same byte limit before preparation starts.
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		return capsule.Input{}, nil, err
	}
	return input, content, nil
}

func readCapsuleParts(reader *multipart.Reader, limit int) (capsule.Input, []byte, error) {
	upload := capsuleUpload{fields: map[string]string{}, seen: map[string]bool{}}
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return capsule.Input{}, nil, err
		}
		if err := upload.add(part, limit); err != nil {
			return capsule.Input{}, nil, err
		}
	}
	input, err := upload.input()
	return input, upload.content, err
}

type capsuleUpload struct {
	fields   map[string]string
	seen     map[string]bool
	content  []byte
	filename string
}

func (upload *capsuleUpload) add(part *multipart.Part, limit int) error {
	name := part.FormName()
	allowance := capsulePartLimit(name, limit)
	if allowance < 0 || upload.seen[name] || !capsulePartHeaders(part) {
		return capsule.ErrInput
	}
	upload.seen[name] = true
	value, err := io.ReadAll(io.LimitReader(part, int64(allowance)+1))
	if err != nil {
		return err
	}
	if len(value) > allowance {
		return capsule.ErrLimit
	}
	if name == "content" {
		upload.content, upload.filename = value, part.FileName()
		return nil
	}
	if part.FileName() != "" {
		return capsule.ErrInput
	}
	upload.fields[name] = string(value)
	return nil
}

func (upload *capsuleUpload) input() (capsule.Input, error) {
	fields := upload.fields
	if !upload.seen["mode"] || !upload.seen["release_at"] || !upload.seen["content"] {
		return capsule.Input{}, capsule.ErrInput
	}
	if !upload.seen["kind"] {
		fields["kind"] = "text"
		if upload.filename != "" {
			fields["kind"] = "file"
		}
	}
	at, err := time.Parse(time.RFC3339, fields["release_at"])
	if err != nil {
		return capsule.Input{}, capsule.ErrInput
	}
	return capsule.Input{Mode: capsule.Mode(fields["mode"]), Kind: capsule.Kind(fields["kind"]), Title: fields["title"], Filename: upload.filename, ReleaseAt: at}, nil
}

func capsulePartLimit(name string, limit int) int {
	switch name {
	case "content":
		return limit
	case "title":
		return 256
	case "release_at":
		return 64
	case "mode":
		return 32
	case "kind":
		return 16
	default:
		return -1
	}
}

func capsulePartHeaders(part *multipart.Part) bool {
	if len(part.Header) > 2 {
		return false
	}
	for name, values := range part.Header {
		if (name != "Content-Disposition" && name != "Content-Type") || len(values) != 1 || len(values[0]) > 1024 {
			return false
		}
	}
	return true
}

func capsuleEmptyBody(w http.ResponseWriter, r *http.Request) error {
	if r.ContentLength > 1024 {
		return &http.MaxBytesError{Limit: 1024}
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		return err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || data[0] != '{' {
		return capsule.ErrInput
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&struct{}{}) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return capsule.ErrInput
	}
	return nil
}

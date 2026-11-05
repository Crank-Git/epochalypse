package capsule

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

const testID = "0123456789abcdef0123456789abcdef"

func TestObjectAuthenticatesContentAndIdentity(t *testing.T) {
	content := []byte("private capsule content")
	object, key, err := SealObject(testID, FormatVersion, content, len(content))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenObject(testID, FormatVersion, object, key, len(content))
	if err != nil || !bytes.Equal(opened, content) {
		t.Fatalf("round trip: %q, %v", opened, err)
	}
	second, secondKey, err := SealObject(testID, FormatVersion, content, len(content))
	if err != nil || key == secondKey || object == second {
		t.Fatal("each object requires a fresh key and nonce")
	}
	header, _ := base64.RawURLEncoding.DecodeString(strings.Split(object, ".")[0])
	otherID := strings.Repeat("a", 32)
	reboundHeader := base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(header), testID, otherID, 1)))
	for _, tc := range []struct {
		name, id string
		version  int
		object   string
		key      [32]byte
	}{
		{"different identifier", strings.Repeat("a", 32), 1, object, key},
		{"rebound identifier", otherID, 1, mutateSegment(object, 0, reboundHeader), key},
		{"unsupported version", testID, 2, object, key},
		{"different key", testID, 1, object, secondKey},
		{"corrupt ciphertext", testID, 1, mutateSegment(object, 3, "AAAA"), key},
		{"corrupt tag", testID, 1, mutateSegment(object, 4, strings.Repeat("A", 22)), key},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plaintext, err := OpenObject(tc.id, tc.version, tc.object, tc.key, len(content))
			if !errors.Is(err, ErrObject) || plaintext != nil {
				t.Fatalf("authentication failure exposes content: %q, %v", plaintext, err)
			}
			if strings.Contains(err.Error(), string(content)) {
				t.Fatal("error exposes plaintext")
			}
		})
	}
}

func TestObjectEnforcesPlaintextLimit(t *testing.T) {
	if _, _, err := SealObject(testID, 1, []byte("12345"), 4); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversize: %v", err)
	}
	for _, size := range []int{0, 1, 2, 3, DefaultPlaintextLimit} {
		content := bytes.Repeat([]byte("x"), size)
		object, key, err := SealObject(testID, 1, content, DefaultPlaintextLimit)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := OpenObject(testID, 1, object, key, DefaultPlaintextLimit)
		if err != nil || !bytes.Equal(opened, content) {
			t.Fatalf("size %d: %v", size, err)
		}
	}
	object, key, err := SealObject(testID, 1, []byte("12345"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext, err := OpenObject(testID, 1, object, key, 4); err == nil || plaintext != nil {
		t.Fatal("reader admits oversized content")
	}
}

func TestObjectRejectsUnapprovedHeaders(t *testing.T) {
	object, key, err := SealObject(testID, 1, []byte("secret"), 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{
		`{"alg":"dir","enc":"A256GCM","zip":"DEF"}`,
		`{"alg":"dir","enc":"A256GCM","crit":["capsule_id"]}`,
		`{"alg":"dir","enc":"A256GCM","jwk":{}}`,
		`{"alg":"dir","enc":"A256GCM","x5c":[]}`,
		`{"alg":"dir","alg":"dir","enc":"A256GCM"}`,
		`{"alg":null,"enc":"A256GCM"}`,
		`{"alg":"RSA-OAEP","enc":"A256GCM"}`,
		strings.Repeat("x", 1025),
	} {
		candidate := mutateSegment(object, 0, base64.RawURLEncoding.EncodeToString([]byte(header)))
		if plaintext, err := OpenObject(testID, 1, candidate, key, 6); !errors.Is(err, ErrObject) || plaintext != nil {
			t.Fatalf("header accepted: %v", err)
		}
	}
	for _, candidate := range []string{"", "{}", object + ".", strings.Repeat(".", 2000), mutateSegment(object, 1, "AAAA"), mutateSegment(object, 2, "AAAA"), mutateSegment(object, 4, "AAAA")} {
		if plaintext, err := OpenObject(testID, 1, candidate, key, 6); !errors.Is(err, ErrObject) || plaintext != nil {
			t.Fatalf("malformed object accepted: %v", err)
		}
	}
}

func TestObjectRejectsInvalidLimitsAndIdentifiers(t *testing.T) {
	for _, limit := range []int{0, -1, int(^uint(0) >> 1)} {
		if _, err := EncodedObjectLimit(limit); !errors.Is(err, ErrLimit) {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if _, _, err := SealObject(testID, 1, nil, limit); !errors.Is(err, ErrLimit) {
			t.Fatalf("seal limit %d: %v", limit, err)
		}
		if plaintext, err := OpenObject(testID, 1, "", [32]byte{}, limit); !errors.Is(err, ErrLimit) || plaintext != nil {
			t.Fatalf("open limit %d: %v", limit, err)
		}
	}
	for _, id := range []string{"", strings.Repeat("a", 33), strings.Repeat("g", 32), strings.ToUpper(testID)} {
		if _, _, err := SealObject(id, 1, nil, 1); !errors.Is(err, ErrObject) {
			t.Fatalf("identifier accepted: %v", err)
		}
	}
	if _, _, err := SealObject(testID, 2, nil, 1); !errors.Is(err, ErrObject) {
		t.Fatalf("version accepted: %v", err)
	}
	if limit, err := EncodedObjectLimit(DefaultPlaintextLimit); err != nil || limit != 13982080 {
		t.Fatalf("default encoded limit: %d, %v", limit, err)
	}
}

func TestObjectRejectsNoncanonicalSegments(t *testing.T) {
	object, key, err := SealObject(testID, 1, []byte("a"), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, segment := range []string{"A", "AB", "AAB", "AA=", "AA\n", "!A"} {
		if plaintext, err := OpenObject(testID, 1, mutateSegment(object, 3, segment), key, 3); !errors.Is(err, ErrObject) || plaintext != nil {
			t.Fatalf("noncanonical segment accepted: %q, %v", segment, err)
		}
	}
	if plaintext, err := OpenObject(testID, 1, strings.Repeat("x", 2000), key, 1); !errors.Is(err, ErrObject) || plaintext != nil {
		t.Fatalf("oversized object accepted: %v", err)
	}
}

func TestObjectOpensVersionOneFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/object-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		ID        string `json:"id"`
		Version   int    `json:"version"`
		Object    string `json:"object"`
		Key       []byte `json:"key"`
		Plaintext string `json:"plaintext"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Key) != 32 {
		t.Fatal("fixture requires a 32-byte synthetic key")
	}
	plaintext, err := OpenObject(fixture.ID, fixture.Version, fixture.Object, [32]byte(fixture.Key), DefaultPlaintextLimit)
	if err != nil || string(plaintext) != fixture.Plaintext {
		t.Fatalf("version 1 fixture: %v", err)
	}
}

func FuzzObjectRejectsUntrustedInputs(f *testing.F) {
	f.Add("", testID, 1)
	f.Add(strings.Repeat(".", 100), testID, 1)
	f.Fuzz(func(t *testing.T, object, id string, version int) {
		plaintext, err := OpenObject(id, version, object, [32]byte{}, 1024)
		if err != nil && plaintext != nil {
			t.Fatal("failure exposes plaintext")
		}
	})
}

func mutateSegment(object string, segment int, value string) string {
	parts := strings.Split(object, ".")
	parts[segment] = value
	return strings.Join(parts, ".")
}

package service

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestVersionReportsBuildIdentity(t *testing.T) {
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest("GET", "/version", nil))
	var body struct {
		Commit string `json:"commit"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Commit == "" {
		t.Fatalf("version=%d %s", w.Code, w.Body)
	}
}

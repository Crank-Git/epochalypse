package owner

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDashboardReturnsCurrentFeatureStates(t *testing.T) {
	s := sessionStore(t)
	token, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "https://localhost/api/v1/dashboard", nil)
	r.Header.Set("Cookie", cookieName+"="+token)
	w := httptest.NewRecorder()
	s.Handler(false).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("dashboard returns %d: %s", w.Code, w.Body)
	}
	var summary map[string]struct {
		State string            `json:"state"`
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"capsules", "switches", "countdowns", "agents"} {
		list, ok := summary[section]
		if !ok || list.Items == nil || len(list.Items) != 0 {
			t.Fatalf("%s does not show an empty list", section)
		}
		if section == "switches" && list.State != "blocked" {
			t.Fatal("switches do not show the production block")
		}
		if section == "agents" && list.State != "unavailable" {
			t.Fatal("agent health claims an observation")
		}
		if section == "capsules" && list.State != "available" {
			t.Fatal("capsule controls are unavailable")
		}
	}
}

func TestDashboardRequiresOwnerOrExactReadScope(t *testing.T) {
	s := sessionStore(t)
	now := time.Now()
	read, err := s.CreateToken(TokenRequest{Name: "read", Scopes: []string{"read"}, ExpiresAt: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	write, err := s.CreateToken(TokenRequest{Name: "write", Scopes: []string{"capsule:write"}, ExpiresAt: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := s.CreateToken(TokenRequest{Name: "expired", Scopes: []string{"read"}, ExpiresAt: now.Add(-time.Second)}, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.SignIn([]byte("synthetic"), now)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		authorization, cookie string
		code                  int
	}{
		{"", "", 401}, {"Bearer invalid", session, 401}, {"Bearer " + write.Token, session, 403},
		{"Bearer " + read.Token, "", 200}, {"Bearer " + expired.Token, "", 401},
	} {
		r := httptest.NewRequest("GET", "https://localhost/api/v1/dashboard", nil)
		if test.authorization != "" {
			r.Header.Set("Authorization", test.authorization)
		}
		if test.cookie != "" {
			r.Header.Set("Cookie", cookieName+"="+test.cookie)
		}
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		assertDashboardResponse(t, w, test.code, read.Token, write.Token, session)
	}
	if err := s.RevokeToken(read.ID); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "https://localhost/api/v1/dashboard", nil)
	r.Header.Set("Authorization", "Bearer "+read.Token)
	w := httptest.NewRecorder()
	s.Handler(false).ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("revoked read token admits dashboard access")
	}
}

func assertDashboardResponse(t *testing.T, w *httptest.ResponseRecorder, code int, secrets ...string) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("dashboard authorization: got %d want %d", w.Code, code)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("dashboard omits response safeguards")
	}
	for _, secret := range secrets {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("dashboard exposes credentials")
		}
	}
}

func TestDashboardValidatesListLimits(t *testing.T) {
	s := sessionStore(t)
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		query string
		code  int
	}{
		{"", 200}, {"?limit=1", 200}, {"?limit=100", 200}, {"?limit=0", 400}, {"?limit=101", 400},
		{"?limit=-1", 400}, {"?limit=bad", 400}, {"?limit=", 400}, {"?limit=1&limit=2", 400},
	} {
		r := httptest.NewRequest("GET", "https://localhost/api/v1/dashboard"+test.query, nil)
		r.Header.Set("Cookie", cookieName+"="+session)
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, r)
		if w.Code != test.code {
			t.Fatalf("%s returns %d", test.query, w.Code)
		}
		if w.Body.Len() > 4096 {
			t.Fatal("empty summary exceeds its response bound")
		}
	}
	for _, limit := range []int{0, 101} {
		if _, err := s.Dashboard(limit); !errors.Is(err, ErrDashboardLimit) {
			t.Fatalf("invalid store limit: %v", err)
		}
	}
}

func TestDashboardShowsSignInWithoutOwnerAuthorization(t *testing.T) {
	s := sessionStore(t)
	session, err := s.SignIn([]byte("synthetic"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "https://localhost/owner", nil)
	r.Header.Set("Cookie", cookieName+"="+session)
	r.Header.Set("Authorization", "Bearer invalid")
	w := httptest.NewRecorder()
	s.Handler(false).ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "Owner sign-in") || strings.Contains(w.Body.String(), "Agent health") {
		t.Fatal("explicit bearer falls back to owner dashboard")
	}
}

func TestDashboardEscapesObjectMetadata(t *testing.T) {
	const content = `<img src=x onerror="alert(1)">`
	summary := DashboardSummary{Capsules: SummaryList{State: "active", Items: []SummaryItem{{ID: content, Name: content, State: content}}}}
	w := httptest.NewRecorder()
	renderDashboard(w, content, summary)
	if strings.Contains(w.Body.String(), content) || !strings.Contains(w.Body.String(), "&lt;img") {
		t.Fatal("dashboard does not escape object metadata")
	}
	for _, text := range []string{`aria-label="Dashboard sections"`, `aria-labelledby="capsules-heading"`, `href="#dashboard"`, `name="csrf_token"`} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatalf("dashboard omits %s", text)
		}
	}
}

func TestDashboardFailsClosedOnUnavailableState(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dashboard(1); err == nil {
		t.Fatal("closed dashboard succeeds")
	}
	w := httptest.NewRecorder()
	s.dashboardJSON(w, httptest.NewRequest("GET", "https://localhost/api/v1/dashboard", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "Try again") {
		t.Fatal("dashboard JSON omits retry guidance")
	}
	w = httptest.NewRecorder()
	s.dashboardHTML(w, httptest.NewRequest("GET", "https://localhost/owner", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "Retry dashboard") || !strings.Contains(w.Body.String(), `role="alert"`) {
		t.Fatal("dashboard HTML omits retry guidance")
	}
}

func TestDashboardRequiresTLSOutsideTheLab(t *testing.T) {
	s := sessionStore(t)
	for _, path := range []string{"/api/v1/dashboard", "/owner/style.css"} {
		w := httptest.NewRecorder()
		s.Handler(false).ServeHTTP(w, httptest.NewRequest("GET", "http://localhost"+path, nil))
		if w.Code != 426 {
			t.Fatalf("%s bypasses TLS", path)
		}
	}
	w := httptest.NewRecorder()
	s.Handler(true).ServeHTTP(w, httptest.NewRequest("GET", "http://localhost/owner/style.css", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/css; charset=utf-8" || !strings.Contains(w.Body.String(), ":focus-visible") {
		t.Fatal("dashboard stylesheet omits keyboard focus")
	}
}

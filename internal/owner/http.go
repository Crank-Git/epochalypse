package owner

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"mime"
	"net/http"
	"time"
)

const cookieName = "__Host-epochalypse-session"

var ownerPage = template.Must(template.New("owner").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Owner access — epochalypse</title></head>
<body><main>{{if .SignedIn}}<h1>Signed in</h1><p>Your owner session is active.</p>
<form method="post" action="/owner/sign-out"><button type="submit">Sign out</button></form>
{{else}}<h1>Owner sign-in</h1>{{if .Error}}<p role="alert">Invalid credentials. Try again.</p>{{end}}
<form method="post" action="/owner/sign-in"><label for="password">Password</label>
<input id="password" name="password" type="password" autocomplete="current-password" required maxlength="1024">
<button type="submit">Sign in</button></form>{{end}}</main></body></html>`))

// Handler returns the owner routes. The lab option permits local HTTP requests.
func (s *Store) Handler(lab bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /owner", s.page)
	mux.HandleFunc("POST /api/v1/session", s.signIn)
	mux.HandleFunc("DELETE /api/v1/session", s.signOut)
	mux.HandleFunc("POST /owner/sign-in", s.signIn)
	mux.HandleFunc("POST /owner/sign-out", s.signOut)
	mux.Handle("POST /api/v1/tokens", s.RequireOwner(http.HandlerFunc(s.createToken)))
	mux.Handle("DELETE /api/v1/tokens/{id}", s.RequireOwner(http.HandlerFunc(s.revokeToken)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", rand.Text())
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if !lab && r.TLS == nil {
			webError(w, http.StatusUpgradeRequired, "tls_required", "Use HTTPS.")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func webError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"code": code, "message": message, "fields": map[string]string{}, "request_id": w.Header().Get("X-Request-ID"),
	}})
}

func (s *Store) page(w http.ResponseWriter, r *http.Request) {
	signedIn := false
	if cookie, err := r.Cookie(cookieName); err == nil {
		_, err = s.Authenticate(cookie.Value, time.Now())
		if err != nil && !errors.Is(err, ErrCredentials) {
			webError(w, 503, "unavailable", "Owner access is unavailable.")
			return
		}
		signedIn = err == nil
	}
	renderOwner(w, signedIn, false)
}

func renderOwner(w http.ResponseWriter, signedIn, failed bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = ownerPage.Execute(w, struct{ SignedIn, Error bool }{signedIn, failed})
}

func credentials(w http.ResponseWriter, r *http.Request) (string, error) {
	if r.ContentLength > 8192 {
		return "", &http.MaxBytesError{Limit: 8192}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if r.URL.Path == "/owner/sign-in" {
		if err := r.ParseForm(); err != nil {
			return "", err
		}
		return r.PostForm.Get("password"), nil
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return "", errors.New("require JSON")
	}
	var body struct {
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return "", err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", errors.New("require one document")
	}
	return body.Password, nil
}

func (s *Store) signIn(w http.ResponseWriter, r *http.Request) {
	password, err := credentials(w, r)
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			webError(w, 413, "request_too_large", "The request exceeds 8192 bytes.")
			return
		}
		webError(w, 400, "invalid_request", "Provide a password in the request body.")
		return
	}
	token, err := s.SignIn([]byte(password), time.Now())
	if errors.Is(err, ErrCredentials) {
		if r.URL.Path == "/owner/sign-in" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(401)
			renderOwner(w, false, true)
			return
		}
		webError(w, 401, "invalid_credentials", "Invalid credentials.")
		return
	}
	if err != nil {
		webError(w, 503, "unavailable", "Owner access is unavailable.")
		return
	}
	setSessionCookie(w, token, 0)
	if r.URL.Path == "/owner/sign-in" {
		http.Redirect(w, r, "/owner", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}

func (s *Store) signOut(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		webError(w, 401, "invalid_credentials", "Invalid credentials.")
		return
	}
	if err := s.SignOut(cookie.Value); err != nil {
		if errors.Is(err, ErrCredentials) {
			webError(w, 401, "invalid_credentials", "Invalid credentials.")
			return
		}
		webError(w, 503, "unavailable", "Owner access is unavailable.")
		return
	}
	setSessionCookie(w, "", -1)
	if r.URL.Path == "/owner/sign-out" {
		http.Redirect(w, r, "/owner", http.StatusSeeOther)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package owner

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
)

// The host prefix prevents a sibling host from planting a known proof cookie.
const csrfCookieName = "__Host-epochalypse-csrf"

func csrfToken(w http.ResponseWriter, r *http.Request) string {
	cookies := r.CookiesNamed(csrfCookieName)
	if len(cookies) == 1 && len(cookies[0].Value) == 43 {
		return cookies[0].Value
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: token, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return token
}

func csrfBootstrap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"csrf_token": csrfToken(w, r)})
}

func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	origin, err := url.Parse(origins[0])
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return err == nil && origin.Scheme == scheme && origin.Host == r.Host && origin.User == nil &&
		origin.Path == "" && origin.RawQuery == "" && origin.Fragment == "" && !origin.ForceQuery
}

func requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
		return true
	}
	if !sameOrigin(r) {
		webError(w, 403, "invalid_csrf", "Invalid request verification.")
		return false
	}
	token := r.Header.Get("X-CSRF-Token")
	if len(r.Header.Values("X-CSRF-Token")) > 1 {
		token = ""
	} else if token == "" && (r.URL.Path == "/owner/sign-in" || r.URL.Path == "/owner/sign-out") {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err == nil && len(r.PostForm["csrf_token"]) == 1 {
			token = r.PostForm.Get("csrf_token")
		}
	}
	cookies := r.CookiesNamed(csrfCookieName)
	if len(cookies) != 1 || len(token) != 43 || subtle.ConstantTimeCompare([]byte(cookies[0].Value), []byte(token)) != 1 {
		webError(w, 403, "invalid_csrf", "Invalid request verification.")
		return false
	}
	return true
}

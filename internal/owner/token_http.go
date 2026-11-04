package owner

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

// RequireOwner admits owner sessions for credential administration.
func (s *Store) RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An explicit bearer credential must never fall back to a privileged cookie.
		if len(r.Header.Values("Authorization")) != 0 {
			authorizationError(w, ErrCredentials)
			return
		}
		now := s.webNow()
		cookie, err := r.Cookie(cookieName)
		if err == nil {
			// Rejected request policies must not extend the session's idle deadline.
			_, err = s.authenticateSession(cookie.Value, now, false)
		}
		if err != nil {
			authorizationError(w, err)
			return
		}
		if !requireCSRF(w, r) {
			return
		}
		// Recheck durable state because recovery or revocation can follow the first check.
		if _, err := s.Authenticate(cookie.Value, s.webNow()); err != nil {
			authorizationError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope admits API tokens with the exact scope or authenticated owner sessions.
// Mount the result inside the owner handler's TLS and response safeguards.
func (s *Store) RequireScope(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validScope(scope) {
			authorizationError(w, ErrScope)
			return
		}
		if len(r.Header.Values("Authorization")) == 0 {
			s.RequireOwner(next).ServeHTTP(w, r)
			return
		}
		scheme, value, found := strings.Cut(r.Header.Get("Authorization"), " ")
		if len(r.Header.Values("Authorization")) != 1 || !found || !strings.EqualFold(scheme, "Bearer") {
			authorizationError(w, ErrCredentials)
			return
		}
		if _, err := s.AuthenticateToken(strings.TrimLeft(value, " "), scope, time.Now()); err != nil {
			authorizationError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrScope) {
		w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope"`)
		webError(w, 403, "insufficient_scope", "The token lacks the required scope.")
		return
	}
	if errors.Is(err, ErrCredentials) || errors.Is(err, http.ErrNoCookie) {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		webError(w, 401, "invalid_credentials", "Invalid credentials.")
		return
	}
	webError(w, 503, "unavailable", "Owner access is unavailable.")
}

func tokenBody(w http.ResponseWriter, r *http.Request) (TokenRequest, error) {
	var body TokenRequest
	if r.ContentLength > 8192 {
		return body, &http.MaxBytesError{Limit: 8192}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return body, ErrTokenRequest
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return body, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err != nil {
			return body, err
		}
		return body, ErrTokenRequest
	}
	return body, nil
}

func (s *Store) createToken(w http.ResponseWriter, r *http.Request) {
	body, err := tokenBody(w, r)
	if err != nil {
		tokenRequestError(w, err)
		return
	}
	grant, err := s.CreateToken(body, time.Now())
	if err != nil {
		if errors.Is(err, ErrTokenRequest) {
			tokenRequestError(w, err)
			return
		}
		webError(w, 503, "unavailable", "Owner access is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/tokens/"+grant.ID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(grant)
}

func tokenRequestError(w http.ResponseWriter, err error) {
	var sizeError *http.MaxBytesError
	if errors.As(err, &sizeError) {
		webError(w, 413, "request_too_large", "The request exceeds 8192 bytes.")
		return
	}
	webError(w, 400, "invalid_request", "Provide a name, supported scopes, and a future expires_at.")
}

func (s *Store) revokeToken(w http.ResponseWriter, r *http.Request) {
	if err := s.RevokeToken(r.PathValue("id")); err != nil {
		if errors.Is(err, ErrTokenRequest) {
			tokenRequestError(w, err)
			return
		}
		webError(w, 503, "unavailable", "Owner access is unavailable.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

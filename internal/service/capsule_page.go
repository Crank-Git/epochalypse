package service

import (
	"html/template"
	"net/http"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/web"
)

var capsulePage = template.Must(template.New("capsule").Parse(web.CapsuleHTML))

func (api *capsuleAPI) ownerControls(mux *http.ServeMux, owners *owner.Store, lab bool) {
	mux.Handle("GET /owner/capsules", owner.SecureHandler(lab, owners.RequireOwner(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		_ = capsulePage.Execute(w, api.store.PlaintextLimit())
	}))))
	mux.Handle("GET /owner/capsules.js", owner.SecureHandler(lab, owners.RequireOwner(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(web.CapsuleJS))
	}))))
}

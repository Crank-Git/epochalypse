package service

import (
	"html/template"
	"net/http"

	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/web"
)

var switchPage = template.Must(template.New("switches").Parse(web.SwitchHTML))

func (api *switchAPI) ownerControls(mux *http.ServeMux, owners *owner.Store, lab bool) {
	mux.Handle("GET /owner/switches", owner.SecureHandler(lab, owners.RequireOwner(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		records, err := api.store.List()
		if err != nil {
			switchDomainError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		_ = switchPage.Execute(w, records)
	}))))
	mux.Handle("GET /owner/switches.js", owner.SecureHandler(lab, owners.RequireOwner(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(web.SwitchJS))
	}))))
}

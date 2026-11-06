package service

import (
	"html/template"
	"net/http"

	"github.com/Crank-Git/epochalypse/internal/countdown"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/web"
)

var publicCountdownTemplate = template.Must(template.New("public-countdown").Parse(web.PublicCountdownHTML))

func (api *countdownAPI) publicControls(mux *http.ServeMux, lab bool) {
	for _, path := range []string{"/events/{id}", "/api/v1/public/countdowns/{id}"} {
		mux.Handle(path, owner.SecureHandler(lab, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.Header().Set("Allow", "GET")
				countdownError(w, 405, "method_not_allowed", "Use GET.")
				return
			}
			api.serve(w, r)
		})))
	}
	for _, asset := range []struct{ path, media, body string }{
		{"/events/countdown.js", "text/javascript; charset=utf-8", web.PublicCountdownJS},
		{"/events/style.css", "text/css; charset=utf-8", web.OwnerCSS + "h1{overflow-wrap:anywhere}"},
	} {
		mux.Handle("GET "+asset.path, owner.SecureHandler(lab, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", asset.media)
			_, _ = w.Write([]byte(asset.body))
		})))
	}
}

func (api *countdownAPI) readResponse(w http.ResponseWriter, r *http.Request, record countdown.Record) {
	if r.Pattern == "/events/{id}" {
		state := "Active"
		if !api.now().Before(record.EventAt) {
			state = "Completed"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		_ = publicCountdownTemplate.Execute(w, struct {
			countdown.Record
			State string
		}{record, state})
		return
	}
	if r.Pattern == "/api/v1/public/countdowns/{id}" {
		countdownJSON(w, 200, struct {
			countdown.Record
			countdown.Timing
		}{record, record.Timing(api.now())})
		return
	}
	countdownJSON(w, 200, record)
}

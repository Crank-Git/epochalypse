package service

import (
	"encoding/json"
	"errors"
	"html/template"
	"mime"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Crank-Git/epochalypse/internal/capsule"
	"github.com/Crank-Git/epochalypse/internal/owner"
	"github.com/Crank-Git/epochalypse/web"
)

var publicCapsuleTemplate = template.Must(template.New("public-capsule").Parse(web.PublicCapsuleHTML))

func (api *capsuleAPI) publicControls(mux *http.ServeMux, lab bool) {
	for _, route := range []struct {
		path  string
		serve http.HandlerFunc
	}{
		{"/c/{id}", api.publicPage},
		{"/api/v1/public/capsules/{id}/download", api.download},
		{"/api/v1/public/capsules/{id}/bundle", api.bundle},
	} {
		mux.Handle(route.path, owner.SecureHandler(lab, api.route("GET", route.serve)))
	}
	mux.Handle("GET /c/style.css", owner.SecureHandler(lab, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write([]byte(web.WindowCSS))
	})))
}

type publicCapsuleView struct {
	Manifest                                                             capsule.Manifest
	Requested, Effective, Checked, Trust, Release, Availability, Preview string
	Available, Text                                                      bool
}

func (api *capsuleAPI) publicPage(w http.ResponseWriter, r *http.Request) {
	record, err := api.store.Published(r.PathValue("id"))
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	now := api.now()
	view := publicCapsuleView{
		Manifest: record.Manifest, Requested: record.RequestedReleaseAt.Format(time.RFC3339),
		Effective: record.EffectiveReleaseAt.Format(time.RFC3339), Checked: now.UTC().Format(time.RFC3339),
		Trust: capsuleTrust(record.Mode), Release: "Sealed: release condition is not due.",
	}
	if !now.Before(*record.EffectiveReleaseAt) {
		view.Release = "Release condition is due. Retrieval can still fail."
	}
	plaintext, err := api.openPublic(r, now)
	defer clear(plaintext)
	view.Availability = capsuleAvailability(err)
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	view.Available = err == nil
	if view.Available && record.Kind == capsule.Text {
		view.Text = true
		view.Preview = capsulePreview(plaintext)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_ = publicCapsuleTemplate.Execute(w, view)
}

func capsuleTrust(mode capsule.Mode) string {
	switch mode {
	case capsule.Public:
		return "Public mode is public data after publication. The requested release time is advisory. Its key is public immediately."
	case capsule.ServerHeld:
		return "Server-held mode trusts the server operator. The operator holds the key and can read the content. A server outage can delay availability. A lost key has no automatic recovery."
	default:
		return "Tlock depends on the pinned quicknet beacon and available ciphertext. The server verifies the beacon signature locally. Beacon failure can delay availability. A published tlock bundle remains usable after local deletion. CLI preparation stays on the owner's machine. Web preparation passes plaintext through the server in v1; the operator can read it during preparation."
	}
}

func capsuleAvailability(err error) string {
	switch {
	case err == nil:
		return "Verified and available. Complete content authentication succeeds."
	case errors.Is(err, capsule.ErrTooEarly):
		return "Sealed. Content remains unavailable before the effective release time."
	case errors.Is(err, capsule.ErrIntegrity):
		return "Integrity error. No plaintext download is available."
	default:
		return "Delayed availability. Content, its key, or the beacon is unavailable. Refresh to retry."
	}
}

func capsulePreview(plaintext []byte) string {
	preview := plaintext[:min(len(plaintext), 4096)]
	for !utf8.Valid(preview) && len(preview) > 0 {
		preview = preview[:len(preview)-1]
	}
	return string(preview)
}

func (api *capsuleAPI) openPublic(r *http.Request, now time.Time) ([]byte, error) {
	bundle, err := api.store.Bundle(r.PathValue("id"), now)
	if err != nil {
		return nil, err
	}
	return bundle.Open(r.Context(), api.beacon, now)
}

func (api *capsuleAPI) download(w http.ResponseWriter, r *http.Request) {
	record, err := api.store.Published(r.PathValue("id"))
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	plaintext, err := api.openPublic(r, api.now())
	defer clear(plaintext)
	if err != nil {
		capsulePublicError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": record.Filename}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Length", strconv.Itoa(len(plaintext)))
	_, _ = w.Write(plaintext)
}

func (api *capsuleAPI) bundle(w http.ResponseWriter, r *http.Request) {
	bundle, err := api.store.Bundle(r.PathValue("id"), api.now())
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		capsuleDomainError(w, err)
		return
	}
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="capsule-bundle.json"`)
	_, _ = w.Write(data)
}

func capsulePublicError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil {
		capsuleRequestError(w, r.Context().Err())
		return
	}
	capsuleDomainError(w, err)
}

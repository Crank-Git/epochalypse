package owner

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/Crank-Git/epochalypse/web"
	bolt "go.etcd.io/bbolt"
)

// ErrDashboardLimit identifies a limit outside the supported range.
var ErrDashboardLimit = errors.New("dashboard limit must be from 1 through 100")

// SummaryItem exposes public metadata without content or credentials.
type SummaryItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// SummaryList reports feature availability separately from retained objects.
type SummaryList struct {
	State   string        `json:"state"`
	Message string        `json:"message"`
	Items   []SummaryItem `json:"items"`
}

// DashboardSummary contains the four bounded metadata lists.
type DashboardSummary struct {
	Capsules   SummaryList `json:"capsules"`
	Switches   SummaryList `json:"switches"`
	Countdowns SummaryList `json:"countdowns"`
	Agents     SummaryList `json:"agents"`
}

// Dashboard returns the current feature states with at most limit items per list.
// The caller must authorize access. Invalid limits or owner state return an error.
func (s *Store) Dashboard(limit int) (DashboardSummary, error) {
	if limit < 1 || limit > 100 {
		return DashboardSummary{}, ErrDashboardLimit
	}
	if err := s.db.View(func(tx *bolt.Tx) error { _, err := readIdentity(tx); return err }); err != nil {
		return DashboardSummary{}, fmt.Errorf("read dashboard state: %w", err)
	}
	// Future feature stores supply metadata here after their implementation passes its gates.
	return DashboardSummary{
		Capsules:   SummaryList{"not_implemented", "No capsules. Capsule creation is not available yet.", []SummaryItem{}},
		Switches:   SummaryList{"blocked", "No switches. Switch production use remains blocked by the owner decision.", []SummaryItem{}},
		Countdowns: SummaryList{"not_implemented", "No countdowns. Countdown creation is not available yet.", []SummaryItem{}},
		Agents:     SummaryList{"unavailable", "Agent health is unavailable. This dashboard has no verified agent observations.", []SummaryItem{}},
	}, nil
}

func (s *Store) dashboardJSON(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if values, present := r.URL.Query()["limit"]; present {
		var err error
		limit, err = strconv.Atoi(values[0])
		if len(values) != 1 || err != nil || limit < 1 || limit > 100 {
			webError(w, 400, "invalid_request", "Provide one limit from 1 through 100.")
			return
		}
	}
	summary, err := s.Dashboard(limit)
	if err != nil {
		webError(w, 503, "unavailable", "The dashboard is unavailable. Try again.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summary)
}

var dashboardPage = template.Must(template.New("dashboard").Parse(web.DashboardHTML))

type dashboardSection struct {
	ID, Heading string
	SummaryList
}

func (s *Store) dashboardHTML(w http.ResponseWriter, r *http.Request) {
	summary, err := s.Dashboard(100)
	if err != nil {
		dashboardUnavailable(w)
		return
	}
	renderDashboard(w, csrfToken(w, r), summary)
}

func renderDashboard(w http.ResponseWriter, csrf string, summary DashboardSummary) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dashboardPage.Execute(w, struct {
		CSRF     string
		Sections []dashboardSection
	}{csrf, []dashboardSection{
		{"capsules", "Capsules", summary.Capsules},
		{"switches", "Switches", summary.Switches},
		{"countdowns", "Countdowns", summary.Countdowns},
		{"agents", "Agent health", summary.Agents},
	}})
}

func dashboardUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Dashboard unavailable</title></head><body><main><h1>Dashboard unavailable</h1><p role="alert">The dashboard is unavailable. Try again.</p><a href="/owner">Retry dashboard</a></main></body></html>`))
}

func dashboardStyle(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(web.OwnerCSS))
}

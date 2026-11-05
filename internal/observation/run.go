package observation

import "time"

// Run preserves the resolver results for one immutable window.
type Run struct {
	Version   int        `json:"version"`
	ID        string     `json:"id"`
	WindowID  string     `json:"window_id"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	Started   time.Time  `json:"started"`
	Completed *time.Time `json:"completed"`
	Results   []Result   `json:"results"`
}

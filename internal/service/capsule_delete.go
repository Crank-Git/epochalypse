package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Crank-Git/epochalypse/internal/capsule"
)

func (api *capsuleAPI) resource(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		api.delete(w, r)
		return
	}
	api.metadata(w, r)
}

func (api *capsuleAPI) delete(w http.ResponseWriter, r *http.Request) {
	confirmation, err := capsuleDeleteConfirmation(w, r)
	if err != nil {
		capsuleRequestError(w, err)
		return
	}
	if err := api.store.Delete(r.Context(), r.PathValue("id"), confirmation); err != nil {
		if r.Context().Err() != nil {
			capsuleRequestError(w, r.Context().Err())
		} else {
			capsuleDomainError(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func capsuleDeleteConfirmation(w http.ResponseWriter, r *http.Request) (string, error) {
	if len(r.Header.Values("Content-Type")) != 1 || r.Header.Get("Content-Type") != "application/json" {
		return "", capsule.ErrInput
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128))
	if err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 1 {
		return "", capsule.ErrInput
	}
	var confirmation string
	if json.Unmarshal(fields["confirm"], &confirmation) != nil {
		return "", capsule.ErrInput
	}
	// Exact canonical proof rejects duplicate fields and alternate identifier encodings.
	canonical, _ := json.Marshal(struct {
		Confirm string `json:"confirm"`
	}{confirmation})
	if !bytes.Equal(data, canonical) {
		return "", capsule.ErrInput
	}
	return confirmation, nil
}

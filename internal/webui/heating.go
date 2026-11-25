package webui

import (
	"errors"
	"net/http"

	"github.com/mlipscombe/boiler-mate/internal/heating"
)

func (ui *UI) getExternalHeating(w http.ResponseWriter, _ *http.Request) {
	if ui.heating == nil {
		problem(w, http.StatusServiceUnavailable, "External heating control is unavailable.")
		return
	}
	writeJSON(w, ui.heating.Snapshot())
}

func (ui *UI) putExternalHeating(w http.ResponseWriter, r *http.Request) {
	if ui.heating == nil {
		problem(w, http.StatusServiceUnavailable, "External heating control is unavailable.")
		return
	}
	var settings heating.Settings
	if !readJSON(w, r, &settings) {
		return
	}
	if err := ui.heating.Set(settings); err != nil {
		code := http.StatusUnprocessableEntity
		switch {
		case errors.Is(err, heating.ErrReadOnly):
			code = http.StatusForbidden
		case errors.Is(err, heating.ErrSave):
			code = http.StatusInternalServerError
		}
		problem(w, code, err.Error())
		return
	}
	writeJSON(w, ui.heating.Snapshot())
}

func (ui *UI) resumeExternalHeating(w http.ResponseWriter, _ *http.Request) {
	if ui.heating == nil {
		problem(w, http.StatusServiceUnavailable, "External heating control is unavailable.")
		return
	}
	if err := ui.heating.Resume(); err != nil {
		code := http.StatusConflict
		switch {
		case errors.Is(err, heating.ErrReadOnly):
			code = http.StatusForbidden
		case errors.Is(err, heating.ErrSave):
			code = http.StatusInternalServerError
		}
		problem(w, code, err.Error())
		return
	}
	writeJSON(w, ui.heating.Snapshot())
}

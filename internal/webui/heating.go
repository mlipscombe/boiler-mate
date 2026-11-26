package webui

import (
	"bytes"
	"encoding/json"
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
	ui.writeHeatingSettingsResult(w, ui.heating.Set(settings))
}

func (ui *UI) patchExternalHeating(w http.ResponseWriter, r *http.Request) {
	if ui.heating == nil {
		problem(w, http.StatusServiceUnavailable, "External heating control is unavailable.")
		return
	}
	var patch map[string]json.RawMessage
	if !readJSON(w, r, &patch) {
		return
	}
	if len(patch) == 0 {
		problem(w, http.StatusBadRequest, "Expected at least one heating setting.")
		return
	}
	invalid := errors.New("invalid heating setting")
	err := ui.heating.UpdateSettings(func(current heating.Settings) (heating.Settings, error) {
		var settings heating.Settings
		encoded, err := json.Marshal(current)
		if err != nil {
			return settings, heating.ErrSave
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			return settings, heating.ErrSave
		}
		for name, value := range patch {
			if _, ok := fields[name]; !ok {
				return settings, invalid
			}
			if bytes.Equal(value, []byte("null")) && name != "stopAt" && name != "thermostats" && name != "bands" {
				return settings, invalid
			}
			fields[name] = value
		}
		encoded, err = json.Marshal(fields)
		if err != nil {
			return settings, invalid
		}
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&settings); err != nil {
			return settings, invalid
		}
		return settings, nil
	})
	if errors.Is(err, invalid) {
		problem(w, http.StatusBadRequest, "Invalid heating setting.")
		return
	}
	ui.writeHeatingSettingsResult(w, err)
}

func (ui *UI) writeHeatingSettingsResult(w http.ResponseWriter, err error) {
	if err != nil {
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

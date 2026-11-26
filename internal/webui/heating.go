package webui

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/heating"
)

type lastOutdoorReading struct {
	Celsius float64   `json:"celsius"`
	At      time.Time `json:"at"`
}

func (ui *UI) writeExternalHeating(w http.ResponseWriter, ctx context.Context) {
	snapshot := struct {
		heating.Status
		LastOutdoorReading *lastOutdoorReading `json:"lastOutdoorReading,omitempty"`
	}{Status: ui.heating.Snapshot()}
	if sensor := snapshot.Settings.OutdoorSensor; sensor != "" && snapshot.OutdoorTemperature == nil {
		var reading lastOutdoorReading
		var received int64
		now := time.Now()
		err := ui.store.db.QueryRowContext(ctx, `
			SELECT received_ms, celsius FROM outdoor_sensor_readings
			WHERE serial = ? AND sensor = ? AND received_ms BETWEEN ? AND ?
		`, ui.serial, sensor, now.Add(-24*time.Hour).UnixMilli(), now.UnixMilli()).Scan(&received, &reading.Celsius)
		if err == nil {
			reading.At = time.UnixMilli(received)
			snapshot.LastOutdoorReading = &reading
		} else if !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
			ui.logger.Warn("Could not load the last outdoor sensor reading", "error", err)
		}
	}
	writeJSON(w, snapshot)
}

func (ui *UI) getExternalHeating(w http.ResponseWriter, r *http.Request) {
	if ui.heating == nil {
		problem(w, http.StatusServiceUnavailable, "External heating control is unavailable.")
		return
	}
	ui.writeExternalHeating(w, r.Context())
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
	ui.writeHeatingSettingsResult(w, r.Context(), ui.heating.Set(settings))
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
		if _, legacy := patch["enabled"]; legacy {
			for _, name := range []string{"zoneControl", "outdoorControl"} {
				if _, explicit := patch[name]; !explicit {
					delete(fields, name)
				}
			}
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
	ui.writeHeatingSettingsResult(w, r.Context(), err)
}

func (ui *UI) writeHeatingSettingsResult(w http.ResponseWriter, ctx context.Context, err error) {
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
	ui.writeExternalHeating(w, ctx)
}

func (ui *UI) resumeExternalHeating(w http.ResponseWriter, r *http.Request) {
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
	ui.writeExternalHeating(w, r.Context())
}

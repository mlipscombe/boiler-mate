package webui

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

type zonePump struct {
	Zone        string `json:"zone"`
	Temperature string `json:"temperature"`
	Flow        string `json:"flow"`
}

func (ui *UI) getZonePumps(w http.ResponseWriter, r *http.Request) {
	pumps, err := ui.store.zonePumps(r.Context(), ui.serial)
	if err != nil {
		ui.serverError(w, "load zone pump assignments", err)
		return
	}
	writeJSON(w, pumps)
}

func (ui *UI) putZonePump(w http.ResponseWriter, r *http.Request) {
	if ui.heating == nil {
		problem(w, http.StatusServiceUnavailable, "External heating is unavailable.")
		return
	}
	var pump zonePump
	if !readJSON(w, r, &pump) {
		return
	}
	if len(pump.Zone) == 0 || len(pump.Zone) > 512 || strings.ContainsRune(pump.Zone, 0) {
		problem(w, http.StatusUnprocessableEntity, "Choose an external heating zone.")
		return
	}
	if pump.Temperature != "" || pump.Flow != "" {
		heating := ui.heating.Snapshot()
		known := slices.Contains(heating.Settings.Thermostats, pump.Zone)
		for _, input := range heating.Inputs {
			known = known || input.Kind == "thermostat" && input.ID == pump.Zone
		}
		if !known {
			problem(w, http.StatusUnprocessableEntity, "Choose an external heating thermostat.")
			return
		}
		ui.mu.RLock()
		temperature, flow := ui.haSignals[pump.Temperature], ui.haSignals[pump.Flow]
		ui.mu.RUnlock()
		if (pump.Temperature != "" && temperature.Unit != "°C") || (pump.Flow != "" && flow.Unit != "m³/h") {
			problem(w, http.StatusUnprocessableEntity, "Choose a Home Assistant temperature sensor, flow sensor, or both.")
			return
		}
	}
	if err := ui.store.saveZonePump(r.Context(), ui.serial, pump); err != nil {
		ui.serverError(w, "save zone pump assignment", err)
		return
	}
	writeJSON(w, pump)
}

func (s *store) zonePumps(ctx context.Context, serial string) ([]zonePump, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT zone, temperature, flow FROM zone_pumps WHERE serial = ? ORDER BY zone`, serial)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	pumps := []zonePump{}
	for rows.Next() {
		var pump zonePump
		if err := rows.Scan(&pump.Zone, &pump.Temperature, &pump.Flow); err != nil {
			return nil, err
		}
		pumps = append(pumps, pump)
	}
	return pumps, rows.Err()
}

func (s *store) saveZonePump(ctx context.Context, serial string, pump zonePump) error {
	if pump.Temperature == "" && pump.Flow == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM zone_pumps WHERE serial = ? AND zone = ?`, serial, pump.Zone)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO zone_pumps(serial, zone, temperature, flow) VALUES (?, ?, ?, ?)
		ON CONFLICT(serial, zone) DO UPDATE SET temperature = excluded.temperature, flow = excluded.flow
	`, serial, pump.Zone, pump.Temperature, pump.Flow)
	return err
}

package webui

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/efficiency"
)

func (ui *UI) getEfficiency(w http.ResponseWriter, _ *http.Request) {
	ui.mu.RLock()
	report := ui.efficiency
	ui.mu.RUnlock()
	writeJSON(w, report)
}

func (ui *UI) recordEfficiency(ctx context.Context, now time.Time, operating map[string]any) {
	ui.mu.RLock()
	regulation, diagnostics, boiler := ui.snapshots["regulation"], ui.snapshots["advanced_data"], ui.snapshots["boiler"]
	ui.mu.RUnlock()

	state, _ := number(operating["state"])
	substate, _ := number(operating["substate"])
	valve, _ := number(operating["dhw_valve_state"])
	water, waterOK := readingIn(operating, "boiler_temp", 0, 120)
	target, targetOK := readingIn(operating, "boiler_ref", 1, 120)
	output, outputOK := readingIn(operating, "power_pct", 0, 100)
	sample := efficiency.Sample{
		At: now, Session: ui.efficiencySession, State: int(state), Water: water, Output: output,
		Load: efficiency.Load{Target: target},
	}
	if ui.heating != nil {
		demand := ui.heating.AdvisoryDemand(now)
		sample.Load.Demand, sample.Zones, sample.Connection = demand.Key, demand.Zones, demand.Epoch
	}
	if freshObservation(regulation, now, 25*time.Second) {
		minimum, minOK := readingIn(regulation.Values, "boiler_power_min", 1, 100)
		maximum, maxOK := readingIn(regulation.Values, "boiler_power_max", 1, 100)
		p, pOK := readingIn(regulation.Values, "boiler_gain_p", 0, 100)
		i, iOK := readingIn(regulation.Values, "boiler_gain_i", 0, 100)
		sample.Load.Minimum, sample.Load.Maximum = minimum, maximum
		sample.Load.Proportional, sample.Load.Integral = p, i
		sample.Load.Ramp, _ = readingIn(regulation.Values, "power_per_minute", 0, 100)
		sample.Load.ConfiguredGains = pOK && iOK
		sample.Suitable = sample.Load.Demand != "" && waterOK && targetOK && outputOK &&
			minOK && maxOK && minimum <= maximum && (substate == 0 || substate == 21) && valve == 0
	}
	if freshObservation(boiler, now, 25*time.Second) {
		sample.Load.StopDifferential, _ = readingIn(boiler.Values, "diff_over", 0, 40)
	}
	if sample.Suitable && sample.State == efficiency.StateFiring && sample.Load.ConfiguredGains &&
		freshObservation(diagnostics, now, 12*time.Second) {
		p, pOK := readingIn(diagnostics.Values, "boiler_part_p", -1000, 1000)
		i, iOK := readingIn(diagnostics.Values, "boiler_part_i", -1000, 1000)
		effectiveP, hasEffectiveP := readingIn(diagnostics.Values, "boiler_gain_p", 0, 100)
		effectiveI, hasEffectiveI := readingIn(diagnostics.Values, "boiler_gain_i", 0, 100)
		usingConfiguredGains := (!hasEffectiveP || math.Abs(effectiveP-sample.Load.Proportional) <= 0.05) &&
			(!hasEffectiveI || math.Abs(effectiveI-sample.Load.Integral) <= 0.05)
		if pOK && iOK && usingConfiguredGains {
			sample.PI = &efficiency.PIReading{Proportional: p, Integral: i}
		}
	}
	data, err := json.Marshal(sample)
	if err == nil {
		_, err = ui.store.db.ExecContext(ctx, `
			INSERT INTO efficiency_samples(serial, time, observation) VALUES (?, ?, ?)
			ON CONFLICT(serial, time) DO UPDATE SET observation = excluded.observation
		`, ui.serial, now.UnixMilli(), data)
	}
	if err != nil && ctx.Err() == nil {
		ui.logger.Debug("Could not save efficiency observation", "error", err)
	}
}

func freshObservation(observation snapshot, now time.Time, within time.Duration) bool {
	age := now.Sub(observation.At)
	// Other controller polls may land during collection of the operating frame.
	return !observation.At.IsZero() && age >= -time.Second && age <= within
}

func readingIn(values map[string]any, key string, min, max float64) (float64, bool) {
	value, ok := number(values[key])
	if !ok || math.IsNaN(value) || value < min || value > max {
		return 0, false
	}
	return value, true
}

func (ui *UI) runEfficiency(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		now := time.Now()
		report, err := ui.store.efficiencyReport(ctx, ui.serial, now)
		if err == nil {
			ui.mu.Lock()
			ui.efficiency = report
			ui.mu.Unlock()
		} else if ctx.Err() == nil {
			ui.logger.Warn("Could not evaluate efficiency suggestions", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *store) efficiencyReport(ctx context.Context, serial string, now time.Time) (efficiency.Report, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT time, observation FROM efficiency_samples
		WHERE serial = ? AND time BETWEEN ? AND ? ORDER BY time
	`, serial, now.Add(-efficiency.Lookback).UnixMilli(), now.UnixMilli())
	if err != nil {
		return efficiency.Report{}, err
	}
	defer func() { _ = rows.Close() }()
	analyzer := efficiency.NewAnalyzer()
	for rows.Next() {
		var timestamp int64
		var data []byte
		if err := rows.Scan(&timestamp, &data); err != nil {
			return efficiency.Report{}, err
		}
		sample := efficiency.Sample{At: time.UnixMilli(timestamp)}
		if err := json.Unmarshal(data, &sample); err != nil {
			// A damaged sample is a gap in the evidence.
			analyzer.Observe(efficiency.Sample{At: sample.At})
			continue
		}
		analyzer.Observe(sample)
	}
	return analyzer.Report(), rows.Err()
}

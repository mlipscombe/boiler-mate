package webui

import (
	"context"
	"strings"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/hass"
)

const homeAssistantGroup = "home_assistant"

type sensorLabel struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}

func (ui *UI) ObserveHomeAssistant(ctx context.Context, readings []hass.Reading) {
	now := time.Now()
	minute := now.Truncate(time.Minute)
	values := make(map[string]any)
	samples := make(map[string]any)
	ui.mu.Lock()
	if ui.haSignals == nil {
		ui.haSignals = make(map[string]sensorLabel)
	}
	for _, reading := range readings {
		if !strings.HasPrefix(reading.ID, "sensor.") {
			continue
		}
		var prefix string
		switch reading.Unit {
		case "°C":
			prefix = "t_"
		case "m³/h":
			prefix = "f_"
		default:
			continue
		}
		key := prefix + strings.TrimPrefix(reading.ID, "sensor.")
		series := homeAssistantGroup + "." + key
		name := reading.Name
		if name == "" {
			name = ui.haSignals[series].Name
		}
		if name == "" {
			name = reading.ID
		}
		ui.haSignals[series] = sensorLabel{Name: name, Unit: reading.Unit}
		if reading.HasValue {
			values[key] = reading.Value
			if ui.lastSample[series] != minute {
				samples[key] = reading.Value
			}
		}
	}
	ui.snapshots[homeAssistantGroup] = snapshot{At: now, Values: values}
	ui.signalChange()
	ui.mu.Unlock()
	if len(samples) == 0 {
		return
	}
	if err := ui.store.record(ctx, homeAssistantGroup, now, samples); err != nil {
		if ctx.Err() == nil {
			ui.logger.Warn("Could not save Home Assistant telemetry", "error", err)
		}
		return
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	for key := range samples {
		ui.lastSample[homeAssistantGroup+"."+key] = minute
	}
	ui.historyRev++
	ui.signalChange()
}

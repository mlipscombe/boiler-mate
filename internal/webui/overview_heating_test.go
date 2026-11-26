package webui

import (
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/heating"
)

func TestOverviewShowsConfiguredZonesAndPrefersLocalOutdoorTemperature(t *testing.T) {
	const zone = "homeassistant/climate/downstairs/config"
	const local = "homeassistant/sensor/outside/config"
	dir := t.TempDir()
	control := heating.New(dir, "TEST", slog.Default(), heating.Options{DashboardReadings: true})
	settings := control.Snapshot().Settings
	settings.Thermostats, settings.OutdoorSensor, settings.Unit = []string{zone}, local, "F"
	if err := control.Set(settings); err != nil {
		t.Fatal(err)
	}
	control.Receive(zone, []byte(`{"name":"Downstairs","act_t":"downstairs/action","curr_temp_t":"downstairs/temperature","temp_unit":"C"}`), true)
	control.Receive("downstairs/action", []byte("heating"), false)
	control.Receive("downstairs/temperature", []byte("21"), false)
	control.Receive("homeassistant/climate/office/config", []byte(`{"name":"Office","act_t":"office/action"}`), true)
	control.Receive("office/action", []byte("idle"), false)
	control.Receive(local, []byte(`{"dev_cla":"temperature","unit_of_meas":"°C","stat_t":"outside","avty_t":"outside/status"}`), true)
	control.Receive("outside/status", []byte("online"), true)
	control.Receive("outside", []byte("3"), false)

	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, Options{Heating: control, OutdoorWeatherFahrenheit: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	ui.snapshots["advanced_data"] = snapshot{At: time.Now(), Values: map[string]any{"outdoor_temp": int64(50)}}
	status, _ := ui.statusSnapshot(true)
	inputs := status.Heating
	if inputs == nil || len(inputs.Thermostats) != 1 || !inputs.HasLocalOutdoorSensor || inputs.Unit != "F" {
		t.Fatalf("wrong heating summary: %+v", inputs)
	}
	downstairs := inputs.Thermostats[0]
	if downstairs.Name != "Downstairs" || downstairs.Action != "heating" || downstairs.Temperature == nil ||
		math.Abs(*downstairs.Temperature-69.8) > 0.001 {
		t.Fatalf("wrong thermostat reading: %+v", downstairs)
	}
	if inputs.OutdoorSource != "local" || inputs.OutdoorTemperature == nil || math.Abs(*inputs.OutdoorTemperature-37.4) > 0.001 {
		t.Fatalf("local sensor was not preferred: %+v", inputs)
	}
	control.Receive("outside/status", []byte("offline"), false)
	status, _ = ui.statusSnapshot(true)
	if fallback := status.Heating; fallback.OutdoorSource != "remote" || fallback.OutdoorTemperature == nil || *fallback.OutdoorTemperature != 50 {
		t.Fatalf("missing fresh fallback: %+v", fallback)
	}
	ui.snapshots["advanced_data"] = snapshot{At: time.Now().Add(-time.Minute), Values: map[string]any{"outdoor_temp": int64(50)}}
	status, _ = ui.statusSnapshot(true)
	if status.Heating.OutdoorTemperature != nil {
		t.Fatal("stale controller weather was shown")
	}
}

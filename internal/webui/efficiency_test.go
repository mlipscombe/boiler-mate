package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/efficiency"
	"github.com/mlipscombe/boiler-mate/internal/heating"
)

func TestEfficiencyReportUsesPersistedThermostatAndControllerObservations(t *testing.T) {
	dir := t.TempDir()
	const node = "zwave/Office/Thermostat"
	const action = node + "/thermostat_operating_state/endpoint_0/state"
	const gateway = "zwave/_CLIENTS/ZWAVE_GATEWAY-test/status"
	external := heating.New(dir, "TEST", slog.Default(), heating.Options{})
	settings := external.Snapshot().Settings
	settings.Thermostats = []string{action}
	if err := external.Set(settings); err != nil {
		t.Fatal(err)
	}

	at := time.Now().Add(-30 * time.Minute)
	call := []byte(fmt.Sprintf(`{"value":1,"time":%d}`, at.Add(-time.Hour).UnixMilli()))
	external.Receive(gateway, []byte("true"), true)
	external.Receive(action, call, true)
	external.Receive(gateway, []byte("true"), true)
	external.Receive(node+"/status", []byte("true"), true)
	external.Receive("zwave/driver/status", []byte("true"), true)
	if demand := external.AdvisoryDemand(at); demand.Key == "" || demand.Zones != "Thermostat" {
		t.Fatalf("expected timestamped retained call with online source, got %+v", demand)
	}

	var writes int
	opts := Options{
		Heating: external,
		Write: func(context.Context, string, []byte) error {
			writes++
			return nil
		},
	}
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	add := func(state, water, output int64) {
		t.Helper()
		at = at.Add(5 * time.Second)
		ui.snapshots["regulation"] = snapshot{at, map[string]any{"boiler_power_min": int64(10), "boiler_power_max": int64(90)}}
		ui.recordEfficiency(t.Context(), at, map[string]any{"state": state, "boiler_temp": water, "boiler_ref": int64(80), "power_pct": output})
	}
	for range 3 {
		add(2, 70, 0)
		for range 40 {
			add(5, 83, 32)
		}
		add(9, 92, 0)
	}
	if err := ui.Close(); err != nil {
		t.Fatal(err)
	}

	ui, err = New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Close()
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ui.runEfficiency(ctx)
	}()
	defer func() {
		cancel()
		<-stopped
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		response := httptest.NewRecorder()
		ui.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/efficiency", nil))
		var report efficiency.Report
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || response.Code != http.StatusOK {
			t.Fatalf("response %d %s: %v", response.Code, response.Body.String(), err)
		}
		if report.LastSample != nil {
			if len(report.Suggestions) != 1 || report.Suggestions[0].Rule != "zone-output" || report.ObservedRuns != 3 {
				t.Fatalf("incorrect persisted report: %+v", report)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background report was not published")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if writes != 0 {
		t.Fatalf("efficiency monitoring wrote %d setting changes", writes)
	}
}

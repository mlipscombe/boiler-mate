package webui

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mlipscombe/boiler-mate/internal/heating"
)

func TestExternalHeatingSettingsPatch(t *testing.T) {
	dir := t.TempDir()
	control := heating.New(dir, "TEST", slog.Default(), heating.Options{
		Write: func(context.Context, string, int, func() bool) error { return nil },
	})
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, Options{Heating: control})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	handler := ui.Handler()
	patch := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/external-heating", strings.NewReader(body))
		req.Header.Set("X-Boiler-Mate", "1")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	responses := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		responses <- patch(`{"zoneControl":true,"thermostats":["ha:climate.downstairs"],"dropoutSeconds":240}`)
	}()
	go func() {
		responses <- patch(`{"outdoorControl":true,"outdoorSensor":"ha:sensor.outdoor","stopAt":17,"bands":[{"from":null,"maxPower":80}]}`)
	}()
	for range 2 {
		if res := <-responses; res.Code != http.StatusOK {
			t.Fatalf("save settings: %d %s", res.Code, res.Body)
		}
	}

	settings := control.Snapshot().Settings
	if !settings.ZoneControl || !settings.OutdoorControl ||
		len(settings.Thermostats) != 1 || settings.Thermostats[0] != "ha:climate.downstairs" || settings.DropoutSeconds != 240 ||
		settings.OutdoorSensor != "ha:sensor.outdoor" || settings.StopAt == nil || *settings.StopAt != 17 ||
		len(settings.Bands) != 1 || settings.Bands[0].MaxPower != 80 || settings.Unit != "C" {
		t.Fatalf("concurrent patches lost settings: %+v", settings)
	}

	if res := patch(`{"stopAt":null}`); res.Code != http.StatusOK || control.Snapshot().Settings.StopAt != nil {
		t.Fatalf("clear cutoff: %d %s", res.Code, res.Body)
	}
	if res := patch(`{"outdoorSensor":"","bands":[{"maxPower":70}]}`); res.Code != http.StatusUnprocessableEntity || control.Snapshot().Settings.OutdoorSensor != "ha:sensor.outdoor" {
		t.Fatalf("invalid outdoor settings were accepted: %d %s", res.Code, res.Body)
	}
	if res := patch(`{"dropoutSeconds":300,"unexpected":true}`); res.Code != http.StatusBadRequest || control.Snapshot().Settings.DropoutSeconds != 240 {
		t.Fatalf("unexpected field was accepted: %d %s", res.Code, res.Body)
	}
	if res := patch(`{"zoneControl":false}`); res.Code != http.StatusOK {
		t.Fatalf("disable zones: %d %s", res.Code, res.Body)
	}
	if settings := control.Snapshot().Settings; settings.ZoneControl || !settings.OutdoorControl {
		t.Fatalf("zone change affected outdoor control: %+v", settings)
	}
	if res := patch(`{"enabled":false}`); res.Code != http.StatusOK || control.Enabled() {
		t.Fatalf("legacy disable: %d %s", res.Code, res.Body)
	}
}

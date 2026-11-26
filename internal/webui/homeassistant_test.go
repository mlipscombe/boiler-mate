package webui

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/hass"
)

func TestChartPumpAlongsideBoiler(t *testing.T) {
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	ui.Observe(t.Context(), "operating_data", map[string]any{"boiler_temp": int64(80), "power_pct": int64(47)})
	at := time.Now()
	ui.ObserveHomeAssistant(t.Context(), []hass.Reading{
		{ID: "sensor.media_temperature", Name: "Pump media temperature", Unit: "°C", Available: true, HasValue: true, Value: 48.4, At: at},
		{ID: "sensor.flow", Name: "Pump flow", Unit: "m³/h", Available: true, HasValue: true, Value: 2.78, At: at},
	})
	status, _ := ui.statusSnapshot(true)
	if label := status.HASignals["home_assistant.f_flow"]; label.Name != "Pump flow" || label.Unit != "m³/h" {
		t.Fatalf("missing chart metadata: %+v", status.HASignals)
	}
	ui.ObserveHomeAssistant(t.Context(), []hass.Reading{{ID: "sensor.media_temperature"}, {ID: "sensor.flow"}})
	status, _ = ui.statusSnapshot(true)
	if len(status.Data[homeAssistantGroup].Values) != 0 || len(status.HASignals) != 2 {
		t.Fatalf("failed reads should clear live values and preserve labels: %+v", status)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/history?series=operating_data.boiler_temp&series=operating_data.power_pct&series=home_assistant.t_media_temperature&series=home_assistant.f_flow", nil)
	ui.Handler().ServeHTTP(recorder, req)
	var result map[string][]point
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); recorder.Code != http.StatusOK || err != nil {
		t.Fatalf("history response %d: %s", recorder.Code, recorder.Body)
	}
	want := map[string]float64{"home_assistant.t_media_temperature": 48.4, "home_assistant.f_flow": 2.78,
		"operating_data.boiler_temp": 80, "operating_data.power_pct": 47}
	for key, value := range want {
		if points := result[key]; len(points) != 1 || points[0].Value != value {
			t.Errorf("history for %s = %+v, want %v", key, points, value)
		}
	}
}

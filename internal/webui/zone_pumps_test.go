package webui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/hass"
	"github.com/mlipscombe/boiler-mate/internal/heating"
)

func TestZonePumpAssignmentDoesNotChangeHeatingControl(t *testing.T) {
	dir := t.TempDir()
	control := heating.New(dir, "TEST", slog.Default(), heating.Options{DashboardReadings: true})
	const zone = "homeassistant/climate/downstairs/config"
	control.Receive(zone, []byte(`{"act_t":"downstairs/action","name":"Downstairs"}`), true)
	before := control.Snapshot()
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, Options{Heating: control})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	ui.ObserveHomeAssistant(t.Context(), []hass.Reading{
		{ID: "sensor.pump_temperature", Unit: "°C", HasValue: true, Value: 48, At: time.Now()},
		{ID: "sensor.pump_flow", Unit: "m³/h", HasValue: true, Value: 2.8, At: time.Now()},
	})
	handler := ui.Handler()
	request := func(method string, assignment zonePump) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(assignment)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/external-heating/pumps", bytes.NewReader(body))
		req.Header.Set("X-Boiler-Mate", "1")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	pump := zonePump{Zone: zone, Temperature: "home_assistant.t_pump_temperature", Flow: "home_assistant.f_pump_flow"}
	if response := request(http.MethodPut, pump); response.Code != http.StatusOK {
		t.Fatalf("save assignment: %d %s", response.Code, response.Body)
	}
	if response := request(http.MethodPut, zonePump{Zone: zone, Temperature: pump.Flow}); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accepts flow as temperature: %d %s", response.Code, response.Body)
	}
	response := request(http.MethodGet, zonePump{})
	loaded := []zonePump{}
	if err := json.Unmarshal(response.Body.Bytes(), &loaded); response.Code != http.StatusOK || err != nil || len(loaded) != 1 || loaded[0] != pump {
		t.Fatalf("read assignment: %d %s", response.Code, response.Body)
	}
	if response := request(http.MethodPut, zonePump{Zone: zone}); response.Code != http.StatusOK {
		t.Fatalf("remove assignment: %d %s", response.Code, response.Body)
	}
	if after := control.Snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatalf("pump assignment changed heating control: before %+v, after %+v", before, after)
	}
}

package hass

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestReadPumpAndThermostatStates(t *testing.T) {
	const temperatureID = "sensor.basement_downstairs_radiator_pump_media_temperature"
	const flowID = "sensor.office_radiator_pumps_flow"
	const zoneID = "climate.hallway"
	const missingID = "climate.missing"
	const modeID = "climate.mode_only"
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Home Assistant redirected an entity read to another server")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected Home Assistant request %s %s", r.Method, r.URL.Path)
		}
		var body string
		switch r.URL.Path {
		case "/api/config":
			body = `{"unit_system":{"temperature":"°F"}}`
		case "/api/states/" + temperatureID:
			body = `{"entity_id":"` + temperatureID + `","state":"119.109497070312","attributes":{"unit_of_measurement":"°F"}}`
		case "/api/states/" + flowID:
			body = `{"entity_id":"` + flowID + `","state":"2.78216433525085","attributes":{"unit_of_measurement":"m³/h"}}`
		case "/api/states/" + zoneID:
			body = `{"entity_id":"` + zoneID + `","state":"heat","attributes":{"hvac_action":"idle","current_temperature":68}}`
		case "/api/states/" + modeID:
			body = `{"entity_id":"` + modeID + `","state":"heat","attributes":{}}`
		case "/api/states/" + missingID:
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		default:
			t.Errorf("read unlisted entity %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result := make(chan []Reading, 1)
	go Run(ctx, Config{URL: base, Token: "test-token", Entities: []string{temperatureID, flowID, zoneID, missingID, modeID}},
		slog.Default(), func(readings []Reading) { result <- readings })
	select {
	case readings := <-result:
		temp, flow, zone, missing, mode := readings[0], readings[1], readings[2], readings[3], readings[4]
		if !temp.HasValue || temp.Unit != "°C" || math.Abs(temp.Value-48.394165) > .00001 {
			t.Errorf("pump temperature = %+v", temp)
		}
		if !flow.HasValue || flow.Unit != "m³/h" || math.Abs(flow.Value-2.782164) > .00001 {
			t.Errorf("pump flow = %+v", flow)
		}
		if zone.Action != "idle" || !zone.HasValue || zone.Value != 20 {
			t.Errorf("zone = %+v", zone)
		}
		if missing.ID != missingID || missing.Available || mode.Action != "" {
			t.Errorf("missing or mode-only thermostat was interpreted as demand: %+v %+v", missing, mode)
		}
	case <-ctx.Done():
		t.Fatal("did not receive Home Assistant states")
	}
}

package webui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
)

type readOnlyFixture struct{}

func (readOnlyFixture) Get(_ context.Context, function nbe.Function, path string) (*nbe.NBEResponse, error) {
	switch function {
	case nbe.GetSetupFunction:
		payload := map[string]any{"temp": int64(60)}
		switch path {
		case "alarm.*", "alarm.output":
			payload = map[string]any{"output": int64(0)}
		case "hopper.*":
			payload = map[string]any{"trip2": int64(0), "fixed_counter": int64(0)}
		case "hopper.trip2":
			payload = map[string]any{"trip2": int64(0)}
		case "misc.expansion_module":
			payload = map[string]any{"expansion_module": int64(0)}
		}
		return &nbe.NBEResponse{Payload: payload}, nil
	case nbe.GetSetupRangeFunction:
		key, maximum, defaultValue := "temp", int64(85), int64(60)
		switch {
		case strings.HasPrefix(path, "alarm."):
			key, maximum, defaultValue = "output", 20, 0
		case strings.HasPrefix(path, "hopper."):
			key, maximum, defaultValue = "trip2", 0, 0
		}
		record := map[string]any{"min": int64(0), "max": maximum, "default": defaultValue, "decimals": int64(0)}
		payload := map[string]any{key: record}
		if path == "hopper.*" {
			payload["fixed_counter"] = record
		}
		return &nbe.NBEResponse{Payload: payload}, nil
	case nbe.GetConsumptionDataFunction:
		return &nbe.NBEResponse{Payload: map[string]any{path: "0.0,1.2,2.3"}}, nil
	default:
		return nil, fmt.Errorf("unsupported fixture request %d", function)
	}
}

func TestControllerAPIReadOnlyChecks(t *testing.T) {
	write := func(_ context.Context, _ string, _ []byte) error {
		t.Fatal("the read-only verification attempted to write a controller setting")
		return nil
	}
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), write)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	ui.Observe(t.Context(), "operating_data", map[string]any{"boiler_temp": nbe.RoundedFloat(42.5), "dhw_temp": nbe.RoundedFloat(999.9)})
	ui.Observe(t.Context(), "misc", map[string]any{"xtea_key": "LOCAL_TEST_SECRET"})
	ui.Observe(t.Context(), "pump", map[string]any{"output": int64(5)})
	handler := ui.Handler()
	for _, tc := range []struct {
		method, path, body string
		crossSite, header  bool
		want               int
	}{
		{"GET", "/api/status", "", false, false, 200},
		{"GET", "/api/settings/boiler", "", false, false, 200},
		{"GET", "/api/settings/alarm", "", false, false, 200},
		{"GET", "/api/settings/hopper", "", false, false, 200},
		{"GET", "/api/consumption?period=days", "", false, false, 200},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"77"}`, false, false, 403},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"78"}`, true, true, 403},
		{"PUT", "/api/settings/boiler/temp", `{"current":"59","value":"77"}`, false, true, 409},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"900"}`, false, true, 422},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"60"}`, false, true, 204},
		{"PUT", "/api/settings/alarm/output", `{"current":"0","value":"4"}`, false, true, 422},
		{"PUT", "/api/settings/alarm/output", `{"current":"0","value":"14"}`, false, true, 422},
		{"PUT", "/api/settings/hopper/trip2", `{"current":"0","value":"30001"}`, false, true, 422},
		{"POST", "/api/power", `{"action":"anything else"}`, false, true, 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.crossSite {
				request.Header.Set("Sec-Fetch-Site", "cross-site")
			}
			if tc.header {
				request.Header.Set("X-Boiler-Mate", "1")
			}
			if tc.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			result := httptest.NewRecorder()
			handler.ServeHTTP(result, request)
			if result.Code != tc.want || strings.Contains(result.Body.String(), "LOCAL_TEST_SECRET") {
				t.Fatalf("status %d body %s, want %d", result.Code, result.Body.String(), tc.want)
			}
			if tc.path == "/api/status" && !strings.Contains(result.Body.String(), `"control":true`) {
				t.Fatalf("controller writes were disabled: %s", result.Body.String())
			}
			if tc.path == "/api/settings/boiler" {
				var data []field
				if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil || len(data) != 1 || data[0].Range == nil {
					t.Fatalf("missing setting metadata: %+v, %v", data, err)
				}
			}
			if tc.path == "/api/settings/alarm" {
				var data []field
				if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil || len(data) != 1 {
					t.Fatalf("missing alarm settings: %+v, %v", data, err)
				}
				options := data[0].Options
				if len(options) != 10 || options[1] != (settingOption{Value: "5", Label: "Output L5 (Output pump)"}) || options[9].Value != "13" {
					t.Fatalf("incorrect alarm relay options: %+v", options)
				}
			}
			if tc.path == "/api/settings/hopper" {
				var data []field
				if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil || len(data) != 2 || !data[0].Fixed || data[0].Range != nil ||
					data[1].Key != "trip2" || data[1].Fixed || data[1].Range == nil || data[1].Range.Max != 30000 {
					t.Fatalf("expected only Trip 2 to use the fallback range: %+v, %v", data, err)
				}
			}
		})
	}
	from, to := time.Now().Add(-time.Hour).Unix(), time.Now().Unix()
	points, err := ui.store.history(t.Context(), "operating_data.boiler_temp", from, to)
	if err != nil || len(points) != 1 || points[0].Value != 42.5 {
		t.Fatalf("stored history = %v, %v", points, err)
	}
	points, err = ui.store.history(t.Context(), "operating_data.dhw_temp", from, to)
	if err != nil || len(points) != 0 {
		t.Fatalf("disconnected sensor was recorded: %v, %v", points, err)
	}
}

type trip2Fixture struct {
	readOnlyFixture
	value int64
}

func (fixture *trip2Fixture) Get(ctx context.Context, function nbe.Function, path string) (*nbe.NBEResponse, error) {
	if function == nbe.GetSetupFunction && path == "hopper.trip2" {
		return &nbe.NBEResponse{Payload: map[string]any{"trip2": fixture.value}}, nil
	}
	return fixture.readOnlyFixture.Get(ctx, function, path)
}

func TestTrip2VerifiesControllerReadback(t *testing.T) {
	for _, check := range []struct {
		name  string
		value int64
		want  int
	}{
		{"change confirmed", 123, http.StatusNoContent},
		{"value still reads zero", 0, http.StatusConflict},
	} {
		t.Run(check.name, func(t *testing.T) {
			fixture := &trip2Fixture{}
			writes := 0
			write := func(_ context.Context, key string, value []byte) error {
				if key != "hopper.trip2" || string(value) != "123" {
					t.Fatalf("unexpected simulated write %q = %q", key, value)
				}
				writes++
				fixture.value = check.value
				return nil
			}
			ui, err := New(t.Context(), slog.Default(), fixture, "TEST", t.TempDir(), write)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ui.Close() })
			ui.Observe(t.Context(), "operating_data", map[string]any{"boiler_temp": int64(60)})
			ui.Observe(t.Context(), "hopper", map[string]any{"trip2": int64(0)})
			request := httptest.NewRequest(http.MethodPut, "/api/settings/hopper/trip2", strings.NewReader(`{"current":"0","value":"123"}`))
			request.Header.Set("X-Boiler-Mate", "1")
			request.Header.Set("Content-Type", "application/json")
			result := httptest.NewRecorder()
			ui.Handler().ServeHTTP(result, request)
			if result.Code != check.want || writes != 1 || !ui.snapshots["hopper"].At.IsZero() {
				t.Fatalf("status %d, writes %d, body %s", result.Code, writes, result.Body.String())
			}
		})
	}
}

func TestVendorOptionsRespectControllerRange(t *testing.T) {
	oneBit := &settingRange{Max: 1}
	if options := settingOptionKind("boiler.timer", oneBit).options(oneBit, false, nil); len(options) != 2 || options[0].Label != "Off" || options[1].Label != "On" {
		t.Fatalf("incorrect Boolean options: %+v", options)
	}
	if options := settingOptionKind("hopper.distance_sensor", oneBit).options(oneBit, false, nil); len(options) != 3 || options[0].Disabled || options[1].Disabled || !options[2].Disabled {
		t.Fatalf("incorrect distance sensor options: %+v", options)
	}
	if options := settingOptionKind("weather.input_reference", nil).options(nil, true, nil); len(options) != 21 || options[0].Label != "www" || options[20].Value != "20" {
		t.Fatalf("incorrect expansion-board input options: %+v", options)
	}
}

func TestStatusStream(t *testing.T) {
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	ui.Observe(t.Context(), "misc", map[string]any{"clock_hour": int64(12)})
	ui.Observe(t.Context(), "operating_data", map[string]any{"boiler_temp": int64(42)})
	server := httptest.NewUnstartedServer(ui.Handler())
	server.Config.WriteTimeout = 200 * time.Millisecond
	server.Start()
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Get(server.URL + "/api/status/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected stream response: %s %v", response.Status, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	readEvent := func(want float64) {
		t.Helper()
		line, err := reader.ReadString('\n')
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "data: ")
		if err != nil || !ok {
			t.Fatalf("could not read stream event: %q %v", line, err)
		}
		var status dashboardStatus
		if err := json.Unmarshal([]byte(value), &status); err != nil {
			t.Fatal(err)
		}
		if got := status.Data["operating_data"].Values["boiler_temp"]; got != want || !status.Online || status.Control || len(status.Data) != 1 {
			t.Fatalf("unexpected dashboard status: %+v", status)
		}
		if separator, err := reader.ReadString('\n'); separator != "\n" || err != nil {
			t.Fatalf("invalid event separator %q: %v", separator, err)
		}
	}
	readEvent(42)
	time.Sleep(250 * time.Millisecond)
	ui.Observe(t.Context(), "operating_data", map[string]any{"boiler_temp": int64(43)})
	readEvent(43)
}

func TestTimerRange(t *testing.T) {
	bounds := &settingRange{Max: 111111111111}
	for _, check := range []struct {
		value string
		want  bool
	}{{"000010010011", true}, {"101", true}, {"2", false}, {"0.1", false}, {"-1", false}} {
		if got := validSettingValue(check.value, bounds, true); got != check.want {
			t.Errorf("timer %q = %v, want %v", check.value, got, check.want)
		}
	}
}

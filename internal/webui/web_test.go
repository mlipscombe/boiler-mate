package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
		return &nbe.NBEResponse{Payload: map[string]any{"temp": int64(60)}}, nil
	case nbe.GetSetupRangeFunction:
		return &nbe.NBEResponse{Payload: map[string]any{"temp": map[string]any{"min": int64(0), "max": int64(85), "default": int64(60), "decimals": int64(0)}}}, nil
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
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), "test-password", write)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	ui.Observe(t.Context(), "operating_data", map[string]any{"boiler_temp": nbe.RoundedFloat(42.5), "dhw_temp": nbe.RoundedFloat(999.9)})
	ui.Observe(t.Context(), "misc", map[string]any{"xtea_key": "LOCAL_TEST_SECRET"})
	handler := ui.Handler()
	for _, tc := range []struct {
		method, path, body string
		auth, header       bool
		want               int
	}{
		{"GET", "/api/status", "", false, false, 401},
		{"GET", "/api/status", "", true, false, 200},
		{"GET", "/api/settings/boiler", "", true, false, 200},
		{"GET", "/api/consumption?period=days", "", true, false, 200},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"77"}`, true, false, 403},
		{"PUT", "/api/settings/boiler/temp", `{"current":"59","value":"77"}`, true, true, 409},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"900"}`, true, true, 422},
		{"PUT", "/api/settings/boiler/temp", `{"current":"60","value":"60"}`, true, true, 204},
		{"POST", "/api/power", `{"action":"anything else"}`, true, true, 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.auth {
				request.SetBasicAuth("boiler", "test-password")
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
			if tc.path == "/api/settings/boiler" {
				var data []field
				if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil || len(data) != 1 || data[0].Range == nil {
					t.Fatalf("missing setting metadata: %+v, %v", data, err)
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

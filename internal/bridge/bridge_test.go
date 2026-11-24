package bridge

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestContracts(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/healthz", `{"healthy":true}`},
		{"/liveness", "OK"},
	} {
		got := httptest.NewRecorder()
		metricsHandler(prometheus.NewRegistry()).ServeHTTP(got, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if got.Code != http.StatusOK || got.Body.String() != tc.want {
			t.Errorf("%s = %d %q", tc.path, got.Code, got.Body.String())
		}
	}
	for _, tc := range []struct{ path, prefix string }{{"/", "nbe/1234"}, {"/home/boiler", "home/boiler"}} {
		if got := mqttPrefix(&url.URL{Path: tc.path}, "1234"); got != tc.prefix {
			t.Errorf("prefix = %q, want %q", got, tc.prefix)
		}
	}
}

func TestPowerCommand(t *testing.T) {
	for _, tc := range []struct{ input, path, value string }{
		{"ON", "misc.start", "1"},
		{"1", "misc.start", "1"},
		{"OFF", "misc.stop", "1"},
		{"0", "misc.stop", "1"},
		{"false", "misc.stop", "1"},
		{"", "", ""},
		{"unknown", "", ""},
	} {
		path, value := translatePower("device.power_switch", []byte(tc.input))
		if path != tc.path || string(value) != tc.value {
			t.Errorf("translation of %q = %q %q", tc.input, path, value)
		}
	}
}

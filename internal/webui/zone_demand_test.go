package webui

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestZoneDemandHistoryPreservesEdgesAndGaps(t *testing.T) {
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	const zone = "zwave/Downstairs/thermostat_operating_state/endpoint_0/state"
	start := time.Now().Add(-time.Minute).Truncate(time.Second)
	at := func(seconds int) time.Time { return start.Add(time.Duration(seconds) * time.Second) }
	record := func(now time.Time) {
		t.Helper()
		if err := ui.flushZoneDemand(t.Context(), now); err != nil {
			t.Fatal(err)
		}
	}

	ui.zoneDemand.observe(at(0), map[string]bool{zone: true})
	record(at(2))
	ui.zoneDemand.observe(at(5), map[string]bool{zone: false})
	ui.zoneDemand.observe(at(9), map[string]bool{})
	ui.zoneDemand.observe(at(15), map[string]bool{zone: true})
	record(at(20))
	// A new recorder cannot extend demand across a restart.
	ui.zoneDemand = newDemandRecorder()
	ui.zoneDemand.observe(at(27), map[string]bool{zone: false})
	record(at(30))

	query := url.Values{"zone": {zone}, "from": {strconv.FormatInt(at(1).Unix(), 10)}}
	req := httptest.NewRequest(http.MethodGet, "/api/external-heating/pumps/demand?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	ui.Handler().ServeHTTP(rec, req)
	var periods []demandPeriod
	want := []demandPeriod{
		{Start: at(1).UnixMilli(), End: at(5).UnixMilli(), Calling: true},
		{Start: at(5).UnixMilli(), End: at(9).UnixMilli()},
		{Start: at(15).UnixMilli(), End: at(20).UnixMilli(), Calling: true},
		{Start: at(27).UnixMilli(), End: at(30).UnixMilli()},
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &periods); err != nil || rec.Code != http.StatusOK || !reflect.DeepEqual(periods, want) {
		t.Fatalf("demand history %d: %s, want %+v", rec.Code, rec.Body, want)
	}
}

package webui

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/mlipscombe/boiler-mate/internal/heating"
)

func TestDegreeWeatherPrefersLocalAndRecordsComparableHours(t *testing.T) {
	dir := t.TempDir()
	external := heating.New(dir, "TEST", slog.Default(), heating.Options{})
	const sensor = "homeassistant/sensor/outside/config"
	settings := external.Snapshot().Settings
	settings.OutdoorSensor, settings.SensorTimeout = sensor, 60
	if err := external.Set(settings); err != nil {
		t.Fatal(err)
	}
	external.Receive(sensor, []byte(`{"dev_cla":"temperature","unit_of_meas":"°C","stat_t":"outside"}`), true)
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir,
		Options{Heating: external, OutdoorWeatherFahrenheit: true})
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Close()
	now := time.Now()
	at := now
	ui.snapshots["advanced_data"] = snapshot{at, map[string]any{"outdoor_temp": int64(50)}}
	ui.recordWeather(t.Context(), at) // 50 °F = 10 °C when local is absent.
	external.Receive("outside", []byte("3"), false)
	ui.recordWeather(t.Context(), at) // Prefer local even in a slot that has remote.
	slot := at.Truncate(5 * time.Minute)
	var source string
	var celsius float64
	if err := ui.store.db.QueryRowContext(t.Context(),
		"SELECT source, celsius FROM weather_observations WHERE time = ?", slot.Unix()).Scan(&source, &celsius); err != nil ||
		source != weatherLocal || celsius != 3 {
		t.Fatalf("selected weather source %q, temperature %g: %v", source, celsius, err)
	}

	next := slot.Add(10*time.Minute + time.Second)
	if err := ui.store.recordPelletCounter(t.Context(), "TEST", next.Add(-time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	ui.snapshots["advanced_data"] = snapshot{next, map[string]any{"outdoor_temp": int64(50)}}
	ui.recordWeather(t.Context(), next) // Stale local sensor falls back.
	before := []weatherPeriod{{From: slot, To: slot.Add(20 * time.Minute)}}
	if err := ui.store.fillWeatherPeriods(t.Context(), "TEST", 15, 1.8, before); err != nil {
		t.Fatal(err)
	}
	period := before[0]
	wantHours := (12.0 + 5.0) * 1.8 / 12
	if period.Source != "mixed" || period.Coverage != .5 || period.FuelCoverage != .25 ||
		math.Abs(wantHours-period.DegreeHours) > 1e-9 || math.Abs(wantHours/24-period.DegreeDays) > 1e-9 {
		t.Fatalf("incorrect mixed-source weather result: %+v", period)
	}
}

func TestDegreeReportSupportsDSTAndMatchesSingleBrand(t *testing.T) {
	local, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// This completed local day has 25 hours.
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, local)
	end := day.AddDate(0, 0, 1)
	if end.Sub(day) != 25*time.Hour {
		t.Fatalf("test needs a 25-hour day, got %s", end.Sub(day))
	}
	dir := t.TempDir()
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer ui.Close()
	ctx := t.Context()
	if _, err := ui.store.db.ExecContext(ctx, `
		INSERT INTO pellet_purchases (id, purchased_at, quantity, weight, unit, brand, price)
		VALUES ('test', ?, 1, 40, 'kg', 'Sample Pellets', 20)
	`, day.Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := ui.store.recordPelletCounter(ctx, "TEST", day, 20); err != nil {
		t.Fatal(err)
	}
	if err := ui.store.recordPelletCounter(ctx, "TEST", end, 35); err != nil {
		t.Fatal(err)
	}
	for at := day; at.Before(end); at = at.Add(5 * time.Minute) {
		if _, err := ui.store.db.ExecContext(ctx, `
			INSERT INTO weather_observations VALUES ('TEST', ?, 5, 'local', 1)
		`, at.Unix()); err != nil {
			t.Fatal(err)
		}
	}
	windows := []weatherPeriod{{From: end, To: end.Add(time.Hour)}, {From: day, To: end}}
	if err := ui.store.fillWeatherPeriods(ctx, "TEST", 15, 1, windows); err != nil {
		t.Fatal(err)
	}
	fuel, err := ui.store.pelletReport(ctx, end.Add(time.Hour), []pelletPeriod{
		{Kind: "day", From: end, To: end.Add(time.Hour)}, {Kind: "day", From: day, To: end},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := combineDegreeFuelReport(windows, fuel, "day", 1)
	row := report.Periods[1]
	if math.Abs(row.DegreeHours-250) > 1e-9 || row.Coverage != 1 || row.FuelCoverage != 1 ||
		row.UsedKg == nil || *row.UsedKg != 15 || row.Cost == nil || *row.Cost != 7.5 ||
		row.KgPerDay == nil || math.Abs(*row.KgPerDay-1.44) > 1e-9 ||
		row.CostPerDay == nil || math.Abs(*row.CostPerDay-.72) > 1e-9 ||
		len(report.Brands) != 1 || report.Brands[0].Name != "Sample Pellets" {
		t.Fatalf("incorrect period or brand allocation: %+v", report)
	}
	windows[1].FuelCoverage = .8
	report = combineDegreeFuelReport(windows, fuel, "day", 1)
	if report.Periods[1].KgPerDay != nil || len(report.Brands) != 0 {
		t.Fatalf("incomplete overlap produced a comparison: %+v", report)
	}
	fuel.Periods[1].BrandUsage = map[string]float64{"Sample Pellets": 10, "Other": 5}
	windows[1].FuelCoverage = 1
	report = combineDegreeFuelReport(windows, fuel, "day", 1)
	if !report.Periods[1].MixedBrands || len(report.Brands) != 0 {
		t.Fatalf("mixed brand interval was attributed to one brand: %+v", report)
	}

	response := httptest.NewRecorder()
	ui.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/efficiency/weather?period=day&base=65&tz=America%2FNew_York", nil))
	var payload degreeReport
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || response.Code != http.StatusOK ||
		payload.Base != 65 || payload.Unit != "F" || len(payload.Periods) != 31 {
		t.Fatalf("report response %d %s: %v", response.Code, response.Body.String(), err)
	}
}

package webui

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPelletLedgerPricesMixedPurchasesAndTracksBrands(t *testing.T) {
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	now := time.Date(2025, time.March, 12, 14, 5, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour)
	price := 20.0
	entry := pelletPurchase{At: hour.Add(-time.Hour), Quantity: 2, Weight: 40, Unit: "lb", Price: &price, Brand: "Cedar"}
	const first = "/api/pellets/purchases/11111111111111111111111111111111"
	requestJSON(t, ui, http.MethodPut, first, entry, http.StatusNoContent)
	requestJSON(t, ui, http.MethodPut, first, entry, http.StatusNoContent)
	price = 9.4
	entry = pelletPurchase{At: hour.Add(time.Minute), Quantity: 1, Weight: 18.8, Unit: "kg", Price: &price, Brand: "Maple"}
	requestJSON(t, ui, http.MethodPut, "/api/pellets/purchases/22222222222222222222222222222222", entry, http.StatusNoContent)
	entry = pelletPurchase{At: hour.Add(4 * time.Minute), Quantity: 1, Weight: 1000, Unit: "kg", Brand: "Maple"}
	requestJSON(t, ui, http.MethodPut, "/api/pellets/purchases/33333333333333333333333333333333", entry, http.StatusNoContent)

	for _, sample := range []struct {
		at time.Time
		kg float64
	}{
		{hour.Add(-time.Minute), 1000},
		{hour.Add(time.Minute), 1040}, // 20 kg before the hour, 20 kg within it.
		{hour.Add(2 * time.Minute), 1058.8},
		{hour.Add(3 * time.Minute), 1058.8},
		{hour.Add(4 * time.Minute), 1058.8},
		{hour.Add(5 * time.Minute), 1060.8}, // Unpriced opening stock.
	} {
		if err := ui.store.recordPelletCounter(t.Context(), ui.serial, sample.at, sample.kg); err != nil {
			t.Fatal(err)
		}
	}
	report, err := ui.store.pelletReport(t.Context(), now, pelletPeriods(now, ""))
	if err != nil {
		t.Fatal(err)
	}
	approx := func(got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("got %v, want %v", got, want)
		}
	}
	if len(report.Purchases) != 3 || len(report.Brands) != 2 || report.RemainingKG == nil {
		t.Fatalf("unexpected ledger: %+v", report)
	}
	cedarKG := 80 * kgPerPound
	approx(*report.RemainingKG, 998)
	approx(report.Periods[0].UsedKG, 40.8)
	approx(report.Periods[0].PricedKG, cedarKG-20+18.8)
	approx(report.Periods[0].Cost, 40-20*40/cedarKG+9.4)
	approx(report.Periods[1].UsedKG, 60.8)
	approx(report.Periods[1].Cost, 49.4)
	approx(report.Brands[0].UsedKG, cedarKG)
	approx(report.Brands[1].UsedKG, 20.8)
	approx(report.Brands[1].Cost, 9.4)
	if report.Brands[0].Name != "Cedar" || report.Brands[1].Name != "Maple" {
		t.Fatalf("unexpected brands: %+v", report.Brands)
	}
	requestJSON(t, ui, http.MethodPut, "/api/pellets/settings",
		pelletSettings{Currency: "CAD", LowUnit: "kg"}, http.StatusConflict)
	requestJSON(t, ui, http.MethodGet, "/api/pellets?tz=America%2FNew_York&period=week", nil, http.StatusOK)
	requestJSON(t, ui, http.MethodGet, "/api/pellets?tz=unknown", nil, http.StatusBadRequest)
}

func TestPelletCounterIgnoresTransientDropsAndConfirmsReset(t *testing.T) {
	store, err := openStore(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.db.Close() })
	now := time.Now().Truncate(time.Minute)
	for i, value := range []float64{100, 0, 103, 1, 2, 4} {
		if err := store.recordPelletCounter(t.Context(), "TEST", now.Add(time.Duration(i)*time.Minute), value); err != nil {
			t.Fatal(err)
		}
	}
	var total float64
	err = store.db.QueryRowContext(t.Context(), "SELECT SUM(kg) FROM pellet_usage").Scan(&total)
	if err != nil || total != 6 {
		t.Fatalf("tracked use: %v kg, %v", total, err)
	}
}

func TestPelletPurchasesWithSameDateKeepEntryOrder(t *testing.T) {
	ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ui.Close() })
	now := time.Now().Truncate(time.Second)
	entry := pelletPurchase{At: now.Add(-time.Hour), Quantity: 1, Weight: 40, Unit: "lb", Brand: "Cedar"}
	const first = "/api/pellets/purchases/ffffffffffffffffffffffffffffffff"
	requestJSON(t, ui, http.MethodPut, first, entry, http.StatusNoContent)
	entry.Brand = "Spruce"
	requestJSON(t, ui, http.MethodPut, "/api/pellets/purchases/11111111111111111111111111111111", entry, http.StatusNoContent)
	entry.Brand = "Cedar Premium"
	requestJSON(t, ui, http.MethodPut, first, entry, http.StatusNoContent)
	report, err := ui.store.pelletReport(t.Context(), now, nil)
	if err != nil || len(report.Purchases) != 2 || report.Purchases[0].Brand != "Cedar Premium" ||
		report.Purchases[1].Brand != "Spruce" {
		t.Fatalf("purchases: %+v, %v", report.Purchases, err)
	}
}

func TestLowPelletSupplyAlertsOnceUntilRestock(t *testing.T) {
	dir := t.TempDir()
	open := func() *UI {
		t.Helper()
		ui, err := New(t.Context(), slog.Default(), readOnlyFixture{}, "TEST", dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		return ui
	}
	ui := open()
	t.Cleanup(func() { _ = ui.Close() })
	now := time.Now().Add(-3 * time.Minute).Truncate(time.Second)
	purchase := pelletPurchase{At: now.Add(-time.Minute), Quantity: 1, Weight: 40, Unit: "lb", Brand: "Oak"}
	requestJSON(t, ui, http.MethodPut, "/api/pellets/settings",
		pelletSettings{Currency: "USD", LowWeight: 45, LowUnit: "lb"}, http.StatusNoContent)
	requestJSON(t, ui, http.MethodPut, "/api/pellets/purchases/11111111111111111111111111111111", purchase, http.StatusNoContent)
	readNotices := func(want int) {
		t.Helper()
		reply := requestJSON(t, ui, http.MethodGet, "/api/events?filter=fuel", nil, http.StatusOK)
		var rows []event
		if err := json.Unmarshal(reply.Body.Bytes(), &rows); err != nil || len(rows) != want {
			t.Fatalf("wanted %v fuel notices, got %v (%v)", want, rows, err)
		}
	}
	readNotices(0)
	if err := ui.store.recordPelletCounter(t.Context(), ui.serial, now, 150); err != nil {
		t.Fatal(err)
	}
	ui.checkPelletSupply(t.Context())
	readNotices(1)
	rows, err := ui.store.desktopNoticesAfter(t.Context(), 0)
	if err != nil || len(rows) != 1 || !strings.Contains(rows[0].Message, "40.0 lb") || pushKind(rows[0].Kind) != 8 {
		t.Fatalf("desktop fuel notices: %+v, %v", rows, err)
	}
	if err := ui.Close(); err != nil {
		t.Fatal(err)
	}
	ui = open()
	ui.checkPelletSupply(t.Context())
	readNotices(1)
	purchase.At = now
	requestJSON(t, ui, http.MethodPut, "/api/pellets/purchases/22222222222222222222222222222222", purchase, http.StatusNoContent)
	readNotices(1)
	if err := ui.store.recordPelletCounter(t.Context(), ui.serial, now.Add(time.Minute), 171); err != nil {
		t.Fatal(err)
	}
	ui.checkPelletSupply(t.Context())
	readNotices(2)
}

func TestPelletHourlyReportsDuringDaylightSavingFallback(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	local := time.Date(2025, time.November, 2, 6, 35, 0, 0, time.UTC).In(location)
	periods := pelletPeriods(local, "hour")
	secondHour, firstHour := periods[0].From, periods[1].From
	if secondHour.Hour() != 1 || firstHour.Hour() != 1 || secondHour.Sub(firstHour) != time.Hour ||
		!secondHour.Equal(time.Date(2025, time.November, 2, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected 1 AM boundaries: %s and %s", firstHour, secondHour)
	}
}

func requestJSON(t *testing.T, ui *UI, method, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Boiler-Mate", "1")
	reply := httptest.NewRecorder()
	ui.Handler().ServeHTTP(reply, request)
	if reply.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, reply.Code, want, reply.Body.String())
	}
	return reply
}

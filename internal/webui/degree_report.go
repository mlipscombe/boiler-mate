package webui

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

type degreeReport struct {
	Unit     string             `json:"unit"`
	Base     float64            `json:"base"`
	Currency string             `json:"currency"`
	Periods  []degreeFuelPeriod `json:"periods"`
	Brands   []degreeBrand      `json:"brands"`
}

type degreeFuelPeriod struct {
	weatherPeriod
	Complete    bool     `json:"complete"`
	UsedKg      *float64 `json:"usedKg"`
	Cost        *float64 `json:"cost"`
	KgPerDay    *float64 `json:"kgPerDegreeDay"`
	CostPerDay  *float64 `json:"costPerDegreeDay"`
	Brand       string   `json:"brand,omitempty"`
	MixedBrands bool     `json:"mixedBrands,omitempty"`
}

type degreeBrand struct {
	Name       string  `json:"name"`
	Periods    int     `json:"periods"`
	UsedKg     float64 `json:"usedKg"`
	DegreeDays float64 `json:"degreeDays"`
	KgPerDay   float64 `json:"kgPerDegreeDay"`
}

func (ui *UI) getDegreeReport(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	kind := query.Get("period")
	if kind == "" {
		kind = "day"
	}
	if !slices.Contains([]string{"hour", "day", "week", "month"}, kind) {
		problem(w, http.StatusBadRequest, "Choose hour, day, week, or month.")
		return
	}
	unit := query.Get("unit")
	if unit == "" {
		unit = "F"
	}
	if unit != "C" && unit != "F" {
		problem(w, http.StatusBadRequest, "Choose Celsius or Fahrenheit.")
		return
	}
	base := 65.0
	if unit == "C" {
		base = fahrenheitToCelsius(base)
	}
	if raw := query.Get("base"); raw != "" {
		var err error
		base, err = strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(base) || math.IsInf(base, 0) {
			problem(w, http.StatusBadRequest, "Enter a numeric heating base temperature.")
			return
		}
	}
	baseC, scale := base, 1.0
	if unit == "F" {
		baseC, scale = fahrenheitToCelsius(base), 1.8
	}
	if baseC < 0 || baseC > 30 {
		problem(w, http.StatusBadRequest, "Enter a base from 0 to 30 °C or 32 to 86 °F.")
		return
	}

	location := time.Local
	if zone := query.Get("tz"); zone != "" {
		var err error
		location, err = time.LoadLocation(zone)
		if err != nil {
			problem(w, http.StatusBadRequest, "Unknown report time zone.")
			return
		}
	}

	now := time.Now().In(location)
	periods := pelletPeriods(now, kind)
	if kind == "month" {
		for len(periods) < 24 {
			end := periods[len(periods)-1].From
			periods = append(periods, pelletPeriod{Kind: kind, From: end.AddDate(0, -1, 0), To: end})
		}
	}
	weather := make([]weatherPeriod, len(periods))
	for i, period := range periods {
		weather[i] = weatherPeriod{From: period.From, To: period.To}
	}
	if err := ui.store.fillWeatherPeriods(r.Context(), ui.serial, baseC, scale, weather); err != nil {
		ui.serverError(w, "read degree day history", err)
		return
	}
	fuel, err := ui.store.pelletReport(r.Context(), now, periods)
	if err != nil {
		ui.serverError(w, "read weather-normalized pellet use", err)
		return
	}

	result := combineDegreeFuelReport(weather, fuel, kind, scale)
	result.Base, result.Unit = base, unit
	writeJSON(w, result)
}

func combineDegreeFuelReport(weather []weatherPeriod, fuel pelletReport, kind string, scale float64) degreeReport {
	result := degreeReport{
		Currency: fuel.Settings.Currency,
		Periods:  make([]degreeFuelPeriod, len(weather)),
		Brands:   []degreeBrand{},
	}
	brands := map[string]degreeBrand{}
	for i, period := range weather {
		row := &result.Periods[i]
		row.weatherPeriod, row.Complete = period, i > 0
		if fuel.TrackedFrom == nil || fuel.SampledAt == nil || fuel.TrackedFrom.After(period.From) ||
			!fuel.SampledAt.After(period.From) || (row.Complete && fuel.SampledAt.Before(period.To)) {
			continue
		}
		burn := fuel.Periods[i]
		row.UsedKg = &burn.UsedKG
		if burn.UsedKG > 0 && burn.PricedKG >= burn.UsedKG-1e-6 {
			row.Cost = &burn.Cost
		}
		row.Brand, row.MixedBrands = degreePeriodBrand(burn)
		// Short or warm intervals yield noisy ratios. Require a completed
		// period, at least 95% joint observation coverage, and one equivalent °F-day.
		if kind == "hour" || !row.Complete || period.FuelCoverage < .95 || period.DegreeDays < scale/1.8 {
			continue
		}
		rate := burn.UsedKG / period.DegreeDays
		row.KgPerDay = &rate
		if row.Cost != nil {
			cost := *row.Cost / period.DegreeDays
			row.CostPerDay = &cost
		}
		if row.Brand != "" {
			brand := brands[row.Brand]
			brand.Name = row.Brand
			brand.Periods++
			brand.UsedKg += burn.UsedKG
			brand.DegreeDays += period.DegreeDays
			brands[row.Brand] = brand
		}
	}
	for _, brand := range brands {
		brand.KgPerDay = brand.UsedKg / brand.DegreeDays
		result.Brands = append(result.Brands, brand)
	}
	slices.SortFunc(result.Brands, func(a, b degreeBrand) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return result
}

func degreePeriodBrand(period pelletPeriod) (string, bool) {
	if period.UsedKG <= 0 {
		return "", false
	}
	var name string
	var knownKg float64
	for brand, amount := range period.BrandUsage {
		if brand != "" && amount > 1e-6 {
			if name != "" {
				return "", true
			}
			name, knownKg = brand, amount
		}
	}
	if knownKg < period.UsedKG-1e-6 {
		return "", false
	}
	return name, false
}

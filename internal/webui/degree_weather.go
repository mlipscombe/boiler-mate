package webui

import (
	"context"
	"time"
)

const (
	weatherSlotSeconds = 5 * 60
	weatherLocal       = "local"
	weatherRemote      = "remote"
)

const weatherSchema = `
	CREATE TABLE IF NOT EXISTS weather_observations (
		serial TEXT NOT NULL,
		time INTEGER NOT NULL,
		celsius REAL NOT NULL,
		source TEXT NOT NULL,
		fuel_observed INTEGER NOT NULL,
		PRIMARY KEY(serial, time)
	) WITHOUT ROWID;
`

type weatherPeriod struct {
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	DegreeHours   float64   `json:"degreeHours"`
	DegreeDays    float64   `json:"degreeDays"`
	Coverage      float64   `json:"coverage"`
	FuelCoverage  float64   `json:"fuelCoverage"`
	Source        string    `json:"source,omitempty"`
	localSeconds  int64
	remoteSeconds int64
	fuelSeconds   int64
}

func (ui *UI) recordWeather(ctx context.Context, now time.Time) {
	if ui.heating != nil {
		if celsius, ok := ui.heating.OutdoorReading(now); ok && celsius >= -75 && celsius <= 65 {
			ui.saveWeather(ctx, now, celsius, weatherLocal)
			return
		}
	}
	ui.mu.RLock()
	advanced, operating := ui.snapshots["advanced_data"], ui.snapshots["operating_data"]
	ui.mu.RUnlock()
	for _, reading := range []snapshot{advanced, operating} {
		if !freshObservation(reading, now, 25*time.Second) {
			continue
		}
		raw, ok := number(reading.Values["outdoor_temp"])
		if !ok {
			continue
		}
		celsius := raw
		if ui.outdoorWeatherFahrenheit {
			celsius = fahrenheitToCelsius(raw)
		}
		if celsius >= -75 && celsius <= 65 {
			ui.saveWeather(ctx, now, celsius, weatherRemote)
			return
		}
	}
}

func (ui *UI) saveWeather(ctx context.Context, now time.Time, celsius float64, source string) {
	slot := now.Unix() / weatherSlotSeconds * weatherSlotSeconds
	_, err := ui.store.db.ExecContext(ctx, `
		INSERT INTO weather_observations (serial, time, celsius, source, fuel_observed)
		SELECT ?, ?, ?, ?, EXISTS (
			SELECT 1 FROM pellet_counter WHERE id = 1 AND serial = ? AND sampled_at BETWEEN ? AND ?
		) WHERE true
		ON CONFLICT (serial, time) DO UPDATE SET
			celsius = CASE WHEN excluded.source = 'local' AND weather_observations.source = 'remote'
				THEN excluded.celsius ELSE weather_observations.celsius END,
			source = CASE WHEN excluded.source = 'local' THEN 'local' ELSE weather_observations.source END,
			fuel_observed = MAX(excluded.fuel_observed, weather_observations.fuel_observed)
		WHERE (excluded.source = 'local' AND weather_observations.source = 'remote')
			OR excluded.fuel_observed > weather_observations.fuel_observed
	`, ui.serial, slot, celsius, source, ui.serial, now.Add(-2*time.Minute).Unix(), now.Unix())
	if err != nil && ctx.Err() == nil {
		ui.logger.Warn("Could not save outdoor temperature history", "error", err)
	}
}

// fillWeatherPeriods expects non-overlapping periods in newest-first order.
func (s *store) fillWeatherPeriods(ctx context.Context, serial string, baseC, unitScale float64, periods []weatherPeriod) error {
	if len(periods) == 0 {
		return nil
	}
	first, last := periods[len(periods)-1].From.Unix(), periods[0].To.Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT time, celsius, source, fuel_observed FROM weather_observations
		WHERE serial = ? AND time >= ? AND time < ? ORDER BY time
	`, serial, first-weatherSlotSeconds, last)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	index := len(periods) - 1
	for rows.Next() {
		var start int64
		var celsius float64
		var source string
		var fuelObserved bool
		if err := rows.Scan(&start, &celsius, &source, &fuelObserved); err != nil {
			return err
		}
		for index >= 0 && start >= periods[index].To.Unix() {
			index--
		}
		end := start + weatherSlotSeconds
		for current := index; current >= 0 && end > periods[current].From.Unix(); current-- {
			period := &periods[current]
			seconds := min(end, period.To.Unix()) - max(start, period.From.Unix())
			if seconds <= 0 {
				continue
			}
			if fuelObserved {
				period.fuelSeconds += seconds
			}
			period.DegreeHours += max(0, baseC-celsius) * unitScale * float64(seconds) / 3600
			if source == weatherLocal {
				period.localSeconds += seconds
			} else {
				period.remoteSeconds += seconds
			}
		}
	}
	for i := range periods {
		period := &periods[i]
		totalSeconds := period.To.Unix() - period.From.Unix()
		if totalSeconds > 0 {
			period.Coverage = float64(period.localSeconds+period.remoteSeconds) / float64(totalSeconds)
			period.FuelCoverage = float64(period.fuelSeconds) / float64(totalSeconds)
		}
		period.DegreeDays = period.DegreeHours / 24
		switch {
		case period.localSeconds > 0 && period.remoteSeconds > 0:
			period.Source = "mixed"
		case period.localSeconds > 0:
			period.Source = weatherLocal
		case period.remoteSeconds > 0:
			period.Source = weatherRemote
		}
	}
	return rows.Err()
}

package webui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const pelletSchema = `
	CREATE TABLE IF NOT EXISTS pellet_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		currency TEXT NOT NULL,
		low_weight REAL NOT NULL,
		low_unit TEXT NOT NULL,
		notified INTEGER NOT NULL DEFAULT 0
	);
	INSERT INTO pellet_settings (id, currency, low_weight, low_unit)
	VALUES (1, 'USD', 0, 'kg') ON CONFLICT DO NOTHING;
	CREATE TABLE IF NOT EXISTS pellet_purchases (
		sequence INTEGER PRIMARY KEY,
		id TEXT NOT NULL UNIQUE,
		purchased_at INTEGER NOT NULL,
		quantity REAL NOT NULL,
		weight REAL NOT NULL,
		unit TEXT NOT NULL,
		brand TEXT NOT NULL,
		price REAL
	);
	CREATE INDEX IF NOT EXISTS pellet_purchases_date ON pellet_purchases (purchased_at, sequence);
	CREATE TABLE IF NOT EXISTS pellet_counter (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		serial TEXT NOT NULL,
		started_at INTEGER NOT NULL,
		sampled_at INTEGER NOT NULL,
		kg REAL NOT NULL,
		lower_at INTEGER,
		lower_kg REAL
	);
	CREATE TABLE IF NOT EXISTS pellet_usage (
		ended_at INTEGER PRIMARY KEY,
		started_at INTEGER NOT NULL,
		kg REAL NOT NULL
	);
`

func (s *store) recordPelletCounter(ctx context.Context, serial string, at time.Time, kg float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var previousSerial string
	var previousAt int64
	var previousKG float64
	var lowerAt sql.NullInt64
	var lowerKG sql.NullFloat64
	err = tx.QueryRowContext(ctx, `
		SELECT serial, sampled_at, kg, lower_at, lower_kg FROM pellet_counter WHERE id = 1
	`).Scan(&previousSerial, &previousAt, &previousKG, &lowerAt, &lowerKG)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := at.Unix()
	if now <= previousAt || lowerAt.Valid && now <= lowerAt.Int64 {
		return nil
	}
	if previousSerial == serial && previousAt != 0 {
		if kg < previousKG {
			// Require a second lower reading to confirm a reset. A transient low
			// reading followed by the original counter should not drain stock.
			if !lowerKG.Valid || kg < lowerKG.Float64 {
				if _, err := tx.ExecContext(ctx, "UPDATE pellet_counter SET lower_at = ?, lower_kg = ? WHERE id = 1", now, kg); err != nil {
					return err
				}
				return tx.Commit()
			}
			previousAt, previousKG = lowerAt.Int64, lowerKG.Float64
		}
		if kg > previousKG {
			if _, err := tx.ExecContext(ctx, "INSERT INTO pellet_usage VALUES (?, ?, ?)", now, previousAt, kg-previousKG); err != nil {
				return err
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pellet_counter (id, serial, started_at, sampled_at, kg)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET serial = excluded.serial, sampled_at = excluded.sampled_at,
			kg = excluded.kg, lower_at = NULL, lower_kg = NULL
	`, serial, now, now, kg); err != nil {
		return err
	}
	return tx.Commit()
}

func loadPelletSettings(ctx context.Context, tx *sql.Tx) (pelletSettings, bool, error) {
	var settings pelletSettings
	var notified bool
	err := tx.QueryRowContext(ctx, "SELECT currency, low_weight, low_unit, notified FROM pellet_settings WHERE id = 1").
		Scan(&settings.Currency, &settings.LowWeight, &settings.LowUnit, &notified)
	return settings, notified, err
}

func (s *store) pelletReport(ctx context.Context, now time.Time, periods []pelletPeriod) (pelletReport, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return pelletReport{}, err
	}
	defer func() { _ = tx.Rollback() }()
	settings, _, err := loadPelletSettings(ctx, tx)
	if err != nil {
		return pelletReport{}, err
	}
	return buildPelletReport(ctx, tx, settings, now, periods)
}

func buildPelletReport(ctx context.Context, tx *sql.Tx, settings pelletSettings, now time.Time, periods []pelletPeriod) (pelletReport, error) {
	report := pelletReport{Settings: settings, Periods: periods, Purchases: []pelletPurchase{}, Brands: []pelletBrand{}}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, purchased_at, quantity, weight, unit, brand, price FROM pellet_purchases ORDER BY purchased_at, sequence
	`)
	if err != nil {
		return report, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var entry pelletPurchase
		var at int64
		if err := rows.Scan(&entry.ID, &at, &entry.Quantity, &entry.Weight, &entry.Unit, &entry.Brand, &entry.Price); err != nil {
			return report, err
		}
		entry.At = time.Unix(at, 0).UTC()
		report.Purchases = append(report.Purchases, entry)
	}
	if err := rows.Err(); err != nil {
		return report, err
	}

	var startedAt, sampledAt int64
	err = tx.QueryRowContext(ctx, "SELECT started_at, sampled_at FROM pellet_counter WHERE id = 1").Scan(&startedAt, &sampledAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return report, err
	}
	allocation := allocatePellets(report.Purchases, periods)
	if err == nil {
		trackedFrom, sampled := time.Unix(startedAt, 0).UTC(), time.Unix(sampledAt, 0).UTC()
		report.TrackedFrom, report.SampledAt = &trackedFrom, &sampled
		from := now.Unix()
		if len(report.Purchases) > 0 {
			from = report.Purchases[0].At.Unix()
		}
		for _, period := range periods {
			from = min(from, period.From.Unix())
		}
		usage, err := tx.QueryContext(ctx, "SELECT started_at, ended_at, kg FROM pellet_usage WHERE ended_at > ? ORDER BY ended_at", from)
		if err != nil {
			return report, err
		}
		defer func() { _ = usage.Close() }()
		for usage.Next() {
			var start, end int64
			var kg float64
			if err := usage.Scan(&start, &end, &kg); err != nil {
				return report, err
			}
			allocation.use(start, end, kg)
		}
		if err := usage.Err(); err != nil {
			return report, err
		}
	}
	if len(report.Purchases) > 0 {
		remaining := allocation.remaining(now.Unix())
		report.RemainingKG = &remaining
		report.Brands = allocation.brandSummary()
	}
	return report, nil
}

func (s *store) checkPelletSupply(ctx context.Context, now time.Time, serial string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	settings, notified, err := loadPelletSettings(ctx, tx)
	if err != nil || settings.LowWeight == 0 {
		return false, err
	}
	var sampledAt int64
	err = tx.QueryRowContext(ctx, "SELECT sampled_at FROM pellet_counter WHERE id = 1 AND serial = ?", serial).Scan(&sampledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if now.Sub(time.Unix(sampledAt, 0)) > 5*time.Minute {
		return false, nil
	}
	report, err := buildPelletReport(ctx, tx, settings, now, nil)
	if err != nil {
		return false, err
	}
	low := report.RemainingKG != nil && *report.RemainingKG <= pelletWeightKG(settings.LowWeight, settings.LowUnit)
	if low == notified {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE pellet_settings SET notified = ? WHERE id = 1", low); err != nil {
		return false, err
	}
	if low {
		weight := *report.RemainingKG
		if settings.LowUnit == "lb" {
			weight /= kgPerPound
		}
		message := fmt.Sprintf("Pellet supply is low. About %.1f %s remain on hand.", weight, settings.LowUnit)
		if _, err := tx.ExecContext(ctx, "INSERT INTO events(time, kind, message) VALUES (?, 'fuel', ?)", now.Unix(), message); err != nil {
			return false, err
		}
	}
	return low, tx.Commit()
}

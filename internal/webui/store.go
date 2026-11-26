package webui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/efficiency"
	"github.com/mlipscombe/boiler-mate/nbe"
	_ "modernc.org/sqlite"
)

type store struct {
	db *sql.DB
}

type point struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type event struct {
	ID      int64  `json:"id"`
	Time    int64  `json:"time"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Read    bool   `json:"read"`
}

func openStore(ctx context.Context, dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	path, err := filepath.Abs(filepath.Join(dir, "boiler-mate.sqlite"))
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("open history file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, `
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 3000;
		CREATE TABLE IF NOT EXISTS samples (
			period INTEGER NOT NULL,
			series TEXT NOT NULL,
			time INTEGER NOT NULL,
			total REAL NOT NULL,
			count INTEGER NOT NULL,
			PRIMARY KEY(period, series, time)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY,
			time INTEGER NOT NULL,
			kind TEXT NOT NULL,
			message TEXT NOT NULL,
			seen INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS efficiency_samples (
			serial TEXT NOT NULL,
			time INTEGER NOT NULL,
			observation BLOB NOT NULL,
			PRIMARY KEY(serial, time)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS zone_pumps (
			serial TEXT NOT NULL,
			zone TEXT NOT NULL,
			temperature TEXT NOT NULL,
			flow TEXT NOT NULL,
			PRIMARY KEY(serial, zone)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS zone_demand (
			serial TEXT NOT NULL,
			zone TEXT NOT NULL,
			start INTEGER NOT NULL,
			end INTEGER NOT NULL,
			calling INTEGER NOT NULL,
			PRIMARY KEY(serial, zone, start)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS push_keys (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			private_key TEXT NOT NULL,
			public_key TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS push_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint TEXT NOT NULL UNIQUE,
			auth TEXT NOT NULL,
			p256dh TEXT NOT NULL,
			kinds INTEGER NOT NULL,
			cursor INTEGER NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			failures INTEGER NOT NULL DEFAULT 0,
			retry_at INTEGER NOT NULL DEFAULT 0
		);
	`+pelletSchema+weatherSchema)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize history: %w", err), db.Close())
	}
	return &store{db: db}, nil
}

func (s *store) record(ctx context.Context, prefix string, now time.Time, values map[string]any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO samples(period, series, time, total, count) VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(period, series, time) DO UPDATE SET total = total + excluded.total, count = count + 1
	`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for key, value := range values {
		number, ok := number(value)
		if !ok || (strings.HasSuffix(key, "_temp") && number >= 999) {
			continue
		}
		for _, period := range []int64{60, 3600, 86400} {
			bucket := now.Unix() / period * period
			if _, err := stmt.ExecContext(ctx, period, prefix+"."+key, bucket, number); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func number(value any) (float64, bool) {
	switch v := value.(type) {
	case int64:
		return float64(v), true
	case nbe.RoundedFloat:
		return float64(v), true
	case float64:
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	default:
		return 0, false
	}
}

func (s *store) history(ctx context.Context, series string, from, to int64) ([]point, error) {
	period := int64(60)
	if to-from > 90*86400 {
		period = 86400
	} else if to-from > 7*86400 || time.Now().Unix()-from > 29*86400 {
		period = 3600
	}
	if time.Now().Unix()-from > 365*86400 {
		period = 86400
	}
	// Return a chart-sized result even at the finest supported intervals.
	step := max(period, ((to-from)/450+period-1)/period*period)
	rows, err := s.db.QueryContext(ctx, `
		SELECT (time / ?) * ?, SUM(total) / SUM(count)
		FROM samples
		WHERE period = ? AND series = ? AND time >= ? AND time <= ?
		GROUP BY time / ?
		ORDER BY time / ?
	`, step, step, period, series, from/period*period, to, step, step)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	points := []point{}
	for rows.Next() {
		var p point
		if err := rows.Scan(&p.Time, &p.Value); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func (s *store) performanceHistory(ctx context.Context, now time.Time, showOffTarget bool) (map[string][]point, error) {
	// Keep minute samples to preserve target changes at shutdown.
	rows, err := s.db.QueryContext(ctx, `
		SELECT time,
			MAX(CASE WHEN series = 'operating_data.boiler_temp' THEN total / count END),
			MAX(CASE WHEN series = 'operating_data.boiler_ref' THEN total / count END),
			MAX(CASE WHEN series = 'operating_data.oxygen' THEN total / count END),
			COALESCE(
				MAX(CASE WHEN series = 'operating_data.oxygen_ref' THEN total / count END),
				MAX(CASE WHEN series = 'advanced_data.o2_setpoint' THEN total / count END)
			),
			MAX(CASE WHEN series = 'operating_data.state' THEN total / count END)
		FROM samples
		WHERE period = 60 AND time BETWEEN ? AND ?
		  AND series IN ('operating_data.boiler_temp', 'operating_data.boiler_ref',
			'operating_data.oxygen', 'operating_data.oxygen_ref', 'advanced_data.o2_setpoint', 'operating_data.state')
		GROUP BY time ORDER BY time
	`, now.Add(-24*time.Hour).Unix(), now.Unix())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	temperature, temperatureTarget, oxygen, oxygenTarget := []point{}, []point{}, []point{}, []point{}
	for rows.Next() {
		var timestamp int64
		var boiler, boilerRef, measured, wanted, state sql.NullFloat64
		if err := rows.Scan(&timestamp, &boiler, &boilerRef, &measured, &wanted, &state); err != nil {
			return nil, err
		}
		if boiler.Valid {
			temperature = append(temperature, point{Time: timestamp, Value: boiler.Float64})
		}
		if boilerRef.Valid {
			temperatureTarget = append(temperatureTarget, point{Time: timestamp, Value: boilerRef.Float64})
		}
		if measured.Valid {
			oxygen = append(oxygen, point{Time: timestamp, Value: measured.Float64})
		}
		off := state.Valid && state.Float64 == nbe.PowerStateOff
		if off && showOffTarget {
			oxygenTarget = append(oxygenTarget, point{Time: timestamp, Value: 0})
		} else if !off && wanted.Valid && (wanted.Float64 > 0 || showOffTarget && wanted.Float64 == 0) {
			oxygenTarget = append(oxygenTarget, point{Time: timestamp, Value: wanted.Float64})
		}
	}
	return map[string][]point{
		"operating_data.boiler_temp": temperature, "operating_data.boiler_ref": temperatureTarget,
		"operating_data.oxygen": oxygen, "operating_data.oxygen_ref": oxygenTarget,
	}, rows.Err()
}

func (s *store) prune(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM samples WHERE
			(period = 60 AND time < ?) OR
			(period = 3600 AND time < ?)
	`, now.AddDate(0, 0, -30).Unix(), now.AddDate(-1, 0, 0).Unix())
	if err == nil {
		_, err = s.db.ExecContext(ctx, "DELETE FROM efficiency_samples WHERE time < ?", now.Add(-efficiency.Lookback).UnixMilli())
	}
	if err == nil {
		_, err = s.db.ExecContext(ctx, "DELETE FROM weather_observations WHERE time < ?", now.AddDate(-2, 0, 0).Unix())
	}
	if err == nil {
		_, err = s.db.ExecContext(ctx, "DELETE FROM zone_demand WHERE end <= ?", now.Add(-zoneDemandLookback).UnixMilli())
	}
	return err
}

func (s *store) addEvent(ctx context.Context, at time.Time, kind, message string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO events(time, kind, message) VALUES (?, ?, ?)", at.Unix(), kind, message)
	return err
}

func (s *store) lastOperatingState(ctx context.Context) (string, error) {
	var message string
	err := s.db.QueryRowContext(ctx, `
		SELECT message FROM events WHERE kind IN ('state', 'alarm') ORDER BY id DESC LIMIT 1
	`).Scan(&message)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if message == burnerFiringMessage || strings.HasPrefix(message, burnerFiringMessage+" · ") {
		message = controllerFiringState
	}
	return message, err
}

func (s *store) events(ctx context.Context, before int64, filter string) ([]event, error) {
	return s.queryEvents(ctx, `
		SELECT id, time, kind, message, seen
		FROM events
		WHERE (@before = 0 OR id < @before)
		  AND CASE @filter
			WHEN '' THEN 1
			WHEN 'notices' THEN kind <> 'state'
			WHEN 'unread' THEN kind <> 'state' AND seen = 0
			ELSE kind = @filter
		  END
		ORDER BY id DESC LIMIT 150
	`, sql.Named("before", before), sql.Named("filter", filter))
}

func (s *store) latestEventID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM events").Scan(&id)
	return id, err
}

func (s *store) desktopNoticesAfter(ctx context.Context, after int64) ([]event, error) {
	return s.queryEvents(ctx, `
		SELECT id, time, kind, message, seen
		FROM events
		WHERE id > ? AND seen = 0 AND kind IN ('alarm', 'info', 'connection', 'fuel')
		ORDER BY id LIMIT 150
	`, after)
}

func (s *store) queryEvents(ctx context.Context, query string, args ...any) ([]event, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := []event{}
	for rows.Next() {
		var row event
		if err := rows.Scan(&row.ID, &row.Time, &row.Kind, &row.Message, &row.Read); err != nil {
			return nil, err
		}
		if row.Kind == "info" {
			row.Message = infoMessage(row.Message)
		}
		if row.Kind == "state" && row.Message == controllerFiringState {
			row.Message = burnerFiringMessage
		}
		events = append(events, row)
	}
	return events, rows.Err()
}

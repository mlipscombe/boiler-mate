package webui

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"
)

const zoneDemandLookback = 30 * 24 * time.Hour

type demandPeriod struct {
	Start   int64 `json:"start"`
	End     int64 `json:"end"`
	Calling bool  `json:"calling"`
}

type zoneDemandPeriod struct {
	zone string
	demandPeriod
}

type demandRecorder struct {
	mu      sync.Mutex
	current map[string]demandPeriod
	closed  []zoneDemandPeriod
	wake    chan struct{}
}

func newDemandRecorder() demandRecorder {
	return demandRecorder{current: make(map[string]demandPeriod), wake: make(chan struct{}, 1)}
}

func (d *demandRecorder) observe(now time.Time, states map[string]bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	at := now.UnixMilli()
	for zone, period := range d.current {
		if calling, known := states[zone]; known && calling == period.Calling {
			continue
		}
		period.End = at
		d.closed = append(d.closed, zoneDemandPeriod{zone, period})
		delete(d.current, zone)
	}
	for zone, calling := range states {
		if _, known := d.current[zone]; !known {
			d.current[zone] = demandPeriod{Start: at, Calling: calling}
		}
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (ui *UI) watchPumpDemand() {
	ui.heating.WatchZoneDemand(slices.Collect(maps.Keys(ui.pumpZones)), ui.zoneDemand.observe)
}

func (ui *UI) runZoneDemand(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ui.zoneDemand.wake:
		case <-ticker.C:
		}
		if err := ui.flushZoneDemand(ctx, time.Now()); err != nil && ctx.Err() == nil {
			ui.logger.Warn("Could not save zone demand history", "error", err)
		}
	}
}

// Save both edges and a periodic checkpoint. A restart leaves a gap after the
// last checkpoint until the thermostat reports a confirmed state again.
func (ui *UI) flushZoneDemand(ctx context.Context, now time.Time) error {
	d := &ui.zoneDemand
	d.mu.Lock()
	closed := len(d.closed)
	periods := slices.Clone(d.closed)
	for zone, period := range d.current {
		period.End = now.UnixMilli()
		periods = append(periods, zoneDemandPeriod{zone, period})
	}
	d.mu.Unlock()
	if len(periods) == 0 {
		return nil
	}
	if err := ui.store.saveZoneDemand(ctx, ui.serial, periods); err != nil {
		return err
	}
	d.mu.Lock()
	d.closed = slices.Delete(d.closed, 0, closed)
	d.mu.Unlock()
	ui.mu.Lock()
	ui.historyRev++
	ui.signalChange()
	ui.mu.Unlock()
	return nil
}

func (s *store) saveZoneDemand(ctx context.Context, serial string, periods []zoneDemandPeriod) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, period := range periods {
		if period.End <= period.Start {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO zone_demand(serial, zone, start, end, calling) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(serial, zone, start) DO UPDATE SET end = excluded.end
		`, serial, period.zone, period.Start, period.End, period.Calling); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (ui *UI) getZoneDemand(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	zone := query.Get("zone")
	if !validZone(zone) {
		problem(w, http.StatusBadRequest, "Choose a heating zone.")
		return
	}
	from, err := strconv.ParseInt(query.Get("from"), 10, 64)
	now := time.Now()
	if err != nil || from < now.Add(-zoneDemandLookback-time.Minute).Unix() || from > now.Unix() {
		problem(w, http.StatusBadRequest, "Choose a start within the last 30 days.")
		return
	}
	periods, err := ui.store.zoneDemandHistory(r.Context(), ui.serial, zone, time.Unix(from, 0).UnixMilli(), now.UnixMilli())
	if err != nil {
		ui.serverError(w, "load zone demand history", err)
		return
	}
	writeJSON(w, periods)
}

func (s *store) zoneDemandHistory(ctx context.Context, serial, zone string, from, to int64) ([]demandPeriod, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT MAX(start, ?), MIN(end, ?), calling
		FROM zone_demand WHERE serial = ? AND zone = ? AND start < ? AND end > ?
		ORDER BY start
	`, from, to, serial, zone, to, from)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	periods := []demandPeriod{}
	for rows.Next() {
		var period demandPeriod
		if err := rows.Scan(&period.Start, &period.End, &period.Calling); err != nil {
			return nil, err
		}
		periods = append(periods, period)
	}
	return periods, rows.Err()
}

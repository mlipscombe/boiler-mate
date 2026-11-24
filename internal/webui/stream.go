package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"time"
)

type dashboardStatus struct {
	Serial          string              `json:"serial"`
	Data            map[string]snapshot `json:"data"`
	Online          bool                `json:"online"`
	Control         bool                `json:"control"`
	Fahrenheit      bool                `json:"fahrenheit"`
	HistoryRevision uint64              `json:"historyRevision"`
	EventRevision   uint64              `json:"eventRevision"`
}

// signalChange wakes the streams without holding a queue of past readings. The caller holds ui.mu.
func (ui *UI) signalChange() {
	close(ui.changed)
	ui.changed = make(chan struct{})
}

func (ui *UI) signalEventChange() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.eventRev++
	close(ui.eventChanged)
	ui.eventChanged = make(chan struct{})
	ui.signalChange()
}

func (ui *UI) eventChanges() <-chan struct{} {
	ui.mu.RLock()
	defer ui.mu.RUnlock()
	return ui.eventChanged
}

func (ui *UI) statusSnapshot(telemetryOnly bool) (dashboardStatus, <-chan struct{}) {
	ui.mu.RLock()
	defer ui.mu.RUnlock()
	var data map[string]snapshot
	if telemetryOnly {
		data = make(map[string]snapshot, 2)
		for _, topic := range []string{"operating_data", "advanced_data"} {
			if readings, ok := ui.snapshots[topic]; ok {
				data[topic] = readings
			}
		}
	} else {
		data = maps.Clone(ui.snapshots)
	}
	if ui.outdoorWeatherFahrenheit {
		for _, topic := range []string{"operating_data", "advanced_data"} {
			readings := data[topic]
			if temp, ok := number(readings.Values["outdoor_temp"]); ok && temp < 999 {
				readings.Values = maps.Clone(readings.Values)
				readings.Values["outdoor_temp"] = fahrenheitToCelsius(temp)
				data[topic] = readings
			}
		}
	}
	latest := data["operating_data"].At
	return dashboardStatus{
		Serial: ui.serial, Data: data, Online: !latest.IsZero() && time.Since(latest) < 20*time.Second,
		Control: ui.write != nil, Fahrenheit: ui.fahrenheit, HistoryRevision: ui.historyRev, EventRevision: ui.eventRev,
	}, ui.changed
}

func (ui *UI) streamStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	response := http.NewResponseController(w)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var previous <-chan struct{}
	var wasOnline bool
	for r.Context().Err() == nil {
		status, changed := ui.statusSnapshot(true)
		var data any
		if previous != changed || wasOnline != status.Online {
			data = status
			previous, wasOnline = changed, status.Online
		}
		if writeStreamFrame(w, response, "", data) != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		case <-ticker.C:
		}
	}
}

func (ui *UI) streamNotices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("after")
	}
	var cursor int64
	if raw != "" {
		var err error
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			problem(w, http.StatusBadRequest, "Invalid notice cursor.")
			return
		}
	}
	latest, err := ui.store.latestEventID(ctx)
	if err != nil {
		ui.serverError(w, "read notice cursor", err)
		return
	}
	if raw == "" || cursor > latest {
		cursor = latest
	}
	w.Header().Set("Content-Type", "text/event-stream")
	response := http.NewResponseController(w)
	if writeStreamFrame(w, response, strconv.FormatInt(cursor, 10), []event{}) != nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		// Capture the signal before querying so an event committed during the query wakes the stream.
		changed := ui.eventChanges()
		notices, err := ui.store.desktopNoticesAfter(ctx, cursor)
		if err != nil {
			if ctx.Err() == nil {
				ui.logger.Warn("Could not read desktop notices", "error", err)
			}
			return
		}
		if len(notices) > 0 {
			cursor = notices[len(notices)-1].ID
			if writeStreamFrame(w, response, strconv.FormatInt(cursor, 10), notices) != nil {
				return
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-changed:
		case <-ticker.C:
			if writeStreamFrame(w, response, "", nil) != nil {
				return
			}
		}
	}
}

func writeStreamFrame(w http.ResponseWriter, response *http.ResponseController, id string, value any) error {
	// Extend the HTTP server's write deadline while bounding a blocked connection.
	if err := response.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if value == nil {
		if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
			return err
		}
	} else {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if id != "" {
			if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
	}
	return response.Flush()
}

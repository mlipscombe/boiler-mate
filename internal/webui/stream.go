package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"time"
)

type dashboardStatus struct {
	Serial          string              `json:"serial"`
	Data            map[string]snapshot `json:"data"`
	Online          bool                `json:"online"`
	Control         bool                `json:"control"`
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
	ui.signalChange()
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
	latest := data["operating_data"].At
	return dashboardStatus{
		Serial: ui.serial, Data: data, Online: !latest.IsZero() && time.Since(latest) < 20*time.Second,
		Control: ui.write != nil, HistoryRevision: ui.historyRev, EventRevision: ui.eventRev,
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
		// Extend the normal HTTP write deadline for each event, while bounding a blocked write.
		if err := response.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return
		}
		var err error
		if previous != changed || wasOnline != status.Online {
			var data []byte
			data, err = json.Marshal(status)
			if err == nil {
				_, err = fmt.Fprintf(w, "data: %s\n\n", data)
			}
			previous, wasOnline = changed, status.Online
		} else {
			_, err = io.WriteString(w, ": keepalive\n\n")
		}
		if err != nil || response.Flush() != nil {
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

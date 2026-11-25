package webui

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
)

// The routes in this file read lists straight from the controller. Their
// reply layouts come from the NBE app and are unverified against hardware.

type controllerEvent struct {
	// Time is zero when the controller's date and clock were not readable.
	Time int64 `json:"time"`
	nbe.ControllerEvent
}

// readController sends one read request and parses its reply. It writes the
// error response itself and reports whether the caller may continue.
func readController[T any](ui *UI, w http.ResponseWriter, r *http.Request, function nbe.Function, payload, name string, parse func(string) (T, error)) (T, bool) {
	var result T
	reply, err := ui.controller.Get(r.Context(), function, payload)
	if err == nil {
		result, err = parse(reply.RawPayload)
	}
	if err == nil {
		return result, true
	}
	ui.logger.Debug("Could not read controller list", "list", name, "error", err)
	var format *nbe.ReplyFormatError
	if errors.As(err, &format) {
		problem(w, http.StatusBadGateway, "The controller's "+name+" was not in the expected format.")
	} else {
		problem(w, http.StatusServiceUnavailable, "The controller did not return its "+name+".")
	}
	return result, false
}

// getControllerEvents returns the controller's own event log. It is separate
// from /api/events, which lists the events Boiler Mate records itself.
func (ui *UI) getControllerEvents(w http.ResponseWriter, r *http.Request) {
	at := time.Now()
	if raw := r.URL.Query().Get("at"); raw != "" {
		seconds, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || seconds <= 0 || seconds > at.Unix()+60 {
			problem(w, http.StatusBadRequest, "Invalid event log time.")
			return
		}
		at = time.Unix(seconds, 0)
	}
	// The controller's clock carries no zone; this assumes it matches the host's.
	events, ok := readController(ui, w, r, nbe.GetEventLogFunction, nbe.EventLogRequest(at), "event log",
		func(payload string) ([]nbe.ControllerEvent, error) { return nbe.ParseEventLog(payload, time.Local) })
	if !ok {
		return
	}
	rows := make([]controllerEvent, len(events))
	for index, event := range events {
		rows[index] = controllerEvent{ControllerEvent: event}
		if !event.At.IsZero() {
			rows[index].Time = event.At.Unix()
		}
	}
	writeJSON(w, rows)
}

// getControllerVersions lists the firmware versions the controller offers.
// Nothing in Boiler Mate selects or downloads one.
func (ui *UI) getControllerVersions(w http.ResponseWriter, r *http.Request) {
	versions, ok := readController(ui, w, r, nbe.GetAvailableProgramsFunction, nbe.AvailableProgramsRequest,
		"version list", nbe.ParseAvailablePrograms)
	if ok {
		writeJSON(w, versions)
	}
}

func (ui *UI) getRemoteSensors(w http.ResponseWriter, r *http.Request) {
	sensors, ok := readController(ui, w, r, nbe.GetRemoteSensorsFunction, nbe.RemoteSensorsRequest,
		"remote sensor list", nbe.ParseRemoteSensors)
	if ok {
		writeJSON(w, sensors)
	}
}

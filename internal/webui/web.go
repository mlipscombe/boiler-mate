package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/controllerread"
	"github.com/mlipscombe/boiler-mate/internal/efficiency"
	"github.com/mlipscombe/boiler-mate/internal/heating"
	"github.com/mlipscombe/boiler-mate/nbe"
)

//go:embed assets/*
var assets embed.FS

type controller interface {
	Get(context.Context, nbe.Function, string) (*nbe.NBEResponse, error)
}

type WriteFunc func(context.Context, string, []byte) error

type Options struct {
	ReadController           controllerread.ReadFunc
	Write                    WriteFunc
	OutdoorWeatherFahrenheit bool
	Heating                  *heating.Control
}

type UI struct {
	read                     controllerread.ReadFunc
	controller               controller
	write                    WriteFunc
	store                    *store
	logger                   *slog.Logger
	serial                   string
	push                     *webPush
	outdoorWeatherFahrenheit bool
	heating                  *heating.Control
	efficiencySession        int64

	settingsMu   sync.Mutex
	mu           sync.RWMutex
	snapshots    map[string]snapshot
	lastSample   map[string]time.Time
	changed      chan struct{}
	eventChanged chan struct{}
	historyRev   uint64
	eventRev     uint64
	fahrenheit   bool
	lastState    string
	lastInfo     string
	offline      bool
	efficiency   efficiency.Report
}

type snapshot struct {
	At     time.Time      `json:"at"`
	Values map[string]any `json:"values"`
}

func New(ctx context.Context, logger *slog.Logger, boiler controller, serial, dataDir string, options Options) (*UI, error) {
	store, err := openStore(ctx, dataDir)
	if err != nil {
		return nil, err
	}
	lastState, err := store.lastOperatingState(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("load last operating state: %w", err), store.db.Close())
	}
	push, err := newWebPush(ctx, store)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("initialize web push: %w", err), store.db.Close())
	}
	if options.ReadController == nil {
		options.ReadController = func(ctx context.Context, kind string, q url.Values) (any, error) {
			return controllerread.Read(ctx, boiler, kind, q)
		}
	}
	return &UI{
		read:       options.ReadController,
		controller: boiler, write: options.Write, store: store, logger: logger, serial: serial,
		push:                     push,
		outdoorWeatherFahrenheit: options.OutdoorWeatherFahrenheit,
		heating:                  options.Heating,
		efficiencySession:        time.Now().UnixNano(),
		snapshots:                make(map[string]snapshot), lastSample: make(map[string]time.Time),
		changed: make(chan struct{}), eventChanged: make(chan struct{}), lastState: lastState,
	}, nil
}

func (ui *UI) Close() error {
	return ui.store.db.Close()
}

// Observe receives the same real controller snapshots used for MQTT.
func (ui *UI) Observe(ctx context.Context, topic string, values map[string]any) {
	now := time.Now()
	if topic == "misc" {
		values = maps.Clone(values)
		for key := range values {
			if !visibleSetting(key) {
				delete(values, key)
			}
		}
	}
	ui.mu.Lock()
	ui.snapshots[topic] = snapshot{At: now, Values: values}
	isTelemetry := topic == "operating_data" || topic == "advanced_data"
	var unitChanged bool
	if topic == "misc" {
		fahrenheit := sameSettingValue("1", values["fahrenheit"])
		unitChanged = fahrenheit != ui.fahrenheit
		ui.fahrenheit = fahrenheit
	}
	if isTelemetry || unitChanged {
		ui.signalChange()
	}
	sample := isTelemetry && now.Truncate(time.Minute) != ui.lastSample[topic]
	if sample {
		ui.lastSample[topic] = now.Truncate(time.Minute)
	}
	var state, kind string
	if topic == "operating_data" {
		ui.offline = false
		state, _ = values["state_text"].(string)
		if state == ui.lastState {
			state = ""
		} else if state != "" {
			ui.lastState = state
			kind = "state"
			description := strings.ToLower(state)
			for _, marker := range []string{"alarm", "error", "fault", "fail", "overheat", "disconnected"} {
				if strings.Contains(description, marker) {
					kind = "alarm"
					break
				}
			}
		}
	}
	ui.mu.Unlock()

	if sample {
		if err := ui.store.record(ctx, topic, now, values); err == nil {
			ui.mu.Lock()
			ui.historyRev++
			ui.signalChange()
			ui.mu.Unlock()
		} else if ctx.Err() == nil {
			ui.logger.Warn("Could not save telemetry", "group", topic, "error", err)
		}
	}
	if state != "" {
		ui.addEvent(ctx, now, kind, operatingStateMessage(state, values))
	}
	if topic == "operating_data" {
		ui.recordEfficiency(ctx, now, values)
	}
}

// Run records controller notices, delivers web push, and maintains local history.
func (ui *UI) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Go(func() { ui.runWebPush(ctx) })
	workers.Go(func() { ui.runEfficiency(ctx) })
	workers.Go(func() { ui.runPelletCounter(ctx) })
	defer workers.Wait()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var pruned time.Time
	for ctx.Err() == nil {
		now := time.Now()
		if pruned.IsZero() || now.Sub(pruned) >= 24*time.Hour {
			if err := ui.store.prune(ctx, now); err != nil && ctx.Err() == nil {
				ui.logger.Warn("Could not prune old chart samples", "error", err)
			} else {
				pruned = now
			}
		}
		ui.pollNotices(ctx, now)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (ui *UI) pollNotices(ctx context.Context, now time.Time) {
	ui.mu.Lock()
	last := ui.snapshots["operating_data"].At
	lost := !last.IsZero() && now.Sub(last) > 20*time.Second && !ui.offline
	if lost {
		ui.offline = true
	}
	ui.mu.Unlock()
	if lost {
		ui.addEvent(ctx, now, "connection", "Connection to the controller was interrupted.")
	}

	reply, err := ui.controller.Get(ctx, nbe.GetInfoFunction, "*")
	if err != nil {
		return
	}
	info := strings.TrimSpace(reply.RawPayload)
	if info == "0" {
		info = ""
	}
	if info == ui.lastInfo {
		return
	}
	ui.lastInfo = info
	if info != "" {
		ui.addEvent(ctx, now, "info", infoMessage(info))
	}
}

func (ui *UI) addEvent(ctx context.Context, at time.Time, kind, message string) {
	if err := ui.store.addEvent(ctx, at, kind, message); err == nil {
		ui.signalEventChange()
	} else if ctx.Err() == nil {
		ui.logger.Warn("Could not save event", "kind", kind, "error", err)
	}
}

func (ui *UI) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /api/status", ui.getStatus)
	router.HandleFunc("GET /api/status/stream", ui.streamStatus)
	router.HandleFunc("GET /api/history", ui.getHistory)
	router.HandleFunc("GET /api/history/combustion", ui.performanceHistoryHandler(true))
	router.HandleFunc("GET /api/history/performance", ui.performanceHistoryHandler(false))
	router.HandleFunc("GET /api/consumption", ui.getConsumption)
	router.HandleFunc("GET /api/pellets", ui.getPelletLedger)
	router.HandleFunc("PUT /api/pellets/purchases/{id}", ui.putPelletPurchase)
	router.HandleFunc("DELETE /api/pellets/purchases/{id}", ui.deletePelletPurchase)
	router.HandleFunc("PUT /api/pellets/settings", ui.putPelletSettings)
	router.HandleFunc("GET /api/efficiency", ui.getEfficiency)
	router.HandleFunc("GET /api/efficiency/weather", ui.getDegreeReport)
	router.HandleFunc("GET /api/events", ui.getEvents)
	router.HandleFunc("GET /api/controller/{kind}", ui.getControllerRead)
	router.HandleFunc("GET /api/notifications", ui.getNotices)
	router.HandleFunc("GET /api/notifications/stream", ui.streamNotices)
	router.HandleFunc("GET /api/notifications/push-key", ui.getPushKey)
	router.HandleFunc("PUT /api/notifications/push", ui.putPushSubscription)
	router.HandleFunc("DELETE /api/notifications/push", ui.deletePushSubscription)
	router.HandleFunc("POST /api/notifications/{id}/read", ui.markRead)
	router.HandleFunc("GET /api/settings/{group}", ui.getSettings)
	router.HandleFunc("PUT /api/settings/{group}/{key}", ui.putSetting)
	router.HandleFunc("POST /api/power", ui.postPower)
	router.HandleFunc("POST /api/alarm/reset", ui.resetAlarm)
	router.HandleFunc("GET /api/external-heating", ui.getExternalHeating)
	router.HandleFunc("PUT /api/external-heating", ui.putExternalHeating)
	router.HandleFunc("POST /api/external-heating/resume", ui.resumeExternalHeating)
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	static := http.FileServerFS(files)
	router.Handle("GET /", static)
	router.HandleFunc("GET /site.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		static.ServeHTTP(w, r)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			// Browsers cannot set this header from another origin without a CORS preflight.
			if r.Header.Get("X-Boiler-Mate") != "1" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				problem(w, http.StatusForbidden, "Request must originate in Boiler Mate.")
				return
			}
		}
		router.ServeHTTP(w, r)
	})
}

func (ui *UI) getStatus(w http.ResponseWriter, _ *http.Request) {
	status, _ := ui.statusSnapshot(false)
	writeJSON(w, status)
}

func (ui *UI) getHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	now := time.Now()
	from := now.Add(-24 * time.Hour).Unix()
	if raw := query.Get("from"); raw != "" {
		start, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || start <= 0 || start > now.Unix() {
			problem(w, http.StatusBadRequest, "Invalid history start time.")
			return
		}
		from = start
	}
	to := now.Unix()
	if raw := query.Get("to"); raw != "" {
		end, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || end < from || end > to+60 {
			problem(w, http.StatusBadRequest, "Invalid history end time.")
			return
		}
		to = end
	}
	keys := query["series"]
	if len(keys) == 0 || len(keys) > 8 {
		problem(w, http.StatusBadRequest, "Select between one and eight signals.")
		return
	}
	series := make(map[string][]point, len(keys))
	for _, key := range keys {
		if len(key) > 100 {
			problem(w, http.StatusBadRequest, "Invalid signal identifier.")
			return
		}
		points, err := ui.store.history(r.Context(), key, from, to)
		if err != nil {
			ui.serverError(w, "read history", err)
			return
		}
		if ui.outdoorWeatherFahrenheit && (key == "operating_data.outdoor_temp" || key == "advanced_data.outdoor_temp") {
			for i := range points {
				points[i].Value = fahrenheitToCelsius(points[i].Value)
			}
		}
		if key == "advanced_data."+pelletUse24Hours {
			for i := range points {
				points[i].Value = deciKilogramsToKg(points[i].Value)
			}
		}
		series[key] = points
	}
	writeJSON(w, series)
}

func fahrenheitToCelsius(value float64) float64 {
	return (value - 32) * 5 / 9
}

const pelletUse24Hours = "total_consumption_24hours"

func deciKilogramsToKg(value float64) float64 {
	return value / 10
}

func (ui *UI) performanceHistoryHandler(oxygenOnly bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series, err := ui.store.performanceHistory(r.Context(), time.Now(), !oxygenOnly)
		if err != nil {
			ui.serverError(w, "read performance history", err)
			return
		}
		if oxygenOnly {
			delete(series, "operating_data.boiler_temp")
			delete(series, "operating_data.boiler_ref")
		}
		writeJSON(w, series)
	}
}

func (ui *UI) getEvents(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	switch filter {
	case "", "notices", "unread", "alarm", "info", "connection", "fuel", "state", "command":
		ui.listEvents(w, r, filter)
	default:
		problem(w, http.StatusBadRequest, "Unknown event filter.")
	}
}

func (ui *UI) getNotices(w http.ResponseWriter, r *http.Request) {
	ui.listEvents(w, r, "notices")
}

func (ui *UI) listEvents(w http.ResponseWriter, r *http.Request, filter string) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	events, err := ui.store.events(r.Context(), before, filter)
	if err != nil {
		ui.serverError(w, "read events", err)
		return
	}
	writeJSON(w, events)
}

func (ui *UI) markRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		problem(w, http.StatusBadRequest, "Invalid event identifier.")
		return
	}
	result, err := ui.store.db.ExecContext(r.Context(), "UPDATE events SET seen = 1 WHERE id = ?", id)
	if err != nil {
		ui.serverError(w, "acknowledge event", err)
		return
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		problem(w, http.StatusNotFound, "Event not found.")
		return
	}
	ui.signalEventChange()
	w.WriteHeader(http.StatusNoContent)
}

func (ui *UI) serverError(w http.ResponseWriter, action string, err error) {
	ui.logger.Warn("Web request failed", "action", action, "error", err)
	problem(w, http.StatusInternalServerError, "The local data could not be loaded.")
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func problem(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		problem(w, http.StatusUnsupportedMediaType, "Expected a JSON request.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) {
		problem(w, http.StatusBadRequest, "Invalid or incomplete request.")
		return false
	}
	return true
}

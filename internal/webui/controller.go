package webui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
)

var decimalValue = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)
var settingKey = regexp.MustCompile(`^[a-z0-9_]+$`)

var weekdays = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

type settingRange struct {
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Default  float64 `json:"default"`
	Decimals int     `json:"decimals"`
}

type field struct {
	Key     string          `json:"key"`
	Value   any             `json:"value"`
	Label   string          `json:"label"`
	Help    string          `json:"help,omitempty"`
	Range   *settingRange   `json:"range,omitempty"`
	Options []settingOption `json:"options,omitempty"`
	Timer   bool            `json:"timer,omitempty"`
	Fixed   bool            `json:"fixed,omitempty"`
}

type settingChange struct {
	Current string `json:"current"`
	Value   string `json:"value"`
}

func visibleSetting(key string) bool {
	switch key {
	case "rsa_key", "xtea_key", "dl_version", "push_version", "push_prog_size", "push_picture", "commit_picture", "dummy":
		return false
	default:
		return true
	}
}

func (ui *UI) getSettings(w http.ResponseWriter, r *http.Request) {
	group := r.PathValue("group")
	if !slices.Contains(nbe.Settings, group) {
		problem(w, http.StatusNotFound, "Unknown settings group.")
		return
	}
	ui.mu.RLock()
	current := ui.snapshots[group]
	ui.mu.RUnlock()

	values := current.Values
	if time.Since(current.At) > 20*time.Second {
		response, err := ui.controller.Get(r.Context(), nbe.GetSetupFunction, group+".*")
		if err != nil {
			problem(w, http.StatusServiceUnavailable, "The controller did not return this settings group.")
			return
		}
		values = response.Payload
	}
	ranges, err := ui.controller.Get(r.Context(), nbe.GetSetupRangeFunction, group+".*")
	if err != nil {
		ranges = &nbe.NBEResponse{}
	}
	expanded, roles := ui.settingHardware()
	fields := []field{}
	for key, value := range values {
		if !settingKey.MatchString(key) || !visibleSetting(key) || group == "misc" && (key == "start" || key == "stop" || key == "reset_alarm") {
			continue
		}
		label, help := settingDescription(group, key)
		bounds, trip2Fallback := settingBounds(group, key, ranges.Payload)
		if trip2Fallback {
			help += " Uses StokerCloud's edit range when the controller reports 0–0 kg."
		}
		fixed := bounds != nil && bounds.Min == bounds.Max
		if fixed {
			bounds = nil
		}
		item := field{Key: key, Value: value, Label: label, Help: help, Range: bounds, Timer: isTimer(group, key), Fixed: fixed}
		if !item.Timer {
			item.Options = settingOptionKind(group+"."+key, bounds).options(bounds, expanded, roles)
		}
		fields = append(fields, item)
	}
	slices.SortFunc(fields, func(a, b field) int { return strings.Compare(a.Label, b.Label) })
	writeJSON(w, fields)
}

func readRange(payload map[string]any, key string) *settingRange {
	record, ok := payload[key].(map[string]any)
	if !ok {
		return nil
	}
	minimum, hasMin := number(record["min"])
	maximum, hasMax := number(record["max"])
	defaultValue, _ := number(record["default"])
	decimals, _ := record["decimals"].(int64)
	if !hasMin || !hasMax || minimum > maximum || decimals < 0 || decimals > 6 {
		return nil
	}
	return &settingRange{Min: minimum, Max: maximum, Default: defaultValue, Decimals: int(decimals)}
}

func settingBounds(group, key string, payload map[string]any) (*settingRange, bool) {
	bounds := readRange(payload, key)
	// StokerCloud allows editing Trip 2 even when this firmware advertises a zero range.
	if group == "hopper" && key == "trip2" && bounds != nil && *bounds == (settingRange{}) {
		return &settingRange{Max: 30000}, true
	}
	return bounds, false
}

func isTimer(group, key string) bool {
	return (group == "boiler" || group == "hot_water") && slices.Contains(weekdays, key)
}

func validSettingValue(value string, bounds *settingRange, timer bool) bool {
	if bounds == nil || bounds.Min >= bounds.Max || len(value) > 24 || !decimalValue.MatchString(value) {
		return false
	}
	numeric, err := strconv.ParseFloat(value, 64)
	if err != nil || numeric < bounds.Min || numeric > bounds.Max {
		return false
	}
	_, fraction, _ := strings.Cut(value, ".")
	if len(fraction) > bounds.Decimals {
		return false
	}
	if timer {
		maxSlot := byte(strconv.FormatInt(int64(bounds.Max), 10)[0])
		for _, digit := range []byte(value) {
			if digit < '0' || digit > maxSlot {
				return false
			}
		}
		return len(value) <= 12
	}
	return true
}

func (ui *UI) putSetting(w http.ResponseWriter, r *http.Request) {
	if !ui.allowControl(w) {
		return
	}
	group, key := r.PathValue("group"), r.PathValue("key")
	if !slices.Contains(nbe.Settings, group) || !settingKey.MatchString(key) || !visibleSetting(key) ||
		group == "misc" && (key == "start" || key == "stop" || key == "reset_alarm") {
		problem(w, http.StatusNotFound, "Unknown or reserved controller setting.")
		return
	}
	var change settingChange
	if !readJSON(w, r, &change) {
		return
	}

	ui.settingsMu.Lock()
	defer ui.settingsMu.Unlock()
	path := group + "." + key
	current, err := ui.controller.Get(r.Context(), nbe.GetSetupFunction, path)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Could not read the current controller setting.")
		return
	}
	actual, ok := current.Payload[key]
	if !ok {
		problem(w, http.StatusNotFound, "The controller did not return this setting.")
		return
	}
	if !sameSettingValue(change.Current, actual) {
		problem(w, http.StatusConflict, "The setting changed on the controller. Reload it before applying.")
		return
	}
	ranges, err := ui.controller.Get(r.Context(), nbe.GetSetupRangeFunction, path)
	timer := isTimer(group, key)
	var bounds *settingRange
	var trip2Fallback bool
	if err == nil {
		bounds, trip2Fallback = settingBounds(group, key, ranges.Payload)
	}
	if !validSettingValue(change.Value, bounds, timer) {
		problem(w, http.StatusUnprocessableEntity, "The value is outside the controller's editable range.")
		return
	}
	if sameSettingValue(change.Value, actual) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !timer {
		kind := settingOptionKind(path, bounds)
		expanded := false
		if kind.usesExpansion() {
			response, err := ui.controller.Get(r.Context(), nbe.GetSetupFunction, "misc.expansion_module")
			expanded = err == nil && sameSettingValue("1", response.Payload["expansion_module"])
		}
		options := kind.options(bounds, expanded, nil)
		if len(options) > 0 && !slices.ContainsFunc(options, func(option settingOption) bool {
			return !option.Disabled && option.Value == change.Value
		}) {
			problem(w, http.StatusUnprocessableEntity, "Choose an option supported by this controller.")
			return
		}
	}
	if !ui.sendCommand(w, r, path, change.Value) {
		return
	}
	if trip2Fallback && !ui.verifyTrip2(w, r, change.Value) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ui *UI) verifyTrip2(w http.ResponseWriter, r *http.Request, value string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	ui.mu.Lock()
	hopper := ui.snapshots["hopper"]
	hopper.At = time.Time{}
	ui.snapshots["hopper"] = hopper
	ui.mu.Unlock()
	response, err := ui.controller.Get(ctx, nbe.GetSetupFunction, "hopper.trip2")
	if err == nil {
		if reported, ok := response.Payload["trip2"]; ok {
			if sameSettingValue(value, reported) {
				return true
			}
			problem(w, http.StatusConflict, fmt.Sprintf("The controller acknowledged the change, but Trip 2 still reads %v kg.", reported))
			return false
		}
	}
	problem(w, http.StatusServiceUnavailable, "The controller acknowledged the change, but Boiler Mate could not confirm the new Trip 2 value.")
	return false
}

func sameSettingValue(input string, actual any) bool {
	if number, ok := number(actual); ok && decimalValue.MatchString(input) {
		value, err := strconv.ParseFloat(input, 64)
		if err != nil {
			return false
		}
		if rounded, ok := actual.(nbe.RoundedFloat); ok {
			return rounded.Equal(nbe.RoundedFloat(value))
		}
		return value == number
	}
	return input == fmt.Sprint(actual)
}

func (ui *UI) postPower(w http.ResponseWriter, r *http.Request) {
	if !ui.allowControl(w) {
		return
	}
	var request struct {
		Action string `json:"action"`
	}
	if !readJSON(w, r, &request) {
		return
	}
	switch request.Action {
	case "start", "stop":
		ui.send(w, r, "misc."+request.Action, "1")
	default:
		problem(w, http.StatusBadRequest, "Choose Start or Stop.")
	}
}

func (ui *UI) resetAlarm(w http.ResponseWriter, r *http.Request) {
	if ui.allowControl(w) {
		ui.send(w, r, "misc.reset_alarm", "1")
	}
}

func (ui *UI) allowControl(w http.ResponseWriter) bool {
	if ui.write != nil {
		ui.mu.RLock()
		at := ui.snapshots["operating_data"].At
		ui.mu.RUnlock()
		if time.Since(at) < 20*time.Second {
			return true
		}
		problem(w, http.StatusServiceUnavailable, "Controller readings are stale. Wait for the connection to recover.")
		return false
	}
	problem(w, http.StatusForbidden, "The web interface is in read-only mode.")
	return false
}

func (ui *UI) send(w http.ResponseWriter, r *http.Request, key, value string) {
	if ui.sendCommand(w, r, key, value) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (ui *UI) sendCommand(w http.ResponseWriter, r *http.Request, key, value string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := ui.write(ctx, key, []byte(value)); err != nil {
		var rejected *nbe.ResponseError
		if errors.As(err, &rejected) {
			problem(w, http.StatusUnprocessableEntity, rejected.Error())
			return false
		}
		ui.logger.Warn("Controller did not acknowledge web command", "setting", key, "error", err)
		problem(w, http.StatusServiceUnavailable, "The controller did not acknowledge the command. Check the current value before retrying.")
		return false
	}
	ui.addEvent(ctx, time.Now(), "command", "Controller acknowledged "+strings.ReplaceAll(key, "_", " ")+".")
	return true
}

func (ui *UI) getConsumption(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	switch period {
	case "hours", "days", "months", "years":
	default:
		problem(w, http.StatusBadRequest, "Choose hours, days, months, or years.")
		return
	}
	total, err := ui.consumption(r.Context(), "total_"+period)
	if err != nil && r.Context().Err() == nil {
		// The controller sometimes misses a read during periodic polling.
		timer := time.NewTimer(150 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			total, err = ui.consumption(r.Context(), "total_"+period)
		case <-r.Context().Done():
		}
	}
	if err != nil {
		ui.logger.Debug("Could not read pellet consumption", "period", period, "error", err)
		problem(w, http.StatusServiceUnavailable, "The controller did not return pellet consumption for this period.")
		return
	}
	hotWater, err := ui.consumption(r.Context(), "dhw_"+period)
	if err != nil || len(hotWater) != len(total) {
		hotWater = nil
	}
	counter, _ := ui.consumption(r.Context(), "counter")
	writeJSON(w, map[string]any{"period": period, "total": total, "hotWater": hotWater, "counter": counter})
}

func (ui *UI) consumption(ctx context.Context, key string) ([]float64, error) {
	reply, err := ui.controller.Get(ctx, nbe.GetConsumptionDataFunction, key)
	if err != nil {
		return nil, err
	}
	value, ok := reply.Payload[key]
	if !ok {
		return nil, errors.New("missing consumption data")
	}
	parts := strings.Split(strings.TrimSuffix(fmt.Sprint(value), ","), ",")
	result := make([]float64, len(parts))
	for index, field := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("invalid consumption data")
		}
		result[index] = value
	}
	return result, nil
}

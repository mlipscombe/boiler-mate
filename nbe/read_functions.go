/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021-2026 Mark Lipscombe.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, version 3.
 *
 * This program is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
 * General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package nbe

// Read-only functions 7, 8, 10, 11 and 12.
//
// Their request and reply layouts were read from the official NBE Android app
// (1.176). None of them has been exchanged with a real controller yet, so the
// parsers skip records they do not recognise and report a ReplyFormatError
// when a reply holds nothing they recognise. They never panic on a reply.
//
// Each function has a request builder and a reply parser that work on plain
// strings, for callers that only hold a Get method, and a typed method on NBE
// that combines the two.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const maxPayloadSize = 999

// ReplyFormatError reports a status-0 reply whose payload did not have the
// layout the app expects. Like ResponseError it leaves the payload out.
type ReplyFormatError struct {
	Function Function
	Reason   string
}

func (err *ReplyFormatError) Error() string {
	return fmt.Sprintf("controller reply to function %d was not understood: %s", err.Function, err.Reason)
}

// splitRecords splits a reply on ';' and drops empty records, so an empty
// reply and a trailing separator both add nothing.
func splitRecords(payload string) []string {
	var records []string
	for record := range strings.SplitSeq(payload, ";") {
		if record = strings.TrimSpace(record); record != "" {
			records = append(records, record)
		}
	}
	return records
}

func splitFields(record string) []string {
	fields := strings.Split(record, ",")
	for index, field := range fields {
		fields[index] = strings.TrimSpace(field)
	}
	return fields
}

// recordsResult turns "nothing matched" into an error and otherwise returns
// the records that did, as a non-nil slice.
func recordsResult[T any](function Function, records []T, malformed int) ([]T, error) {
	if len(records) == 0 && malformed > 0 {
		return nil, &ReplyFormatError{Function: function,
			Reason: fmt.Sprintf("none of %d records had the expected layout", malformed)}
	}
	if records == nil {
		records = []T{}
	}
	return records, nil
}

func parseFiniteFloat(field string) (float64, bool) {
	value, err := strconv.ParseFloat(field, 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// Function 11, custom request.

// CustomRequestPayload builds a function 11 request for the named operating
// values, for example "boiler_temp" and "boiler_ref".
func CustomRequestPayload(names []string) (string, error) {
	if len(names) == 0 {
		return "", errors.New("a custom request needs at least one value name")
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !validValueName(name) {
			return "", fmt.Errorf("invalid custom request value name %q", name)
		}
		// The reply is paired by position, so a repeated name adds nothing.
		if _, repeated := seen[name]; repeated {
			return "", fmt.Errorf("custom request value name %q is repeated", name)
		}
		seen[name] = struct{}{}
	}
	payload := strings.Join(names, ",")
	if len(payload) > maxPayloadSize {
		return "", errors.New("custom request exceeds the NBE payload limit")
	}
	return payload, nil
}

func validValueName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range []byte(name) {
		if char <= ' ' || char > '~' || char == ',' || char == ';' || char == '=' {
			return false
		}
	}
	return true
}

// ParseCustomValues pairs a function 11 reply with the requested names. The
// reply is the bare values in request order, not key=value pairs, so a reply
// with a different number of values is an error and nothing is paired.
func ParseCustomValues(names []string, payload string) (map[string]any, error) {
	payload = strings.TrimSuffix(strings.TrimSpace(payload), ";")
	if payload == "" {
		return nil, &ReplyFormatError{Function: CustomRequestFunction, Reason: "the reply was empty"}
	}
	fields := splitFields(payload)
	// Tolerate one trailing separator.
	if len(fields) == len(names)+1 && fields[len(names)] == "" {
		fields = fields[:len(names)]
	}
	if len(fields) != len(names) {
		return nil, &ReplyFormatError{Function: CustomRequestFunction,
			Reason: fmt.Sprintf("%d values were returned for %d names", len(fields), len(names))}
	}
	values := make(map[string]any, len(names))
	for index, name := range names {
		values[name] = parseValue(fields[index])
	}
	return values, nil
}

// CustomRequest reads the named operating values in one request. It is the
// app's compact alternative to reading everything with function 4.
func (client *NBE) CustomRequest(ctx context.Context, names ...string) (map[string]any, error) {
	payload, err := CustomRequestPayload(names)
	if err != nil {
		return nil, err
	}
	reply, err := client.Get(ctx, CustomRequestFunction, payload)
	if err != nil {
		return nil, err
	}
	return ParseCustomValues(names, reply.RawPayload)
}

// Function 8, controller event log.

// ControllerEvent is one record of the controller's own event log.
//
// The app shows type 0 as a changed setting: Fields[0] is a numeric id it
// looks up in its own tables, and Fields[1] and Fields[2] are the old and new
// values. It also handles types 1, 2, 4, 6, 7 and 8, each using the fields
// differently. Their meaning is unverified against hardware, so the fields
// are kept as the controller sent them.
type ControllerEvent struct {
	Date  string `json:"date"`  // yyMMdd, controller clock
	Clock string `json:"clock"` // HHmmss, controller clock
	// At combines Date and Clock. It is zero when they are not a valid time.
	At     time.Time `json:"-"`
	Type   int       `json:"type"`
	Fields [3]string `json:"fields"`
	// Unit is the seventh field, which the app stores as the event's unit.
	Unit string `json:"unit,omitempty"`
}

// EventLogRequest builds a function 8 request. The controller's clock has no
// zone, so at is formatted in its own location.
//
// Unverified against hardware: whether the controller returns the events
// before or after this time, and how many per reply. The app pages in both
// directions with the same request.
func EventLogRequest(at time.Time) string {
	return at.Format("060102-150405")
}

// ParseEventLog parses a function 8 reply. Controller times are read in
// location, or time.Local when it is nil.
func ParseEventLog(payload string, location *time.Location) ([]ControllerEvent, error) {
	if location == nil {
		location = time.Local
	}
	var events []ControllerEvent
	malformed := 0
	for _, record := range splitRecords(payload) {
		fields := splitFields(record)
		// The app drops events whose text is "*". That text comes from its own
		// lookup tables, but a bare "*" on the wire carries no event either.
		if record == "*" || len(fields) > 3 && fields[3] == "*" {
			continue
		}
		if len(fields) < 6 {
			malformed++
			continue
		}
		kind, err := strconv.Atoi(fields[2])
		if err != nil {
			malformed++
			continue
		}
		event := ControllerEvent{Date: fields[0], Clock: fields[1], Type: kind,
			Fields: [3]string{fields[3], fields[4], fields[5]}}
		if at, err := time.ParseInLocation("060102150405", fields[0]+fields[1], location); err == nil {
			event.At = at
		}
		if len(fields) > 6 {
			event.Unit = fields[6]
		}
		events = append(events, event)
	}
	return recordsResult(GetEventLogFunction, events, malformed)
}

// EventLog reads the controller's event log around at. See EventLogRequest
// for what is unverified about the direction.
func (client *NBE) EventLog(ctx context.Context, at time.Time) ([]ControllerEvent, error) {
	reply, err := client.Get(ctx, GetEventLogFunction, EventLogRequest(at))
	if err != nil {
		return nil, err
	}
	return ParseEventLog(reply.RawPayload, at.Location())
}

// Function 7, chart history.

// ChartRequest selects stored chart history for part of one day.
type ChartRequest struct {
	// From and To bound the interval. They must fall on the same calendar
	// day; the app sends one request per day when a range crosses midnight.
	From, To time.Time
	// Step is the spacing of the returned samples, which the app treats as
	// minutes. 0 and 1 both mean the controller's default.
	Step int
	// IDs are the chart ids, in ascending order as the app sends them. Their
	// meaning is unverified against hardware.
	IDs []int
	// StartFirst sends From before To. The app sends the end of the interval
	// first, which is the default here. Unverified against hardware: set
	// this if a controller turns out to expect the start first.
	StartFirst bool
}

// Payload builds the function 7 request.
func (request ChartRequest) Payload() (string, error) {
	if request.From.IsZero() || request.To.IsZero() || request.To.Before(request.From) {
		return "", errors.New("chart request needs a start time that is not after its end time")
	}
	from := request.From.In(request.To.Location())
	if from.Year() != request.To.Year() || from.YearDay() != request.To.YearDay() {
		return "", errors.New("chart request must not cross midnight")
	}
	if request.Step < 0 {
		return "", errors.New("chart request step must not be negative")
	}
	if len(request.IDs) == 0 {
		return "", errors.New("chart request needs at least one chart id")
	}
	first, second := request.To, from
	if request.StartFirst {
		first, second = from, request.To
	}
	var payload strings.Builder
	payload.WriteString(request.To.Format("060102"))
	payload.WriteString(first.Format(",150405"))
	payload.WriteString(second.Format(",150405"))
	if request.Step > 1 {
		fmt.Fprintf(&payload, ",step=%d", request.Step)
	}
	for index, id := range request.IDs {
		// Values come back in request order and the app only ever asks in
		// ascending order, so any other order could be paired wrongly.
		if id < 0 || index > 0 && id <= request.IDs[index-1] {
			return "", errors.New("chart ids must be non-negative and in ascending order")
		}
		fmt.Fprintf(&payload, ",%d", id)
	}
	payload.WriteByte(';')
	if payload.Len() > maxPayloadSize {
		return "", errors.New("chart request exceeds the NBE payload limit")
	}
	return payload.String(), nil
}

// ChartSample is one record of a function 7 reply.
type ChartSample struct {
	// RawTime is the record's time field as the controller sent it.
	RawTime string `json:"rawTime"`
	// At is the sample time, or zero when RawTime could not be resolved. See
	// ParseChartData.
	At time.Time `json:"-"`
	// Values holds one value per requested chart id, in request order.
	Values []float64 `json:"values"`
}

// ParseChartData parses the reply to request.
//
// The app resolves the time field of each record in one of three ways: six or
// more characters are a clock time HHmmss on the requested day; a shorter
// integer, or "time=N", is a number of seconds before the previous record;
// and an empty field is one step before the previous record. Records
// therefore run from newest to oldest. Unverified against hardware: which of
// these forms a controller sends. A relative time with no resolved record
// before it is left with a zero At.
func ParseChartData(payload string, request ChartRequest) ([]ChartSample, error) {
	step := time.Duration(max(request.Step, 1)) * time.Minute
	var samples []ChartSample
	var previous time.Time
	malformed := 0
records:
	for _, record := range splitRecords(payload) {
		fields := splitFields(record)
		if len(request.IDs) == 0 || len(fields) < 1+len(request.IDs) {
			malformed++
			continue
		}
		sample := ChartSample{RawTime: fields[0], Values: make([]float64, len(request.IDs))}
		for index := range request.IDs {
			value, ok := parseFiniteFloat(fields[1+index])
			if !ok {
				malformed++
				continue records
			}
			sample.Values[index] = value
		}
		sample.At = chartTime(fields[0], previous, request.To, step)
		// An unresolved time also leaves later relative times unresolved.
		previous = sample.At
		samples = append(samples, sample)
	}
	return recordsResult(GetChartDataFunction, samples, malformed)
}

func chartTime(field string, previous, day time.Time, step time.Duration) time.Time {
	seconds, relative := strings.CutPrefix(field, "time=")
	switch {
	case field == "":
		if previous.IsZero() {
			return time.Time{}
		}
		return previous.Add(-step)
	case relative || len(field) < 6:
		count, err := strconv.Atoi(seconds)
		if err != nil || count < 0 || previous.IsZero() {
			return time.Time{}
		}
		return previous.Add(-time.Duration(count) * time.Second)
	case day.IsZero():
		return time.Time{}
	}
	clock, err := time.Parse("150405", field[:6])
	if err != nil {
		return time.Time{}
	}
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, day.Location())
}

// ChartData reads stored chart history. See ChartRequest and ParseChartData
// for what is unverified.
func (client *NBE) ChartData(ctx context.Context, request ChartRequest) ([]ChartSample, error) {
	payload, err := request.Payload()
	if err != nil {
		return nil, err
	}
	reply, err := client.Get(ctx, GetChartDataFunction, payload)
	if err != nil {
		return nil, err
	}
	return ParseChartData(reply.RawPayload, request)
}

// Function 10, version list.

// ProgramVersion is one firmware version the controller says it can fetch.
// The fields are kept as text; the app's own sample is "150203", "0616", "0".
type ProgramVersion struct {
	Date        string `json:"date"` // yyMMdd
	Version     string `json:"version"`
	ReleaseType string `json:"releaseType"`
}

// AvailableProgramsRequest is the function 10 request payload.
const AvailableProgramsRequest = "*"

// ParseAvailablePrograms parses a function 10 reply.
func ParseAvailablePrograms(payload string) ([]ProgramVersion, error) {
	var versions []ProgramVersion
	malformed := 0
	for _, record := range splitRecords(payload) {
		fields := splitFields(record)
		if len(fields) < 3 || fields[1] == "" {
			malformed++
			continue
		}
		versions = append(versions, ProgramVersion{Date: fields[0], Version: fields[1], ReleaseType: fields[2]})
	}
	return recordsResult(GetAvailableProgramsFunction, versions, malformed)
}

// AvailablePrograms lists the firmware versions the controller offers. It
// only reads the list; nothing here selects or downloads a version.
func (client *NBE) AvailablePrograms(ctx context.Context) ([]ProgramVersion, error) {
	reply, err := client.Get(ctx, GetAvailableProgramsFunction, AvailableProgramsRequest)
	if err != nil {
		return nil, err
	}
	return ParseAvailablePrograms(reply.RawPayload)
}

// Function 12, remote sensor read.

// RemoteSensor is one wireless room sensor known to the controller.
type RemoteSensor struct {
	MAC         string  `json:"mac"`
	Temperature float64 `json:"temperature"`
	Battery     float64 `json:"battery"`
	Timeout     int     `json:"timeout"`
	// SecondsSinceReading is the age of Temperature.
	SecondsSinceReading int `json:"secondsSinceReading"`
	// Unknown is the sixth field, kept as sent. The app does not read it and
	// its meaning is unverified against hardware.
	Unknown string `json:"unknown,omitempty"`
	// TransmissionsSent is the seventh field, when the controller sends it.
	TransmissionsSent *int `json:"transmissionsSent,omitempty"`
}

// RemoteSensorsRequest is the function 12 request payload.
const RemoteSensorsRequest = "*"

// ParseRemoteSensors parses a function 12 reply.
func ParseRemoteSensors(payload string) ([]RemoteSensor, error) {
	var sensors []RemoteSensor
	malformed := 0
	for _, record := range splitRecords(payload) {
		fields := splitFields(record)
		if len(fields) < 5 || fields[0] == "" {
			malformed++
			continue
		}
		temperature, temperatureOK := parseFiniteFloat(fields[1])
		battery, batteryOK := parseFiniteFloat(fields[2])
		timeout, timeoutErr := strconv.Atoi(fields[3])
		age, ageErr := strconv.Atoi(fields[4])
		if !temperatureOK || !batteryOK || timeoutErr != nil || ageErr != nil {
			malformed++
			continue
		}
		sensor := RemoteSensor{MAC: fields[0], Temperature: temperature, Battery: battery,
			Timeout: timeout, SecondsSinceReading: age}
		if len(fields) > 5 {
			sensor.Unknown = fields[5]
		}
		if len(fields) > 6 {
			// Like the app, keep the sensor when only this count is unreadable.
			if sent, err := strconv.Atoi(fields[6]); err == nil {
				sensor.TransmissionsSent = &sent
			}
		}
		sensors = append(sensors, sensor)
	}
	return recordsResult(GetRemoteSensorsFunction, sensors, malformed)
}

// RemoteSensors lists the controller's wireless room sensors.
func (client *NBE) RemoteSensors(ctx context.Context) ([]RemoteSensor, error) {
	reply, err := client.Get(ctx, GetRemoteSensorsFunction, RemoteSensorsRequest)
	if err != nil {
		return nil, err
	}
	return ParseRemoteSensors(reply.RawPayload)
}

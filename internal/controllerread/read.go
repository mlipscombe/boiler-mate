// Package controllerread exposes the controller's optional read functions to HTTP and MQTT.
package controllerread

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
)

type Controller interface {
	Get(context.Context, nbe.Function, string) (*nbe.NBEResponse, error)
}

type ReadFunc func(context.Context, string, url.Values) (any, error)

var ErrQuery = errors.New("invalid controller read")
var ErrKind = errors.New("unknown controller read")

func Known(kind string) bool {
	switch kind {
	case "events", "versions", "sensors", "values", "chart":
		return true
	}
	return false
}

type Event struct {
	Time int64 `json:"time"`
	nbe.ControllerEvent
}

type Sample struct {
	Time int64 `json:"time,omitempty"`
	nbe.ChartSample
}

func unixTime(text string) (time.Time, error) {
	seconds, err := strconv.ParseInt(text, 10, 64)
	if err != nil || seconds <= 0 || seconds > time.Now().Unix()+60 {
		return time.Time{}, fmt.Errorf("%w: choose a time in the past", ErrQuery)
	}
	return time.Unix(seconds, 0), nil
}

func fetch[T any](ctx context.Context, c Controller, fn nbe.Function, payload string, parse func(string) (T, error)) (any, error) {
	reply, err := c.Get(ctx, fn, payload)
	if err != nil {
		return nil, err
	}
	return parse(reply.RawPayload)
}

func Read(ctx context.Context, c Controller, kind string, q url.Values) (any, error) {
	switch kind {
	case "versions":
		return fetch(ctx, c, nbe.GetAvailableProgramsFunction, nbe.AvailableProgramsRequest, nbe.ParseAvailablePrograms)
	case "sensors":
		return fetch(ctx, c, nbe.GetRemoteSensorsFunction, nbe.RemoteSensorsRequest, nbe.ParseRemoteSensors)
	case "events":
		at := time.Now()
		if raw := q.Get("at"); raw != "" {
			var err error
			if at, err = unixTime(raw); err != nil {
				return nil, err
			}
		}
		return fetch(ctx, c, nbe.GetEventLogFunction, nbe.EventLogRequest(at), func(raw string) ([]Event, error) {
			events, err := nbe.ParseEventLog(raw, time.Local)
			rows := make([]Event, len(events))
			for i, event := range events {
				rows[i].ControllerEvent = event
				if !event.At.IsZero() {
					rows[i].Time = event.At.Unix()
				}
			}
			return rows, err
		})
	case "values":
		names := strings.Split(q.Get("names"), ",")
		payload, err := nbe.CustomRequestPayload(names)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQuery, err)
		}
		values, err := fetch(ctx, c, nbe.CustomRequestFunction, payload, func(raw string) (map[string]any, error) {
			return nbe.ParseCustomValues(names, raw)
		})
		var rejected *nbe.ResponseError
		if !errors.As(err, &rejected) {
			return values, err
		}
		// Some controllers only expose operating values through function 4.
		// Take one fresh snapshot so the selected fields describe the same instant.
		reply, err := c.Get(ctx, nbe.GetOperatingDataFunction, "*")
		if err != nil {
			return nil, err
		}
		selected := make(map[string]any, len(names))
		for _, name := range names {
			value, ok := reply.Payload[name]
			if !ok {
				return nil, fmt.Errorf("%w: the controller did not return %q", ErrQuery, name)
			}
			selected[name] = value
		}
		return selected, nil
	case "chart":
		request, err := chartRequest(q)
		if err != nil {
			return nil, err
		}
		payload, err := request.Payload()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQuery, err)
		}
		return fetch(ctx, c, nbe.GetChartDataFunction, payload, func(raw string) ([]Sample, error) {
			samples, err := nbe.ParseChartData(raw, request)
			rows := make([]Sample, len(samples))
			for i, sample := range samples {
				rows[i].ChartSample = sample
				if !sample.At.IsZero() {
					rows[i].Time = sample.At.Unix()
				}
			}
			return rows, err
		})
	default:
		return nil, ErrKind
	}
}

func chartRequest(q url.Values) (nbe.ChartRequest, error) {
	var request nbe.ChartRequest
	var err error
	if request.From, err = unixTime(q.Get("from")); err != nil {
		return request, err
	}
	if request.To, err = unixTime(q.Get("to")); err != nil {
		return request, err
	}
	if step := q.Get("step"); step != "" {
		if request.Step, err = strconv.Atoi(step); err != nil || request.Step < 0 || request.Step > 1440 {
			return request, fmt.Errorf("%w: chart step must be 0–1440 minutes", ErrQuery)
		}
	}
	for rawID := range strings.SplitSeq(q.Get("ids"), ",") {
		id, err := strconv.Atoi(rawID)
		if err != nil {
			return request, fmt.Errorf("%w: enter numeric chart IDs", ErrQuery)
		}
		request.IDs = append(request.IDs, id)
	}
	return request, nil
}

func Message(kind string, err error) string {
	if errors.Is(err, ErrQuery) || errors.Is(err, ErrKind) {
		return err.Error()
	}
	name := map[string]string{"events": "event log", "versions": "version list", "sensors": "remote sensor list", "values": "selected readings", "chart": "chart history"}[kind]
	var format *nbe.ReplyFormatError
	if errors.As(err, &format) {
		return "The controller's " + name + " was not in the expected format."
	}
	var rejection *nbe.ResponseError
	if errors.As(err, &rejection) {
		return fmt.Sprintf("The controller declined the %s request (status %d).", name, rejection.Status)
	}
	return "The controller did not return its " + name + "."
}

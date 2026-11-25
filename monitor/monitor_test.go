/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021-2023 Mark Lipscombe.
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

package monitor

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus"
)

func TestOperatingState(t *testing.T) {
	for _, tc := range []struct {
		state         int64
		text, enabled string
	}{
		{14, "Off", "OFF"},
		{5, "Power", "ON"},
		{9, "Stopped - temperature reached", "ON"},
		{23, "Stopped by timer", "ON"},
		{8, "Temperature error boiler", "None"},
		{13, "Fault ignition", "None"},
		{18, "Unknown (18)", "None"},
		{35, "Unknown (35)", "None"},
		{36, "High backpressure alarm. Check the boiler and chimney.", "None"},
		{40, "Calibrating airflow", "None"},
		{45, "The condensate collector is full.", "None"},
		{-1, "Unknown (-1)", "None"},
		{200, "Unknown (200)", "None"},
	} {
		values := map[string]any{"state": tc.state}
		got := withOperatingState(values)
		if got["state_text"] != tc.text || got["state_on"] != tc.enabled || len(values) != 1 {
			t.Errorf("state %d produced %v", tc.state, got)
		}
	}
	if got := withOperatingState(map[string]any{"boiler_temp": int64(60)}); got["state_on"] != "None" || got["state_text"] != "Unknown" {
		t.Errorf("missing phase produced %v", got)
	}
	if got := withOperatingState(map[string]any{"state_text": "Controller fault"}); got["state_on"] != "None" || got["state_text"] != "Controller fault" {
		t.Errorf("controller-provided phase text produced %v", got)
	}
	if !equalValue(nbe.RoundedFloat(12.001), nbe.RoundedFloat(12.004)) {
		t.Error("equivalent MQTT values would be republished")
	}
}

type pollReaderFunc func() (*nbe.NBEResponse, error)

func (read pollReaderFunc) Get(context.Context, nbe.Function, string) (*nbe.NBEResponse, error) {
	return read()
}

type pollPublisherFunc func(string, map[string]any) error

func (publish pollPublisherFunc) PublishMany(topic string, values map[string]any) error {
	return publish(topic, values)
}

func TestControllerAvailabilityFollowsOperatingPolls(t *testing.T) {
	synctest.Test(t, testControllerAvailability)
}

func testControllerAvailability(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	attempt := 0
	read := pollReaderFunc(func() (*nbe.NBEResponse, error) {
		attempt++
		switch attempt {
		case 1, 4, 7, 9, 11:
			return nil, errors.New("controller did not respond")
		case 2, 5, 8, 10:
			return &nbe.NBEResponse{}, nil
		case 3, 6, 12:
			return &nbe.NBEResponse{Payload: map[string]any{"state": int64(13)}}, nil
		default:
			t.Fatal("unexpected poll after recovery")
			return nil, nil
		}
	})
	type transition struct {
		attempt int
		status  string
	}
	var statuses []transition
	publish := pollPublisherFunc(func(topic string, values map[string]any) error {
		if topic == "device" {
			statuses = append(statuses, transition{attempt, values["controller_status"].(string)})
			if attempt == 12 {
				cancel()
			}
		}
		return nil
	})
	task := poll{function: nbe.GetOperatingDataFunction, path: "*", topic: "operating_data", interval: 5 * time.Second}
	gauges := &metricSet{registry: prometheus.NewRegistry(), gauges: make(map[string]metric)}
	runPoll(ctx, slog.Default(), read, publish, gauges, task, nil, nil)
	if want := []transition{{1, "offline"}, {3, "online"}, {10, "offline"}, {12, "online"}}; !slices.Equal(statuses, want) {
		t.Errorf("controller availability = %v, want %v", statuses, want)
	}
}

func TestOperatingMetricWinsOverAdvancedDuplicate(t *testing.T) {
	registry := prometheus.NewRegistry()
	gauges := &metricSet{registry: registry, serial: "1234", gauges: make(map[string]metric)}
	gauges.set(slog.Default(), "advanced_data", "power_kw", int64(1))
	gauges.set(slog.Default(), "operating_data", "power_kw", nbe.RoundedFloat(2.5))
	gauges.set(slog.Default(), "advanced_data", "power_kw", int64(3))
	result, err := registry.Gather()
	if err != nil || len(result) != 1 {
		t.Fatalf("expected one registered metric, got %d, %v", len(result), err)
	}
	if result[0].GetName() != "boiler_mate_operating_data_power_kw" || result[0].Metric[0].Gauge.GetValue() != 2.5 {
		t.Fatalf("unexpected metric %v", result[0])
	}
}

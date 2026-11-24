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
	"log/slog"
	"testing"

	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus"
)

func TestOperatingState(t *testing.T) {
	for _, tc := range []struct {
		state         int64
		text, enabled string
	}{{14, "Off", "OFF"}, {5, "Power", "ON"}, {-1, "Unknown (-1)", "ON"}, {200, "Unknown (200)", "ON"}} {
		values := map[string]any{"state": tc.state}
		got := withOperatingState(values)
		if got["state_text"] != tc.text || got["state_on"] != tc.enabled || len(values) != 1 {
			t.Errorf("state %d produced %v", tc.state, got)
		}
	}
	if !equalValue(nbe.RoundedFloat(12.001), nbe.RoundedFloat(12.004)) {
		t.Error("equivalent MQTT values would be republished")
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

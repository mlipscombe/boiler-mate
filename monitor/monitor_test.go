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
	"testing"

	"github.com/mlipscombe/boiler-mate/nbe"
)

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"int64", int64(42), true},
		{"float64", float64(3.14), true},
		{"RoundedFloat", nbe.RoundedFloat(2.5), true},
		{"string", "hello", false},
		{"bool", true, false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNumeric(tt.value)
			if result != tt.expected {
				t.Errorf("isNumeric(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestUpdateGauge(t *testing.T) {
	// Test that updateGauge doesn't panic with nil gauge
	updateGauge(nil, "test-serial", int64(42))

	// Test with RoundedFloat
	updateGauge(nil, "test-serial", nbe.RoundedFloat(3.14))

	// Test with string (should be ignored)
	updateGauge(nil, "test-serial", "not a number")
}

func TestStartSettingsMonitor(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

func TestStartOperatingDataMonitor(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

func TestStartAdvancedDataMonitor(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

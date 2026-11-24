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

package main

import (
	"net/url"
	"testing"
)

func TestDetermineMQTTPrefix(t *testing.T) {
	tests := []struct {
		name           string
		mqttURL        string
		serial         string
		expectedPrefix string
	}{
		{
			name:           "URL with path",
			mqttURL:        "mqtt://localhost/custom/prefix",
			serial:         "ABC123",
			expectedPrefix: "custom/prefix",
		},
		{
			name:           "URL without path",
			mqttURL:        "mqtt://localhost",
			serial:         "ABC123",
			expectedPrefix: "nbe/ABC123",
		},
		{
			name:           "URL with root path only",
			mqttURL:        "mqtt://localhost/",
			serial:         "XYZ789",
			expectedPrefix: "nbe/XYZ789",
		},
		{
			name:           "URL with single segment path",
			mqttURL:        "mqtt://localhost/boiler",
			serial:         "TEST123",
			expectedPrefix: "boiler",
		},
		{
			name:           "URL with multi-segment path",
			mqttURL:        "mqtt://localhost/home/automation/boiler",
			serial:         "SERIAL",
			expectedPrefix: "home/automation/boiler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mqttURL, err := url.Parse(tt.mqttURL)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			result := determineMQTTPrefix(mqttURL, tt.serial)
			if result != tt.expectedPrefix {
				t.Errorf("Expected prefix %q, got %q", tt.expectedPrefix, result)
			}
		})
	}
}

func TestParseSetTopic(t *testing.T) {
	tests := []struct {
		name        string
		topic       string
		expectedKey string
	}{
		{
			name:        "standard set topic",
			topic:       "nbe/ABC123/set/boiler/temp",
			expectedKey: "boiler.temp",
		},
		{
			name:        "device power switch",
			topic:       "nbe/ABC123/set/device/power_switch",
			expectedKey: "device.power_switch",
		},
		{
			name:        "regulation category",
			topic:       "prefix/set/regulation/mode",
			expectedKey: "regulation.mode",
		},
		{
			name:        "hopper category",
			topic:       "custom/set/hopper/level",
			expectedKey: "hopper.level",
		},
		{
			name:        "minimal topic",
			topic:       "set/cat/param",
			expectedKey: "cat.param",
		},
		{
			name:        "topic with extra segments",
			topic:       "a/b/c/d/set/category/parameter",
			expectedKey: "category.parameter",
		},
		{
			name:        "empty topic",
			topic:       "",
			expectedKey: "",
		},
		{
			name:        "single segment topic",
			topic:       "single",
			expectedKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSetTopic(tt.topic)
			if result != tt.expectedKey {
				t.Errorf("Expected key %q, got %q", tt.expectedKey, result)
			}
		})
	}
}

func TestTranslatePowerCommand(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		value         []byte
		expectedKey   string
		expectedValue string
	}{
		{
			name:          "power ON",
			key:           "device.power_switch",
			value:         []byte("ON"),
			expectedKey:   "misc.start",
			expectedValue: "1",
		},
		{
			name:          "power 1",
			key:           "device.power_switch",
			value:         []byte("1"),
			expectedKey:   "misc.start",
			expectedValue: "1",
		},
		{
			name:          "power OFF",
			key:           "device.power_switch",
			value:         []byte("OFF"),
			expectedKey:   "misc.stop",
			expectedValue: "1",
		},
		{
			name:          "power 0",
			key:           "device.power_switch",
			value:         []byte("0"),
			expectedKey:   "misc.stop",
			expectedValue: "1",
		},
		{
			name:          "power false",
			key:           "device.power_switch",
			value:         []byte("false"),
			expectedKey:   "misc.stop",
			expectedValue: "1",
		},
		{
			name:          "non-power command unchanged",
			key:           "boiler.temp",
			value:         []byte("75"),
			expectedKey:   "boiler.temp",
			expectedValue: "75",
		},
		{
			name:          "different device command unchanged",
			key:           "device.status",
			value:         []byte("online"),
			expectedKey:   "device.status",
			expectedValue: "online",
		},
		{
			name:          "regulation command unchanged",
			key:           "regulation.mode",
			value:         []byte("auto"),
			expectedKey:   "regulation.mode",
			expectedValue: "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultKey, resultValue := translatePowerCommand(tt.key, tt.value)

			if resultKey != tt.expectedKey {
				t.Errorf("Expected key %q, got %q", tt.expectedKey, resultKey)
			}

			if string(resultValue) != tt.expectedValue {
				t.Errorf("Expected value %q, got %q", tt.expectedValue, string(resultValue))
			}
		})
	}
}

func TestParseSetTopicIntegration(t *testing.T) {
	// Test realistic MQTT topics that would be seen in production
	topics := map[string]string{
		"nbe/BOILER123/set/boiler/temp":         "boiler.temp",
		"nbe/BOILER123/set/boiler/diff_under":   "boiler.diff_under",
		"nbe/BOILER123/set/device/power_switch": "device.power_switch",
		"nbe/BOILER123/set/regulation/mode":     "regulation.mode",
		"custom/prefix/set/hopper/fill_level":   "hopper.fill_level",
		"home/automation/boiler/set/misc/start": "misc.start",
	}

	for topic, expectedKey := range topics {
		t.Run(topic, func(t *testing.T) {
			result := parseSetTopic(topic)
			if result != expectedKey {
				t.Errorf("Topic %q: expected key %q, got %q", topic, expectedKey, result)
			}
		})
	}
}

func TestPowerCommandTranslationFlow(t *testing.T) {
	// Test complete flow: parse topic + translate power command
	testCases := []struct {
		topic         string
		value         string
		expectedKey   string
		expectedValue string
	}{
		{
			topic:         "nbe/ABC/set/device/power_switch",
			value:         "ON",
			expectedKey:   "misc.start",
			expectedValue: "1",
		},
		{
			topic:         "nbe/ABC/set/device/power_switch",
			value:         "OFF",
			expectedKey:   "misc.stop",
			expectedValue: "1",
		},
		{
			topic:         "nbe/ABC/set/boiler/temp",
			value:         "75",
			expectedKey:   "boiler.temp",
			expectedValue: "75",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.topic+" with "+tc.value, func(t *testing.T) {
			// Simulate MQTT message processing
			key := parseSetTopic(tc.topic)
			value := []byte(tc.value)

			// Translate power commands
			key, value = translatePowerCommand(key, value)

			if key != tc.expectedKey {
				t.Errorf("Expected key %q, got %q", tc.expectedKey, key)
			}

			if string(value) != tc.expectedValue {
				t.Errorf("Expected value %q, got %q", tc.expectedValue, string(value))
			}
		})
	}
}

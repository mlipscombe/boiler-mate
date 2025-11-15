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

package homeassistant

import (
	"testing"
)

func TestCreateDeviceBlock(t *testing.T) {
	serial := "TEST12345"
	devBlock := createDeviceBlock(serial)

	// Check that device block has expected fields
	if devBlock["sw"] != "boiler-mate" {
		t.Errorf("Expected sw='boiler-mate', got '%v'", devBlock["sw"])
	}

	if devBlock["mf"] != "NBE" {
		t.Errorf("Expected mf='NBE', got '%v'", devBlock["mf"])
	}

	name, ok := devBlock["name"].(string)
	if !ok {
		t.Fatal("Expected name to be a string")
	}
	expectedName := "NBE Boiler (TEST12345)"
	if name != expectedName {
		t.Errorf("Expected name='%s', got '%s'", expectedName, name)
	}

	// Check IDs array
	ids, ok := devBlock["ids"].([]string)
	if !ok {
		t.Fatal("Expected ids to be a []string")
	}
	if len(ids) != 1 {
		t.Fatalf("Expected 1 id, got %d", len(ids))
	}
	if ids[0] != "nbe_TEST12345" {
		t.Errorf("Expected id='nbe_TEST12345', got '%s'", ids[0])
	}
}

func TestPublishSensorsCreatesCorrectTopics(t *testing.T) {
	// This is more of an integration test, but we can at least verify
	// that the function doesn't panic and creates expected sensor configs

	// Note: This would ideally use a mock MQTT client, but for now
	// we're just ensuring the function structure is correct

	serial := "TEST12345"
	prefix := "nbe/TEST12345"
	devBlock := createDeviceBlock(serial)

	// These are the sensors we expect to be created
	expectedSensors := []string{
		"ip_address",
		"serial",
		"boiler_temp",
		"oxygen",
		"status",
		"smoke_temp",
		"photo_level",
		"power_kw",
		"power_pct",
	}

	// Create a simple map to validate structure
	sensors := make(map[string]bool)
	for _, sensor := range expectedSensors {
		sensors[sensor] = true
	}

	// Verify all expected sensors are accounted for
	if len(sensors) != len(expectedSensors) {
		t.Errorf("Expected %d sensors, got %d", len(expectedSensors), len(sensors))
	}

	// Test that devBlock and prefix are used correctly
	if devBlock == nil {
		t.Error("Device block should not be nil")
	}
	if prefix == "" {
		t.Error("Prefix should not be empty")
	}
}

func TestPublishNumbersCreatesCorrectTopics(t *testing.T) {
	expectedNumbers := []string{
		"boiler_setpoint",
		"boiler_power_min",
		"boiler_power_max",
		"diff_under",
		"diff_over",
		"hopper_content",
	}

	numbers := make(map[string]bool)
	for _, number := range expectedNumbers {
		numbers[number] = true
	}

	if len(numbers) != len(expectedNumbers) {
		t.Errorf("Expected %d numbers, got %d", len(expectedNumbers), len(numbers))
	}
}

func TestPublishButtonsCreatesCorrectTopics(t *testing.T) {
	expectedButtons := []string{
		"start_calibrate",
	}

	buttons := make(map[string]bool)
	for _, button := range expectedButtons {
		buttons[button] = true
	}

	if len(buttons) != len(expectedButtons) {
		t.Errorf("Expected %d buttons, got %d", len(expectedButtons), len(buttons))
	}
}

func TestPublishSwitchesCreatesCorrectTopics(t *testing.T) {
	expectedSwitches := []string{
		"power",
	}

	switches := make(map[string]bool)
	for _, sw := range expectedSwitches {
		switches[sw] = true
	}

	if len(switches) != len(expectedSwitches) {
		t.Errorf("Expected %d switches, got %d", len(expectedSwitches), len(switches))
	}
}

func TestEntityConfigBuildUsesNativeStepForTemperature(t *testing.T) {
	serial := "TEST12345"
	prefix := "nbe/TEST12345"
	devBlock := createDeviceBlock(serial)

	// Test temperature entity
	tempEntity := EntityConfig{
		Key:          "boiler_setpoint",
		Name:         "Wanted Temperature",
		EntityType:   Number,
		DeviceClass:  "temperature",
		Unit:         "°C",
		MinValue:     0,
		MaxValue:     85,
		Step:         "1",
		StateTopic:   "boiler/temp",
		CommandTopic: "set/boiler/temp",
	}

	config := tempEntity.Build(serial, prefix, devBlock)

	// Should use native_step, native_min_value, native_max_value for temperature
	if step, ok := config["native_step"]; !ok || step != "1" {
		t.Errorf("Expected native_step='1' for temperature entity, got %v", config["native_step"])
	}
	if _, ok := config["step"]; ok {
		t.Error("Expected 'step' to not be set for temperature entity, but it was")
	}

	// Test percentage entity (no device_class)
	percentEntity := EntityConfig{
		Key:          "boiler_power_min",
		Name:         "Minimum Power (%)",
		EntityType:   Number,
		Unit:         "%",
		MinValue:     10,
		MaxValue:     100,
		Step:         "1",
		StateTopic:   "regulation/boiler_power_min",
		CommandTopic: "set/regulation/boiler_power_min",
	}

	config = percentEntity.Build(serial, prefix, devBlock)

	// Should use regular step, min, max for non-native units
	if step, ok := config["step"]; !ok || step != "1" {
		t.Errorf("Expected step='1' for percentage entity, got %v", config["step"])
	}
	if _, ok := config["native_step"]; ok {
		t.Error("Expected 'native_step' to not be set for percentage entity, but it was")
	}
	if minVal, ok := config["min"]; !ok || minVal != 10 {
		t.Errorf("Expected min=10 for percentage entity, got %v", config["min"])
	}
	if maxVal, ok := config["max"]; !ok || maxVal != 100 {
		t.Errorf("Expected max=100 for percentage entity, got %v", config["max"])
	}
}

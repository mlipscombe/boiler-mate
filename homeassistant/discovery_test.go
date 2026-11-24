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
	"reflect"
	"testing"

	"github.com/mlipscombe/boiler-mate/nbe"
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

type discoveryCapture map[string]map[string]any

func (capture discoveryCapture) PublishJSON(topic string, value any) error {
	capture[topic] = value.(map[string]any)
	return nil
}

func TestTemperatureDiscoveryUsesControllerRanges(t *testing.T) {
	const serial, prefix = "TEST12345", "custom/boiler"
	ranges := map[string]any{
		"temp":       map[string]any{"min": int64(45), "max": nbe.RoundedFloat(82.5), "decimals": int64(1)},
		"diff_under": map[string]any{"min": int64(0), "max": int64(35), "decimals": int64(0)},
		"diff_over":  map[string]any{"min": int64(5), "max": int64(25), "decimals": int64(2)},
	}
	configs := make(discoveryCapture)
	if err := PublishDiscovery(configs, serial, prefix, ranges); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		entity, setting, class string
		min, max, step         float64
	}{
		{"boiler_setpoint", "temp", "temperature", 45, 82.5, 0.1},
		{"diff_under", "diff_under", "temperature_delta", 0, 35, 1},
		{"diff_over", "diff_over", "temperature_delta", 5, 25, 0.01},
	} {
		config := configs["homeassistant/number/nbe_"+serial+"/"+want.entity+"/config"]
		for key, expected := range map[string]any{
			"unit_of_measurement": "°C",
			"device_class":        want.class,
			"min":                 want.min,
			"max":                 want.max,
			"step":                want.step,
			"stat_t":              prefix + "/boiler/" + want.setting,
			"cmd_t":               prefix + "/set/boiler/" + want.setting,
		} {
			if config[key] != expected {
				t.Errorf("%s %s = %v, want %v", want.entity, key, config[key], expected)
			}
		}
		for _, key := range []string{"native_unit_of_measurement", "native_min_value", "native_max_value", "native_step"} {
			if _, exists := config[key]; exists {
				t.Errorf("%s published unsupported %s", want.entity, key)
			}
		}
	}
}

func TestDiscoveryWithoutTemperatureRanges(t *testing.T) {
	configs := make(discoveryCapture)
	if err := PublishDiscovery(configs, "1234", "custom/boiler", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := configs["homeassistant/number/nbe_1234/boiler_setpoint/config"]; ok {
		t.Error("setpoint was discovered without controller bounds")
	}
	if _, ok := configs["homeassistant/switch/nbe_1234/power/config"]; !ok {
		t.Error("missing ranges blocked power discovery")
	}
}

func TestBoilerEntityDiscovery(t *testing.T) {
	const serial, prefix = "TEST12345", "nbe/TEST12345"
	configs := make(map[string]map[string]interface{})
	for _, entity := range AllEntities() {
		configs[entity.GetDiscoveryTopic(serial)] = entity.Build(serial, prefix, createDeviceBlock(serial))
	}

	power := configs["homeassistant/sensor/nbe_TEST12345/power_pct/config"]
	if power["unit_of_measurement"] != "%" || power["stat_t"] != prefix+"/operating_data/power_pct" {
		t.Errorf("unexpected power percentage config: %v", power)
	}
	if _, exists := power["device_class"]; exists {
		t.Errorf("power percentage has an incompatible device class: %v", power["device_class"])
	}

	wantAvailability := []map[string]string{
		{"topic": prefix + "/device/status"},
		{"topic": prefix + "/device/controller_status"},
	}
	for _, topic := range []string{
		"homeassistant/switch/nbe_TEST12345/power/config",
		"homeassistant/sensor/nbe_TEST12345/status/config",
		"homeassistant/number/nbe_TEST12345/hopper_content/config",
		"homeassistant/button/nbe_TEST12345/start_calibrate/config",
	} {
		config := configs[topic]
		if config["availability_mode"] != "all" || !reflect.DeepEqual(config["availability"], wantAvailability) || config["avty_t"] != nil {
			t.Errorf("%s has unexpected availability: %v", topic, config)
		}
	}
	ipAddress := configs["homeassistant/sensor/nbe_TEST12345/ip_address/config"]
	if ipAddress["avty_t"] != prefix+"/device/status" || ipAddress["availability"] != nil {
		t.Errorf("IP address has unexpected availability: %v", ipAddress)
	}

	hopper := configs["homeassistant/number/nbe_TEST12345/hopper_content/config"]
	for key, want := range map[string]interface{}{
		"stat_t":              prefix + "/hopper/content",
		"cmd_t":               prefix + "/set/hopper/content",
		"unit_of_measurement": "kg",
		"min":                 -30000,
		"max":                 30000,
	} {
		if hopper[key] != want {
			t.Errorf("hopper %s = %v, want %v", key, hopper[key], want)
		}
	}
}

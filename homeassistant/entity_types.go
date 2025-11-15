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

import "fmt"

// EntityType represents the type of Home Assistant entity
type EntityType string

const (
	Sensor EntityType = "sensor"
	Number EntityType = "number"
	Button EntityType = "button"
	Switch EntityType = "switch"
)

// EntityConfig represents a Home Assistant entity configuration
type EntityConfig struct {
	Key            string
	Name           string
	EntityType     EntityType
	EntityCategory string
	DeviceClass    string
	Icon           string
	Unit           string
	StateTopic     string
	CommandTopic   string
	Precision      int
	MinValue       interface{}
	MaxValue       interface{}
	Step           string
	Mode           string
	PayloadPress   string
}

// Build creates the MQTT discovery message for this entity
func (e *EntityConfig) Build(serial, prefix string, devBlock map[string]interface{}) map[string]interface{} {
	config := map[string]interface{}{
		"name":    e.Name,
		"uniq_id": fmt.Sprintf("nbe_%s_%s", serial, e.Key),
		"avty_t":  fmt.Sprintf("%s/device/status", prefix),
		"dev":     devBlock,
	}

	// Add optional fields only if they're set
	if e.EntityCategory != "" {
		config["entity_category"] = e.EntityCategory
	}
	if e.DeviceClass != "" {
		config["device_class"] = e.DeviceClass
	}
	if e.Icon != "" {
		config["ic"] = e.Icon
	}
	if e.Unit != "" {
		if e.DeviceClass == "temperature" {
			config["native_unit_of_measurement"] = e.Unit
			config["suggested_unit_of_measurement"] = e.Unit
		} else {
			config["unit_of_measurement"] = e.Unit
		}
	}
	if e.Precision > 0 {
		config["suggested_display_precision"] = e.Precision
	}

	// State topic - use StateTopic if set, otherwise construct from prefix
	if e.StateTopic != "" {
		if e.StateTopic[0] == '/' {
			// Absolute path (starts with /)
			config["stat_t"] = e.StateTopic[1:]
		} else {
			// Relative path
			config["stat_t"] = fmt.Sprintf("%s/%s", prefix, e.StateTopic)
		}
	}

	// Command topic (for numbers, switches, buttons)
	if e.CommandTopic != "" {
		if e.CommandTopic[0] == '/' {
			config["cmd_t"] = e.CommandTopic[1:]
		} else {
			config["cmd_t"] = fmt.Sprintf("%s/%s", prefix, e.CommandTopic)
		}
	}

	// Number-specific fields
	if e.EntityType == Number {
		if e.Mode != "" {
			config["mode"] = e.Mode
		}
		if e.MinValue != nil {
			// Use native_min_value for temperature, otherwise min
			if e.DeviceClass == "temperature" {
				config["native_min_value"] = e.MinValue
			} else {
				config["min"] = e.MinValue
			}
		}
		if e.MaxValue != nil {
			if e.DeviceClass == "temperature" {
				config["native_max_value"] = e.MaxValue
			} else {
				config["max"] = e.MaxValue
			}
		}
		if e.Step != "" {
			if e.DeviceClass == "temperature" {
				config["native_step"] = e.Step
			} else {
				config["step"] = e.Step
			}
		}
	}

	// Button-specific fields
	if e.EntityType == Button && e.PayloadPress != "" {
		config["payload_press"] = e.PayloadPress
	}

	// Switch uses state_topic instead of stat_t
	if e.EntityType == Switch && e.StateTopic != "" {
		delete(config, "stat_t")
		config["state_topic"] = fmt.Sprintf("%s/%s", prefix, e.StateTopic)
	}

	return config
}

// GetDiscoveryTopic returns the MQTT discovery topic for this entity
func (e *EntityConfig) GetDiscoveryTopic(serial string) string {
	return fmt.Sprintf("homeassistant/%s/nbe_%s/%s/config", e.EntityType, serial, e.Key)
}

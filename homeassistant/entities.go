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

// AllEntities returns all entity configurations for NBE boiler
func AllEntities() []EntityConfig {
	return []EntityConfig{
		// Sensors
		{
			Key:            "ip_address",
			Name:           "IP Address",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			StateTopic:     "device/ip_address",
		},
		{
			Key:            "serial",
			Name:           "Serial",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			StateTopic:     "device/serial",
		},
		{
			Key:            "boiler_temp",
			Name:           "Boiler Temperature",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			DeviceClass:    "temperature",
			Unit:           "°C",
			Precision:      2,
			StateTopic:     "operating_data/boiler_temp",
		},
		{
			Key:            "oxygen",
			Name:           "Oxygen",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			Unit:           "%",
			Icon:           "mdi:air-filter",
			Precision:      2,
			StateTopic:     "operating_data/oxygen",
		},
		{
			Key:            "status",
			Name:           "Status",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			Icon:           "mdi:power",
			StateTopic:     "operating_data/state_text",
		},
		{
			Key:            "smoke_temp",
			Name:           "Smoke Temperature",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			DeviceClass:    "temperature",
			Unit:           "°C",
			Precision:      2,
			StateTopic:     "operating_data/smoke_temp",
		},
		{
			Key:            "photo_level",
			Name:           "Photo Level",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			Unit:           "%",
			Icon:           "mdi:lightbulb",
			Precision:      2,
			StateTopic:     "operating_data/photo_level",
		},
		{
			Key:            "power_kw",
			Name:           "Power (kW)",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			DeviceClass:    "power",
			Unit:           "kW",
			Precision:      2,
			StateTopic:     "operating_data/power_kw",
		},
		{
			Key:            "power_pct",
			Name:           "Power (%)",
			EntityType:     Sensor,
			EntityCategory: "diagnostic",
			DeviceClass:    "power",
			Unit:           "%",
			Precision:      2,
			StateTopic:     "operating_data/power_pct",
		},

		// Numbers (controls)
		{
			Key:            "boiler_setpoint",
			Name:           "Wanted Temperature",
			EntityType:     Number,
			EntityCategory: "config",
			DeviceClass:    "temperature",
			Unit:           "°C",
			Mode:           "box",
			MinValue:       0,
			MaxValue:       85,
			Precision:      1,
			Step:           "1",
			StateTopic:     "boiler/temp",
			CommandTopic:   "set/boiler/temp",
		},
		{
			Key:            "boiler_power_min",
			Name:           "Minimum Power (%)",
			EntityType:     Number,
			EntityCategory: "config",
			Unit:           "%",
			Mode:           "box",
			MinValue:       10,
			MaxValue:       100,
			Precision:      0,
			Step:           "1",
			StateTopic:     "regulation/boiler_power_min",
			CommandTopic:   "set/regulation/boiler_power_min",
		},
		{
			Key:            "boiler_power_max",
			Name:           "Maximum Power (%)",
			EntityType:     Number,
			EntityCategory: "config",
			Unit:           "%",
			Mode:           "box",
			MinValue:       10,
			MaxValue:       100,
			Precision:      0,
			Step:           "1",
			StateTopic:     "regulation/boiler_power_max",
			CommandTopic:   "set/regulation/boiler_power_max",
		},
		{
			Key:            "diff_under",
			Name:           "Difference Under",
			EntityType:     Number,
			EntityCategory: "config",
			DeviceClass:    "temperature",
			Unit:           "°C",
			Mode:           "box",
			Icon:           "mdi:arrow-collapse-down",
			MinValue:       0,
			MaxValue:       50,
			Precision:      1,
			Step:           "1",
			StateTopic:     "boiler/diff_under",
			CommandTopic:   "set/boiler/diff_under",
		},
		{
			Key:            "diff_over",
			Name:           "Difference Over",
			EntityType:     Number,
			EntityCategory: "config",
			DeviceClass:    "temperature",
			Unit:           "°C",
			Mode:           "box",
			Icon:           "mdi:arrow-collapse-up",
			MinValue:       10,
			MaxValue:       20,
			Precision:      1,
			Step:           "1",
			StateTopic:     "boiler/diff_over",
			CommandTopic:   "set/boiler/diff_over",
		},
		{
			Key:            "hopper_content",
			Name:           "Hopper",
			EntityType:     Number,
			EntityCategory: "config",
			DeviceClass:    "weight",
			Unit:           "kg",
			Mode:           "box",
			Icon:           "mdi:storage-tank",
			MinValue:       0,
			MaxValue:       999,
			Precision:      1,
			Step:           "1",
			StateTopic:     "hopper/content",
			CommandTopic:   "set/hopper/content",
		},

		// Buttons
		{
			Key:            "start_calibrate",
			Name:           "Start O2 Sensor Calibration",
			EntityType:     Button,
			EntityCategory: "config",
			Icon:           "mdi:air-filter",
			StateTopic:     "oxygen/start_calibrate",
			CommandTopic:   "set/oxygen/start_calibrate",
			PayloadPress:   "1",
		},

		// Switches
		{
			Key:            "power",
			Name:           "Power",
			EntityType:     Switch,
			EntityCategory: "config",
			Icon:           "mdi:power",
			StateTopic:     "operating_data/state_on",
			CommandTopic:   "set/device/power_switch",
		},
	}
}

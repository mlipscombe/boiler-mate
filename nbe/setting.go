/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021 Mark Lipscombe.
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

// SettingRange is the controller's metadata for a setup value.
type SettingRange struct {
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Default  float64 `json:"default"`
	Decimals int     `json:"decimals"`
}

// ReadSettingRange reads one range from a GetSetupRangeFunction response.
func ReadSettingRange(payload map[string]any, key string) *SettingRange {
	record, ok := payload[key].(map[string]any)
	if !ok {
		return nil
	}
	minimum, hasMin := settingNumber(record["min"])
	maximum, hasMax := settingNumber(record["max"])
	defaultValue, _ := settingNumber(record["default"])
	decimals, hasDecimals := record["decimals"].(int64)
	if !hasMin || !hasMax || minimum > maximum || !hasDecimals || decimals < 0 || decimals > 6 {
		return nil
	}
	return &SettingRange{Min: minimum, Max: maximum, Default: defaultValue, Decimals: int(decimals)}
}

func settingNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case int64:
		return float64(v), true
	case RoundedFloat:
		return float64(v), true
	default:
		return 0, false
	}
}

type SettingDefinition struct {
	Name     string       `json:"name"`
	Group    string       `json:"group"`
	Min      RoundedFloat `json:"min"`
	Max      RoundedFloat `json:"max"`
	Decimals int64        `json:"decimals"`
}

func (setting *SettingDefinition) Validate(value interface{}) error {
	return nil
}

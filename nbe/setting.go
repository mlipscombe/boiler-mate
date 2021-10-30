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

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

import (
	"strconv"
)

type RoundedFloat float64

func (r RoundedFloat) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatFloat(float64(r), 'f', 2, 32)), nil
}

func (r RoundedFloat) Equal(other RoundedFloat) bool {
	return strconv.FormatFloat(float64(r), 'f', 2, 32) == strconv.FormatFloat(float64(other), 'f', 2, 32)
}

type Function int16

const (
	DiscoveryFunction            Function = 0
	GetSetupFunction             Function = 1
	SetSetupFunction             Function = 2
	GetSetupRangeFunction        Function = 3
	GetOperatingDataFunction     Function = 4
	GetAdvancedDataFunction      Function = 5
	GetConsumptionDataFunction   Function = 6
	GetChartDataFunction         Function = 7
	GetEventLogFunction          Function = 8
	GetInfoFunction              Function = 9
	GetAvailableProgramsFunction Function = 10
	UnknownFunction              Function = -1
)

var Settings = []string{
	"boiler",
	"hot_water",
	"regulation",
	"weather",
	"weather2",
	"oxygen",
	"cleaning",
	"hopper",
	"fan",
	"auger",
	"ignition",
	"pump",
	"sun",
	"vacuum",
	"misc",
	"alarm",
	"manual",
}

package webui

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

type settingOption struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled,omitempty"`
}

type optionKind int

const (
	numericSetting optionKind = iota
	onOff
	noYes
	distanceSensor
	weatherValve
	lambdaSensor
	oxygenRegulation
	relayOutput
	baseRelayOutput
	temperatureInput
	weatherReferenceInput
)

// StokerCloud's English controller menus assign these names to occupied relays.
var outputRoles = map[string]string{
	"alarm.output":            "Alarm output",
	"cleaning.output_ash":     "Outlet ash clean",
	"cleaning.output_boiler1": "Outlet boiler valve 1",
	"cleaning.output_boiler2": "Outlet boiler valve 2",
	"cleaning.output_burner":  "Output burner valve",
	"fan.output_exhaust":      "Controller output for exhaust fan",
	"hot_water.output":        "Output DHW",
	"pump.output":             "Output pump",
	"sun.output_excess":       "Output sun excess heat",
	"sun.output_pump":         "Output sun pump",
	"weather.output_down":     "Regulating valve close",
	"weather.output_pump":     "Outlet house pump",
	"weather.output_up":       "Regulating valve open",
	"weather2.output_down":    "Zone 2 valve closing",
	"weather2.output_pump":    "Output house pump zone 2",
	"weather2.output_up":      "Zone 2 valve opening",
}

var relaySettings = slices.Sorted(maps.Keys(outputRoles))

func settingOptionKind(path string, bounds *settingRange) optionKind {
	switch path {
	case "misc.expansion_module":
		return noYes
	case "hopper.distance_sensor":
		return distanceSensor
	case "hot_water.dwh_weather", "hot_water.dwh_weather2":
		return weatherValve
	case "oxygen.lambda_type":
		return lambdaSensor
	case "oxygen.regulation":
		return oxygenRegulation
	case "sun.output_pump":
		return baseRelayOutput
	case "sun.input_collector", "sun.input_collector_2", "sun.input_dhw", "sun.input_excess",
		"weather.input_forward", "weather2.input_forward":
		return temperatureInput
	case "weather.input_reference", "weather2.input_reference":
		return weatherReferenceInput
	case "auger.auto_calculation", "boiler.ext_switch", "boiler.timer", "cleaning.pressure_t7",
		"fan.alarm_fan_current", "fan.alarm_fan_rpm", "fan.use_fan_rpm", "hot_water.timer",
		"oxygen.lambda_expansion_module", "sun.excess_from_top", "weather.active", "weather2.active":
		return onOff
	}
	if _, ok := outputRoles[path]; ok {
		return relayOutput
	}
	if bounds != nil && bounds.Min == 0 && bounds.Max == 1 && bounds.Decimals == 0 {
		return onOff
	}
	return numericSetting
}

func (kind optionKind) usesExpansion() bool {
	return kind == relayOutput || kind == temperatureInput || kind == weatherReferenceInput
}

func (kind optionKind) options(bounds *settingRange, expanded bool, roles map[int][]string) []settingOption {
	var options []settingOption
	switch kind {
	case onOff:
		options = numberedOptions("Off", "On")
	case noYes:
		options = []settingOption{{Value: "1", Label: "Yes"}, {Value: "0", Label: "No"}}
	case distanceSensor:
		options = numberedOptions("Off", "Ultra sound", "Infrared(20-150cm)")
	case weatherValve:
		options = numberedOptions("0%", "100%", "Regulation", "Standby")
	case lambdaSensor:
		options = numberedOptions("BOSCH", "NTK", "Denso")
	case oxygenRegulation:
		options = numberedOptions("OFF", "Show only", "ON")
	case relayOutput, baseRelayOutput:
		last := 13
		if expanded && kind == relayOutput {
			last = 20
		}
		options = portOptions(5, last, "- None -", "Output L", roles)
	case temperatureInput, weatherReferenceInput:
		last := 10
		if expanded {
			last = 20
		}
		unassigned := "Off"
		if kind == weatherReferenceInput {
			unassigned = "www"
		}
		options = portOptions(1, last, unassigned, "T", nil)
	}
	if bounds != nil {
		for index := range options {
			options[index].Disabled = !validSettingValue(options[index].Value, bounds, false)
		}
	}
	return options
}

func numberedOptions(labels ...string) []settingOption {
	options := make([]settingOption, len(labels))
	for value, label := range labels {
		options[value] = settingOption{Value: strconv.Itoa(value), Label: label}
	}
	return options
}

func portOptions(first, last int, unassigned, prefix string, roles map[int][]string) []settingOption {
	options := []settingOption{{Value: "0", Label: unassigned}}
	for port := first; port <= last; port++ {
		value := strconv.Itoa(port)
		label := prefix + value
		if assigned := roles[port]; len(assigned) > 0 {
			label += " (" + strings.Join(assigned, ", ") + ")"
		}
		options = append(options, settingOption{Value: value, Label: label})
	}
	return options
}

func (ui *UI) settingHardware() (bool, map[int][]string) {
	ui.mu.RLock()
	defer ui.mu.RUnlock()
	expanded := sameSettingValue("1", ui.snapshots["misc"].Values["expansion_module"])
	roles := make(map[int][]string)
	for _, path := range relaySettings {
		group, key, _ := strings.Cut(path, ".")
		port, _ := number(ui.snapshots[group].Values[key])
		if port >= 5 && port <= 20 && port == float64(int(port)) {
			roles[int(port)] = append(roles[int(port)], outputRoles[path])
		}
	}
	return expanded, roles
}

package webui

import (
	"strconv"
	"strings"
)

type description struct {
	Label string
	Help  string
}

var descriptions = map[string]description{
	"boiler.temp":                      {"Boiler target temperature", "The controller modulates burner output to reach this water temperature."},
	"boiler.diff_under":                {"Restart differential", "How far boiler temperature can fall below target before the controller calls for heat."},
	"boiler.diff_over":                 {"Stop differential", "How far boiler temperature can rise above target before the controller suspends firing."},
	"boiler.min_return":                {"Minimum return temperature", "Minimum temperature requested for water returning to the boiler."},
	"boiler.reduction":                 {"Night reduction", "Amount by which the boiler target is lowered during a reduced schedule period."},
	"boiler.ext_switch":                {"External contact", "Selects whether the external thermostat or enable input affects boiler demand."},
	"boiler.ext_on_delay":              {"External contact on delay", "Time the external input must request heat before the controller reacts."},
	"boiler.ext_off_delay":             {"External contact off delay", "Time the external input must stop requesting heat before the controller reacts."},
	"boiler.ext_stop_temp":             {"Outdoor stop temperature", "Outdoor temperature associated with automatic summer stopping."},
	"boiler.ext_stop_diff":             {"Outdoor stop differential", "Temperature gap between outdoor stopping and restarting thresholds."},
	"boiler.timer":                     {"Boiler schedule enabled", "Enable the heating schedule stored in the controller. The weekly editor changes its two-hour periods."},
	"hot_water.temp":                   {"DHW target temperature", "Target for the domestic hot-water cylinder when its configured sensor and output are active."},
	"hot_water.diff_under":             {"DHW difference (°C)", "How far cylinder temperature falls below target before requesting another heating cycle."},
	"hot_water.output":                 {"DHW output", "Controller output assigned to the domestic hot-water pump or valve. Match the installed wiring."},
	"hot_water.timer":                  {"DHW schedule enabled", "Enable the domestic hot-water schedule stored in the controller."},
	"hot_water.dhw_remain":             {"DHW after-run", "Controller setting for keeping hot-water circulation active after a heat demand ends."},
	"hot_water.dwh_weather":            {"Zone 1 weather valve during DHW", ""},
	"hot_water.dwh_weather2":           {"Zone 2 weather valve during DHW", ""},
	"regulation.boiler_power_min":      {"Minimum burner output", "Lower output limit the controller uses while regulating boiler temperature."},
	"regulation.boiler_power_max":      {"Maximum burner output", "Upper output limit the controller uses while regulating boiler temperature."},
	"regulation.dhw_power_min":         {"Minimum DHW power", ""},
	"regulation.dhw_power_max":         {"Maximum DHW power", "Output limit applied while producing domestic hot water."},
	"regulation.dhw_setpoint_addition": {"DHW boiler setpoint increase", "Extra boiler temperature requested during domestic hot-water recovery."},
	"auger.auger_capacity":             {"Auger capacity", "Pellet weight measured during the manufacturer's auger calibration procedure."},
	"auger.forced_run":                 {"Forced feed duration", "Runs the external auger for the controller-defined time. Use the manufacturer's service procedure."},
	"auger.kw_min":                     {"Power 10% (kW)", ""},
	"auger.kw_max":                     {"Power 100% (kW)", ""},
	"hopper.content":                   {"Pellets in hopper", "Estimated fuel remaining, in kilograms. Enter the measured stock after adding pellets."},
	"hopper.distance_sensor":           {"Hopper level sensor", "Enable the optional distance sensor used to estimate pellet fill level."},
	"hopper.trip1":                     {"Trip 1 (kg)", ""},
	"hopper.trip2":                     {"Trip 2 (kg)", "Second pellet usage trip counter."},
	"ignition.pellets":                 {"Ignition pellet charge", "Fuel delivered at the start of the ignition sequence."},
	"ignition.max_time":                {"Ignition timeout", "Maximum time allowed for the controller to establish a flame."},
	"ignition.ignition_number":         {"Ignition count", "Number of ignition cycles recorded by the controller."},
	"pump.start_temp_run":              {"Pump start temperature during firing", "Boiler water temperature above which the circulation pump may start while the burner is running."},
	"pump.start_temp_idle":             {"Pump start temperature while idle", "Boiler water temperature used for circulation while the burner is idle."},
	"pump.output":                      {"Circulation pump output", "Controller output assigned to the heating circulation pump."},
	"sun.collector_temp":               {"Solar collector temperature limit", "Controller temperature limit associated with the solar collector."},
	"sun.pump_start_diff":              {"Solar pump start differential", "Required temperature difference between collector and cylinder before starting circulation."},
	"sun.pump_stop_diff":               {"Solar pump stop differential", "Temperature difference at which useful solar circulation stops."},
	"sun.dhw_max_temp":                 {"Solar DHW maximum temperature", "Maximum cylinder temperature the solar circuit is configured to heat."},
	"alarm.max_shaft_temp":             {"Maximum drop-shaft temperature", "Controller alarm threshold for the pellet drop shaft. Follow the manufacturer's installation specifications."},
	"alarm.min_boiler_temp":            {"Minimum boiler alarm temperature", "Boiler water temperature below which the configured temperature alarm can activate."},
	"alarm.output":                     {"Alarm output", "Controller output assigned to an external fault indicator."},
	"misc.fahrenheit":                  {"Controller display units", "Selects the temperature unit on compatible controller displays."},
	"misc.expansion_module":            {"Expansion module", "Controller setting for the optional expansion module. Match the installed equipment."},
}

var settingCapitalization = map[string]string{
	"dhw": "DHW", "o2": "O2", "kw": "kW", "i": "I", "p": "P", "rpm": "RPM", "id": "ID", "dl": "DL",
}

var weatherCurveLabels = map[string]string{
	"ref": "Measured temperature", "temp": "Flow temperature", "pow": "Max. power",
}

func settingDescription(group, key string) (string, string) {
	if d, ok := descriptions[group+"."+key]; ok {
		return d.Label, d.Help
	}
	if group == "weather" || group == "weather2" {
		column, point, _ := strings.Cut(key, "_")
		if label, ok := weatherCurveLabels[column]; ok {
			if _, err := strconv.Atoi(point); err == nil {
				return label + " " + point, ""
			}
		}
	}
	words := strings.Split(key, "_")
	for index, word := range words {
		if normalized, ok := settingCapitalization[word]; ok {
			words[index] = normalized
		} else if index == 0 && word != "" {
			words[index] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	title := strings.Join(words, " ")
	if isTimer(group, key) {
		return title, "Controller schedule for this day, divided into 12 two-hour periods."
	}
	return title, ""
}

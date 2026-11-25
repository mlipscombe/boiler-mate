package webui

import (
	"strconv"
	"strings"
)

const infoCodePrefix = "Controller information code "

// Information IDs follow the lng_info_* entries in the StokerCloud translations.
// https://stokercloud.dk/v3/assets/json/translation/uk.json
var infoMessages = map[int]string{
	1:  "The boiler temperature is low enough to risk freezing.",
	2:  "Oxygen regulation at low fire is outside its range. Check the fuel adjustment.",
	3:  "Pellet level in the hopper is low.",
	4:  "The photo sensor detected no light for more than two minutes.",
	5:  "The boiler ignited more than ten times in 24 hours.",
	6:  "The boiler temperature is too high during domestic hot water priority.",
	7:  "The controller has lost its internet connection.",
	8:  "Fuel correction for oxygen regulation at high fire is outside its range. Check burner adjustment.",
	9:  "There is a new StokerCloud message.",
	10: "Weather compensation is limiting boiler power.",
	11: "The flame went out. Check the low-fire settings.",
	12: "The controller ignored the light reading during compressor cleaning.",
	13: "Hopper content is below the controller minimum.",
	14: "The controller cannot reach the expansion board.",
	15: "Unexpected current draw on an output.",
	16: "Startup offset applied to the oxygen target.",
	17: "The controller disabled oxygen regulation because of an oxygen probe fault.",
	18: "The compressor is failing to build pressure.",
	19: "Pressure did not drop during compressor cleaning.",
	20: "The boiler has no vacuum.",
	21: "Solar heating is running in safety mode.",
	22: "The solar heating system has excess heat.",
	23: "The distance sensor reports a low pellet level.",
	24: "Clean out the ash drawer.",
	25: "High air temperature is limiting boiler power.",
	26: "Air heater room temperature sensor fault.",
	27: "Air heater exhaust temperature sensor T2 fault.",
	28: "Air heater outlet temperature sensor T3 fault.",
	29: "Weather compensation circuit 1 flow temperature sensor fault.",
	30: "Weather compensation circuit 1 reference sensor fault.",
	31: "Weather compensation circuit 2 flow temperature sensor fault.",
	32: "Weather compensation circuit 2 reference sensor fault.",
	33: "Domestic hot water temperature sensor T4 fault.",
	34: "Boiler return temperature sensor T3 fault.",
	35: "Solar collector 1 temperature sensor fault.",
	36: "Solar collector 2 temperature sensor fault.",
	37: "Solar hot water temperature sensor fault.",
	38: "Solar surplus temperature sensor fault.",
	39: "The timer is blocking boiler cleaning.",
	40: "The pellet distance sensor stayed out of range beyond the allowed time.",
	41: "The external pellet supply ran out.",
	42: "An operator cleared a vacuum transport fault.",
	43: "The ash tray needs emptying.",
	44: "The ash box contact opened.",
	45: "High backpressure reduced pellet feed. The flue fan, if fitted, is running at full speed.",
	46: "Backpressure has returned to normal.",
	47: "The controller disabled airflow regulation because airflow measurement failed.",
	48: "Vacuum transport has started.",
	49: "Vacuum transport has stopped.",
	50: "The cooling device stopped or reported a fault.",
	51: "Night setback is active.",
	52: "Reduced airflow is limiting boiler power.",
	53: "Insufficient boiler draft is limiting power.",
	57: "A Wi-Fi sensor battery is low.",
	59: "The controller is testing an output.",
	60: "The controller is exercising the boiler pump.",
	61: "The controller is exercising the weather compensation circuit 1 pump.",
	62: "The controller is exercising the weather compensation circuit 2 pump.",
	63: "The controller is exercising the solar pump.",
	64: "Compressor cleaning failed to complete once out of a maximum of five attempts.",
	65: "Compressor cleaning failed to complete twice out of a maximum of five attempts.",
	66: "Compressor cleaning failed to complete three times out of a maximum of five attempts.",
	67: "Compressor cleaning failed to complete four times out of a maximum of five attempts.",
	68: "Compressor cleaning failed to complete five times.",
	69: "The igniter switched on.",
	70: "The igniter switched off.",
	71: "The airflow sensor reported a fault.",
	72: "The airflow sensor is working again.",
	73: "Airflow is no longer limiting power.",
	74: "Return temperature is limiting weather compensation.",
	75: "Return temperature is no longer limiting weather compensation.",
	76: "Boiler draft is sufficient and no longer limiting power.",
	77: "Low-fire auger capacity reverted to its value from three hours earlier.",
	78: "Mid-fire auger capacity reverted to its value from three hours earlier.",
	79: "High-fire auger capacity reverted to its value from three hours earlier.",
	80: "The condensate tank is full. Check the drain.",
	82: "The end stop was not reached. Check the motor and limit switch.",
	84: "The latest heat exchanger cleaning cycle lost its water supply. Check the water shutoff.",
	85: "The wash cycle has started.",
	86: "The wash cycle has stopped.",
}

func infoMessage(raw string) string {
	// Older releases saved individual unknown codes with a prefix and saved
	// comma-separated codes as raw text.
	codes := strings.Split(strings.TrimPrefix(raw, infoCodePrefix), ",")
	messages := make([]string, 0, len(codes))
	for _, code := range codes {
		id, err := strconv.Atoi(strings.TrimSpace(code))
		if err != nil {
			return raw
		}
		message, ok := infoMessages[id]
		if !ok {
			message = infoCodePrefix + strconv.Itoa(id)
		}
		messages = append(messages, message)
	}
	return strings.Join(messages, " · ")
}

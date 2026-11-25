package heating

import (
	"encoding/json"
	"slices"
	"time"
)

type input struct {
	spec         inputSpec
	availability []string
	latest       string
	at           time.Time
	action       string
	celsius      float64
}

func newInput(spec inputSpec) *input {
	return &input{spec: spec, availability: make([]string, len(spec.Availability))}
}

func (in *input) availabilityState() string {
	if in == nil || in.spec.Problem != "" {
		return ""
	}
	if len(in.availability) == 0 {
		return "online"
	}
	switch in.spec.Mode {
	case "all":
		if slices.Contains(in.availability, "offline") {
			return "offline"
		}
		if !slices.Contains(in.availability, "") {
			return "online"
		}
	case "any":
		if slices.Contains(in.availability, "online") {
			return "online"
		}
		if !slices.Contains(in.availability, "") {
			return "offline"
		}
	default:
		return in.latest
	}
	return ""
}

func (in *input) receive(topic string, payload []byte, retained bool, now time.Time) {
	previous := in.availabilityState()
	for i, source := range in.spec.Availability {
		if source.Topic != topic {
			continue
		}
		value, ok := source.value(payload)
		if !ok {
			continue
		}
		switch value {
		case source.Online:
			in.availability[i], in.latest = "online", "online"
		case source.Offline:
			in.availability[i], in.latest = "offline", "offline"
		}
	}
	current := in.availabilityState()
	if previous != "offline" && current == "offline" {
		in.at, in.action = time.Time{}, ""
	}
	if retained && current == "offline" {
		return
	}
	source := in.spec.Value
	if in.spec.Kind == thermostat {
		if source.Topic == topic {
			if value, ok := source.value(payload); ok {
				state, valid := actionValue(source, value)
				if in.spec.ZWave {
					state, valid = zwaveAction(value)
				}
				if valid {
					in.action = state
				}
			}
		}
		source = in.spec.RoomTemperature
	}
	if source.Topic != topic {
		return
	}
	value, ok := source.value(payload)
	if !ok {
		return
	}
	unit := in.spec.Unit
	if in.spec.ZWave {
		var message struct {
			Unit     string `json:"unit"`
			Metadata struct {
				Unit string `json:"unit"`
			} `json:"metadata"`
		}
		if json.Unmarshal(payload, &message) == nil {
			for _, reported := range []string{message.Unit, message.Metadata.Unit} {
				if reported == "°C" || reported == "°F" {
					unit = reported
					break
				}
			}
		}
	}
	if celsius, ok := temperatureValue(unit, value); ok {
		in.celsius, in.at = celsius, now
	} else if value == "None" || value == "null" || value == "unavailable" || value == "unknown" || value == "" {
		in.at = time.Time{}
	}
}

func (in *input) currentAction() string {
	if in.availabilityState() != "online" {
		return ""
	}
	return in.action
}

func (in *input) currentTemperature(now time.Time, timeout time.Duration) (float64, bool) {
	if in.availabilityState() != "online" || in.at.IsZero() {
		return 0, false
	}
	if in.spec.ExpireAfter > 0 && (timeout == 0 || in.spec.ExpireAfter < timeout) {
		timeout = in.spec.ExpireAfter
	}
	return in.celsius, timeout == 0 || now.Sub(in.at) < timeout
}

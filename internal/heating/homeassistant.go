package heating

import (
	"strings"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/hass"
)

const homeAssistantPrefix = "ha:"

func (c *Control) ObserveHomeAssistant(readings []hass.Reading) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make(map[string]*input)
	for _, reading := range readings {
		kind := temperature
		id := homeAssistantPrefix + reading.ID
		previous := c.homeAssistant[id]
		unit, name := reading.Unit, reading.Name
		if previous != nil {
			if unit == "" {
				unit = previous.spec.Unit
			}
			if name == "" {
				name = previous.spec.Name
			}
		}
		if strings.HasPrefix(reading.ID, "climate.") {
			kind = thermostat
		} else if unit != "°C" {
			continue
		}
		if name == "" {
			name = reading.ID
		}
		in := &input{spec: inputSpec{ID: id, Kind: kind, Name: name, Unit: unit}, polledStatus: "offline"}
		if reading.Available {
			in.polledStatus = "online"
			in.action = reading.Action
			if reading.Action != "" {
				in.actionAt, in.actionLive = reading.At, true
			}
			if reading.HasValue && reading.Value >= -100 && reading.Value <= 100 {
				in.celsius, in.at = reading.Value, reading.At
			}
		}
		next[id] = in
	}
	for id := range c.homeAssistant {
		delete(c.active, id)
	}
	for id, reading := range next {
		c.active[id] = reading
	}
	c.homeAssistant = next
	c.updateDemand(time.Now())
	c.notify()
}

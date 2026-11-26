package heating

import "time"

type DashboardThermostat struct {
	Name         string   `json:"name"`
	Action       string   `json:"action,omitempty"`
	Availability string   `json:"availability,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
}

type DashboardInputs struct {
	Unit                  string                `json:"unit"`
	Thermostats           []DashboardThermostat `json:"thermostats"`
	HasLocalOutdoorSensor bool                  `json:"hasLocalOutdoorSensor"`
	OutdoorTemperature    *float64              `json:"outdoorTemperature,omitempty"`
	OutdoorSource         string                `json:"outdoorSource,omitempty"`
}

// DashboardInputs reads only the configured zones and local outdoor sensor.
func (c *Control) DashboardInputs(now time.Time) *DashboardInputs {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.Settings.Thermostats) == 0 && c.Settings.OutdoorSensor == "" {
		return nil
	}
	status := &DashboardInputs{
		Unit: c.Settings.Unit, HasLocalOutdoorSensor: c.Settings.OutdoorSensor != "",
		Thermostats: make([]DashboardThermostat, 0, len(c.Settings.Thermostats)),
	}
	for _, id := range c.Settings.Thermostats {
		in := c.active[id]
		name := id
		if in != nil {
			name = in.spec.Name
		} else if node, endpoint, ok := zwaveNode(id); ok {
			name = zwaveThermostat(id, node, endpoint).Name
		}
		zone := DashboardThermostat{Name: name, Action: in.currentAction(), Availability: in.availabilityState()}
		if temp, ok := in.currentTemperature(now, 0); ok {
			value := c.Settings.displayTemperature(temp)
			zone.Temperature = &value
		}
		status.Thermostats = append(status.Thermostats, zone)
	}
	if temp, ok := c.outdoorReading(now); ok {
		value := c.Settings.displayTemperature(temp)
		status.OutdoorTemperature, status.OutdoorSource = &value, "local"
	}
	return status
}

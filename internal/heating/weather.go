package heating

import "time"

// OutdoorReading returns the selected sensor's current reading in Celsius and its receipt time.
func (c *Control) OutdoorReading(now time.Time) (sensor string, celsius float64, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sensor = c.Settings.OutdoorSensor
	if value, ok := c.outdoorReading(now); ok {
		return sensor, value, c.active[sensor].at
	}
	return sensor, 0, time.Time{}
}

func (c *Control) outdoorReading(now time.Time) (float64, bool) {
	age := time.Duration(c.Settings.SensorTimeout) * time.Second
	return c.active[c.Settings.OutdoorSensor].currentTemperature(now, age)
}

package heating

import "time"

// OutdoorReading returns the selected sensor's current reading in Celsius.
func (c *Control) OutdoorReading(now time.Time) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outdoorReading(now)
}

func (c *Control) outdoorReading(now time.Time) (float64, bool) {
	age := time.Duration(c.Settings.SensorTimeout) * time.Second
	return c.active[c.Settings.OutdoorSensor].currentTemperature(now, age)
}

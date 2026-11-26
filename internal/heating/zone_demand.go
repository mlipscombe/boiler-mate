package heating

import (
	"maps"
	"slices"
	"time"
)

// WatchZoneDemand reports changes in confirmed thermostat demand. Missing zones
// have unknown demand. The observer runs under the control lock and must not block.
func (c *Control) WatchZoneDemand(zones []string, observe func(time.Time, map[string]bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.demandZones = slices.Clone(zones)
	c.demandObserver = observe
	if observe == nil {
		c.demandStates = nil
	}
	c.reportZoneDemand(time.Now())
}

func (c *Control) reportZoneDemand(now time.Time) {
	if c.demandObserver == nil {
		return
	}
	states := make(map[string]bool, len(c.demandZones))
	for _, zone := range c.demandZones {
		if action := c.active[zone].currentAction(); action != "" {
			states[zone] = action == "heating"
		}
	}
	if !maps.Equal(c.demandStates, states) {
		c.demandStates = states
		c.demandObserver(now, states)
	}
}

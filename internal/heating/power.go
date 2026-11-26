package heating

import "time"

// Allow a controller to finish its shutdown cycle before it reports Off.
// An expired command must never mask a subsequent stop at the controller.
const automaticStopWindow = 15 * time.Minute

// HoldManualStop records the operator's intent before a stop request reaches the
// controller. Even an uncertain reply must not leave automatic starts armed.
func (c *Control) HoldManualStop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setManualStop() {
		c.savePowerState()
		c.notify()
	}
}

func (c *Control) setManualStop() bool {
	changed := !c.ManualOff || c.OutdoorResume || !c.AutoStopAt.IsZero()
	c.ManualOff = true
	c.OutdoorResume = false
	c.AutoStopAt = time.Time{}
	return changed
}

// ManualStartAccepted releases a manual stop after the controller accepts Start.
func (c *Control) ManualStartAccepted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordManualStart(time.Now())
	c.savePowerState()
	c.notify()
}

func (c *Control) recordManualStart(now time.Time) {
	c.ManualOff = false
	c.OutdoorResume = false
	c.AutoStopAt = time.Time{}
	c.LastPowerAt, c.LastPowerKey = now, Start
	c.powerConfirmed = c.boiler == "ON"
}

func (c *Control) Resume() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.write == nil {
		return ErrReadOnly
	}
	if !c.Settings.enabled() {
		return ErrDisabled
	}
	if !c.ManualOff {
		return nil
	}
	next := c.savedState
	next.ManualOff = false
	if err := save(c.path, next); err != nil {
		c.logger.Error("Could not save automatic heating resume", "error", err)
		return ErrSave
	}
	c.savedState = next
	c.notify()
	return nil
}

func (c *Control) observePower() {
	power := c.boiler
	if (power != "ON" && power != "OFF") || power == c.ObservedPower {
		return
	}
	previous := c.ObservedPower
	c.ObservedPower = power
	switch power {
	case "OFF":
		autoStop := !c.AutoStopAt.IsZero() && c.boilerAt.Before(c.AutoStopAt.Add(automaticStopWindow))
		// Older settings files recorded automatic commands but not the last
		// observation. An acknowledged stop still accounts for an initial Off.
		unaccounted := previous == "ON" || (previous == "" && c.Settings.enabled() && c.LastPowerKey != Stop)
		if !autoStop && unaccounted {
			c.setManualStop()
		}
		if autoStop {
			c.LastPowerAt, c.LastPowerKey = c.AutoStopAt, Stop
			c.powerConfirmed = true
		}
		c.AutoStopAt = time.Time{}
	case "ON":
		if previous == "OFF" && c.ManualOff {
			c.recordManualStart(c.boilerAt)
		}
	}
	c.savePowerState()
}

func (c *Control) savePowerState() {
	if err := save(c.path, c.savedState); err != nil {
		c.logger.Warn("Could not save external heating power state", "error", err)
		c.lastError = "Could not save external heating power state for the next restart."
	}
}

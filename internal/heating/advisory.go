package heating

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

type AdvisoryDemand struct {
	Key, Zones string
	Epoch      uint64
}

// AdvisoryDemand describes a completely observed set of selected thermostats
// while at least one is calling. A blank key marks an unsuitable observation.
func (c *Control) AdvisoryDemand(now time.Time) AdvisoryDemand {
	c.mu.Lock()
	defer c.mu.Unlock()
	demand := AdvisoryDemand{Epoch: c.mqttEpoch}
	if c.ManualOff || (c.Settings.Enabled && c.Settings.StopAt != nil && c.cutoff) {
		return demand
	}
	ids := slices.Clone(c.Settings.Thermostats)
	slices.Sort(ids)
	var active, parts []string
	for _, id := range ids {
		in := c.active[id]
		action := in.currentAction()
		if action == "" || !in.advisoryActionKnown(now, action) {
			return demand
		}
		parts = append(parts, id)
		if action == "heating" {
			active = append(active, in.spec.Name)
			parts = append(parts, "heating")
		} else {
			parts = append(parts, "idle")
		}
	}
	if len(active) == 0 {
		return demand
	}
	group := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	demand.Key, demand.Zones = hex.EncodeToString(group[:]), strings.Join(active, " + ")
	return demand
}

func (in *input) advisoryActionKnown(now time.Time, action string) bool {
	receiptAge := now.Sub(in.actionAt)
	if in.spec.ExpireAfter > 0 && receiptAge >= in.spec.ExpireAfter {
		return false
	}
	if !in.actionSourceAt.IsZero() {
		// Z-Wave JS UI reports on change. Gateway, driver, and node availability
		// establish continuity, while the payload time bounds retained data.
		age := now.Sub(in.actionSourceAt)
		return age >= -2*time.Minute && age <= 24*time.Hour
	}
	if in.actionLive && receiptAge >= -time.Second && receiptAge <= 3*time.Hour {
		return true
	}
	// An explicitly available on-change publisher may retain its idle state.
	// Require a live or timestamped report for active heat demand.
	return action != "heating" && len(in.spec.Availability) > 0
}

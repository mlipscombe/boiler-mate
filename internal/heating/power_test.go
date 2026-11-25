package heating

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestExternalOffHoldsAcrossRestart(t *testing.T) {
	s := defaultSettings()
	s.Enabled, s.Thermostats = true, []string{north}
	c := newTestControl(t, s)
	c.Observe("operating_data", map[string]any{"state_on": "ON"})
	c.Observe("operating_data", map[string]any{"state_on": "None"})
	if c.Snapshot().ManualOff {
		t.Fatal("an unknown transition is not an actual Off")
	}

	restarted := New(filepath.Dir(c.path), "test", c.logger, Options{Write: c.write})
	restarted.Observe("operating_data", map[string]any{"state_on": "OFF"})
	discoverHeatingThermostat(restarted)
	if !restarted.Snapshot().ManualOff {
		t.Fatal("unexplained Off during restart did not hold automatic control")
	}
	if got := nextAt(restarted, time.Now().Add(time.Hour)); got.key != "" {
		t.Fatalf("automatic command %+v while the boiler was manually off", got)
	}
	held := New(filepath.Dir(c.path), "test", c.logger, Options{Write: c.write})
	if !held.Snapshot().ManualOff {
		t.Fatal("manual stop was lost on restart")
	}
	held.Observe("operating_data", map[string]any{"state_on": "ON"})
	if held.Snapshot().ManualOff {
		t.Fatal("local manual Start did not release a confirmed Off")
	}
}

func TestAutomaticOffKeepsDemandArmed(t *testing.T) {
	s := defaultSettings()
	s.Enabled, s.Thermostats = true, []string{north}
	c := newTestControl(t, s)
	discoverHeatingThermostat(c)
	c.Receive("north/action", []byte("idle"), false)
	c.idleSince = time.Now().Add(-11 * time.Second)
	c.Observe("operating_data", map[string]any{"state_on": "ON"})
	c.write = func(_ context.Context, key string, _ int, current func() bool) error {
		if key != Stop || !current() {
			t.Fatalf("unexpected command %q", key)
		}
		// The boiler can report Off before its acknowledgement arrives.
		c.Observe("operating_data", map[string]any{"state_on": "OFF"})
		return errors.New("lost command reply")
	}
	c.apply(t.Context())
	if c.Snapshot().ManualOff {
		t.Fatal("automatically stopping set a manual hold")
	}
	c.Receive("north/action", []byte("heating"), false)
	if got := nextAt(c, time.Now()); got.key != "" {
		t.Fatalf("automatic restart bypassed the cycle interval: %+v", got)
	}
	if got := nextAt(c, time.Now().Add(3*time.Minute)); got.key != Start {
		t.Fatalf("new heat demand after an automatic stop = %+v, want start", got)
	}
}

func TestManualStopSupersedesAutomaticStartAndOutdoorResume(t *testing.T) {
	s := defaultSettings()
	s.Enabled, s.Thermostats = true, []string{north}
	c := newTestControl(t, s)
	discoverHeatingThermostat(c)
	c.boiler = "OFF"
	c.boilerAt = time.Now()
	c.OutdoorResume = true
	c.write = func(_ context.Context, key string, _ int, current func() bool) error {
		if key != Start {
			t.Fatalf("unexpected command %q", key)
		}
		c.HoldManualStop()
		if current() {
			t.Fatal("queued Start was still current after the manual stop")
		}
		return ErrSuperseded
	}
	c.apply(t.Context())
	if c.OutdoorResume || !c.Snapshot().ManualOff {
		t.Fatal("manual stop did not cancel outdoor resume and hold automatic control")
	}
	if err := c.Resume(); err != nil {
		t.Fatal(err)
	}
	if got := nextAt(c, time.Now()); got.key != Start {
		t.Fatalf("explicit resume with heat demand = %+v, want start", got)
	}
	c.HoldManualStop()
	c.ManualStartAccepted()
	if c.Snapshot().ManualOff || nextAt(c, time.Now()).key != "" {
		t.Fatal("manual Start must release the hold and avoid a duplicate Start")
	}
}

func discoverHeatingThermostat(c *Control) {
	c.Receive(north, []byte(`{"action_topic":"north/action"}`), true)
	c.Receive("north/action", []byte("heating"), true)
}

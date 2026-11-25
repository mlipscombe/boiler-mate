package heating

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

const (
	north   = "homeassistant/climate/north/config"
	south   = "homeassistant/climate/south/config"
	outdoor = "homeassistant/device/weather/config"
)

func newTestControl(t *testing.T, settings Settings) *Control {
	t.Helper()
	control := New(t.TempDir(), "test", slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context, string, int, func() bool) error { return nil })
	if err := control.Set(settings); err != nil {
		t.Fatal(err)
	}
	return control
}

func TestThermostatDemandAndDropout(t *testing.T) {
	s := defaultSettings()
	s.Enabled = true
	s.Thermostats = []string{north, south}
	c := newTestControl(t, s)
	c.Receive(north, []byte(`{"act_t":"north/action","avty_t":"north/status"}`), true)
	c.Receive(south, []byte(`{"action_topic":"south/action"}`), true)
	c.Receive("north/status", []byte("online"), true)
	c.Receive("north/action", []byte("heating"), true)
	c.Receive("south/action", []byte("idle"), true)
	c.boiler = "OFF"
	if got := nextAt(c, time.Now()); got.key != Start {
		t.Fatalf("one thermostat heating = %+v, want start", got)
	}
	c.boiler = "ON"
	if got := nextAt(c, time.Now()); got.key != "" {
		t.Fatalf("already enabled = %+v", got)
	}

	c.Receive("north/status", []byte("offline"), false)
	c.Receive("north/action", []byte("heating"), true)
	c.Receive("north/status", []byte("online"), false)
	now := time.Now()
	if action := c.active[north].currentAction(); action != "" {
		t.Fatalf("recovered thermostat reused stale action %q", action)
	}
	if got := nextAt(c, now.Add(time.Minute)); got.key != "" {
		t.Fatalf("issued %+v during dropout grace", got)
	}
	c.Receive("north/action", []byte("heating"), false)
	if got := nextAt(c, now.Add(3*time.Minute)); got.key != "" {
		t.Fatalf("issued %+v after thermostat recovered and still wants heat", got)
	}

	c.Receive("north/status", []byte("offline"), false)
	c.Receive("south/action", []byte("heating"), false)
	if got := nextAt(c, now.Add(3*time.Minute)); got.key != "" {
		t.Fatalf("issued %+v while another thermostat demands heat", got)
	}
	c.Receive("south/action", []byte("idle"), false)
	if got := nextAt(c, now.Add(3*time.Minute)); got.key != Stop {
		t.Fatalf("after dropout and no demand = %+v, want stop", got)
	}
	c.LastPowerAt, c.LastPowerKey = now.Add(3*time.Minute), Stop
	c.boiler = "OFF"
	c.Receive("south/action", []byte("heating"), false)
	if got := nextAt(c, now.Add(4*time.Minute)); got.key != "" {
		t.Fatalf("issued %+v during minimum cycle interval", got)
	}
	if got := nextAt(c, now.Add(6*time.Minute)); got.key != Start {
		t.Fatalf("after minimum cycle interval = %+v, want start", got)
	}
}

func TestDiscoveryRemovalAndBrokerLoss(t *testing.T) {
	s := defaultSettings()
	s.Enabled = true
	s.Thermostats = []string{north}
	c := newTestControl(t, s)
	c.boiler = "ON"
	if got := nextAt(c, time.Now().Add(time.Hour)); got.key != "" {
		t.Fatalf("issued %+v before any usable thermostat state", got)
	}
	c.Receive(north, []byte(`{"action_topic":"north/action"}`), true)
	c.Receive("north/action", []byte("heating"), false)
	c.Receive(north, nil, false)
	if got := nextAt(c, time.Now().Add(time.Minute)); got.key != "" {
		t.Fatalf("issued %+v during discovery removal grace", got)
	}
	c.Receive(north, []byte(`{"action_topic":"north/action"}`), true)
	c.Receive("north/action", []byte("heating"), true)
	c.Disconnected()
	c.boiler = "OFF"
	if got := nextAt(c, time.Now()); got.key != "" {
		t.Fatalf("issued %+v after disconnect using previous demand", got)
	}
	c.boiler = "ON"
	if got := nextAt(c, time.Now().Add(3*time.Minute)); got.key != Stop {
		t.Fatalf("after broker loss and grace = %+v, want stop", got)
	}
}

func TestOutdoorOverrideAndPowerBands(t *testing.T) {
	s := defaultSettings()
	s.Enabled, s.Unit = true, "F"
	s.Thermostats, s.OutdoorSensor = []string{north}, outdoor+"#outside"
	s.StopAt, s.RestartDelta = pointer(65.0), 3
	s.Bands = []Band{{MaxPower: 90}, {From: pointer(50.0), MaxPower: 55}}
	c := newTestControl(t, s)
	c.Receive(north, []byte(`{"action_topic":"north/action"}`), true)
	c.Receive("north/action", []byte("heating"), true)
	c.Receive(outdoor, []byte(`{"~":"weather","avty_t":"~/availability","cmps":{"outside":{"p":"sensor","dev_cla":"temperature","unit_of_meas":"°F","stat_t":"~/state","val_tpl":"{{ value_json.temperature }}"}}}`), true)
	c.Receive("weather/availability", []byte("online"), true)
	c.boiler = "OFF"
	if got := nextAt(c, time.Now()); got.key != "" {
		t.Fatalf("issued %+v before reading outdoor sensor", got)
	}

	c.Receive("weather/state", []byte(`{"temperature":57}`), true)
	c.Observe("regulation", map[string]any{"boiler_power_min": int64(30), "boiler_power_max": int64(90)})
	if got := nextAt(c, time.Now()); got != (command{key: MaxPower, value: 55}) {
		t.Fatalf("with cooler reading = %+v, want 55%% before start", got)
	}
	c.Observe("regulation", map[string]any{"boiler_power_min": int64(60), "boiler_power_max": int64(90)})
	if got := nextAt(c, time.Now()); got.key != "" {
		t.Fatalf("issued %+v for band below controller minimum", got)
	}
	c.Observe("regulation", map[string]any{"boiler_power_min": int64(30), "boiler_power_max": int64(55)})
	if got := nextAt(c, time.Now()); got.key != Start {
		t.Fatalf("confirmed cap = %+v, want start", got)
	}

	c.boiler = "ON"
	c.LastPowerAt, c.LastPowerKey = time.Now(), Start
	c.Receive("weather/state", []byte(`{"temperature":65}`), false)
	if got := nextAt(c, time.Now()); got != (command{key: Stop, value: 1, outdoorStop: true}) {
		t.Fatalf("warm cutoff = %+v, want stop despite active thermostat and cooldown", got)
	}
	c.boiler = "OFF"
	c.LastPowerAt, c.LastPowerKey = time.Now(), Stop
	c.Receive("weather/state", []byte(`{"temperature":63}`), false)
	if got := nextAt(c, time.Now().Add(3*time.Minute)); got.key != "" || !c.cutoff {
		t.Fatalf("inside hysteresis = %+v, cutoff %t", got, c.cutoff)
	}
	c.Receive("weather/state", []byte(`{"temperature":61}`), false)
	if got := nextAt(c, time.Now()); got.key != "" || c.cutoff {
		t.Fatalf("cool but minimum off interval = %+v, cutoff %t", got, c.cutoff)
	}
	now := time.Now().Add(3 * time.Minute)
	c.capAt = now
	if got := nextAt(c, now); got.key != Start {
		t.Fatalf("cold after minimum off interval = %+v, want start", got)
	}
	if got := nextAt(c, time.Now().Add(time.Hour)); got.key != "" {
		t.Fatalf("issued %+v with stale temperature", got)
	}
}

func TestOutdoorOnlyResumesItsOwnStopAfterRestart(t *testing.T) {
	s := defaultSettings()
	s.Enabled, s.OutdoorSensor, s.StopAt = true, outdoor+"#outside", pointer(20.0)
	c := newTestControl(t, s)
	c.write = func(_ context.Context, key string, _ int, current func() bool) error {
		if key != Stop || !current() {
			t.Fatalf("unexpected outdoor-only command %q", key)
		}
		return nil
	}
	discover := func(c *Control) {
		c.Receive(outdoor, []byte(`{"cmps":{"outside":{"p":"sensor","dev_cla":"temperature","unit_of_meas":"°C","stat_t":"outside"}}}`), true)
	}
	discover(c)
	c.Receive("outside", []byte("18"), false)
	c.boiler = "OFF"
	if got := nextAt(c, time.Now()); got.key != "" {
		t.Fatalf("issued %+v for a boiler already off before the cutoff", got)
	}
	c.Receive("outside", []byte("21"), false)
	c.Observe("operating_data", map[string]any{"state_on": "ON"})
	c.apply(t.Context())
	restarted := New(filepath.Dir(c.path), "test", c.logger, c.write)
	discover(restarted)
	restarted.Receive("outside", []byte("17"), false)
	restarted.boiler = "OFF"
	if got := nextAt(restarted, time.Now().Add(3*time.Minute)); got.key != Start {
		t.Fatalf("outdoor cutoff release after restart = %+v, want start", got)
	}
}

func TestZigbeeActionAndAvailability(t *testing.T) {
	specs := readDiscovery(north, "climate", []byte(`{
		"act_t":"zigbee/thermostat","act_tpl":"{% if \"running_state_1\" in value_json %}{% set values = {None:None,'idle':'idle','heat':'heating','cool':'cooling','fan_only':'fan'} %}{{ values[value_json[\"running_state_1\"]] }}{% endif %}",
		"avty":[{"t":"zigbee/bridge","val_tpl":"{{ value_json.state }}"},{"topic":"zigbee/thermostat/availability"}],"avty_mode":"all"
	}`))
	if len(specs) != 1 || specs[0].Problem != "" {
		t.Fatalf("discovery = %+v", specs)
	}
	in := newInput(specs[0])
	now := time.Now()
	in.receive("zigbee/thermostat/availability", []byte("online"), true, now)
	in.receive("zigbee/thermostat", []byte(`{"running_state_1":"heat","system_mode":"heat"}`), true, now)
	if got := in.currentAction(); got != "" {
		t.Fatalf("action available before both availability topics: %q", got)
	}
	in.receive("zigbee/bridge", []byte(`{"state":"online"}`), true, now)
	in.receive("zigbee/thermostat", []byte(`{"local_temperature":19}`), false, now)
	if got := in.currentAction(); got != "heating" {
		t.Fatalf("action after partial payload = %q, want heating", got)
	}
	in.receive("zigbee/thermostat", []byte(`{"running_state_1":"idle","system_mode":"heat"}`), false, now)
	if got := in.currentAction(); got != "idle" {
		t.Fatalf("idle thermostat in heat mode = %q", got)
	}
}

func nextAt(c *Control, now time.Time) command {
	c.boilerAt = now
	got, _ := c.plan(now)
	return got
}

func pointer(value float64) *float64 { return &value }

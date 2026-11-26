package heating

import (
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/internal/hass"
)

func TestHomeAssistantAndMQTTActIndependently(t *testing.T) {
	const zone = "climate.hallway"
	s := defaultSettings()
	s.ZoneControl = true
	s.Thermostats = []string{north, homeAssistantPrefix + zone}
	c := newTestControl(t, s)
	c.Receive(north, []byte(`{"act_t":"north/action"}`), true)
	c.Receive("north/action", []byte("idle"), false)
	c.ObserveHomeAssistant([]hass.Reading{{ID: zone, Name: "Hallway", Action: "heating",
		Available: true, HasValue: true, Value: 21, Unit: "°C", At: time.Now()}})
	c.boiler = "OFF"
	if got := nextAt(c, time.Now()); got.key != Start {
		t.Fatalf("Home Assistant demand = %+v, want start", got)
	}
	c.Disconnected()
	if got := nextAt(c, time.Now()); got.key != Start {
		t.Fatalf("MQTT disconnect interrupted Home Assistant demand: %+v", got)
	}
	c.boiler = "ON"
	c.ObserveHomeAssistant([]hass.Reading{{ID: zone}})
	now := time.Now()
	if got := nextAt(c, now.Add(time.Minute)); got.key != "" {
		t.Fatalf("Home Assistant disconnect bypassed dropout grace: %+v", got)
	}
	c.Receive(north, []byte(`{"act_t":"north/action"}`), true)
	c.Receive("north/action", []byte("heating"), false)
	if got := nextAt(c, now.Add(3*time.Minute)); got.key != "" {
		t.Fatalf("Home Assistant disconnect overrode MQTT demand: %+v", got)
	}
	c.Receive("north/action", []byte("idle"), false)
	if got := nextAt(c, now.Add(3*time.Minute)); got.key != Stop {
		t.Fatalf("all idle or offline after grace = %+v, want stop", got)
	}
}

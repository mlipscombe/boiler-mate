package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/mlipscombe/boiler-mate/config"
	"github.com/mlipscombe/boiler-mate/internal/bridge"
	"github.com/mlipscombe/boiler-mate/internal/heating"
	"github.com/mlipscombe/boiler-mate/nbe"
)

func TestExternalHeatingWithMosquitto(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("set INTEGRATION_TESTS=1 to run with Mosquitto")
	}
	mqttURL := os.Getenv("TEST_MQTT_URL")
	if mqttURL == "" {
		mqttURL = "mqtt://127.0.0.1:1883"
	}
	broker, err := url.Parse(mqttURL)
	if err != nil {
		t.Fatal(err)
	}
	socket := *broker
	socket.Scheme, socket.Path = "tcp", ""
	serial := fmt.Sprintf("HEAT%d", time.Now().UnixNano())
	peer := paho.NewClient(paho.NewClientOptions().AddBroker(socket.String()).SetClientID(serial + "-thermostat"))
	checkToken(t, peer.Connect())
	t.Cleanup(func() { peer.Disconnect(250) })

	device, err := nbe.NewMockBoiler(serial)
	if err != nil {
		t.Fatal(err)
	}
	if err := device.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(device.Stop)
	discovery := "integration-heating/" + serial
	action := discovery + "/inputs/heat"
	availability := discovery + "/inputs/availability"
	reading := discovery + "/inputs/outdoor"
	thermostat := discovery + "/climate/kitchen/config"
	sensor := discovery + "/device/weather/config"
	for topic, payload := range map[string]string{
		thermostat: fmt.Sprintf(`{"name":"Kitchen","act_t":%q,"avty_t":%q}`, action, availability),
		sensor:     fmt.Sprintf(`{"cmps":{"outdoor":{"p":"sensor","name":"Outside","stat_t":%q,"dev_cla":"temperature","unit_of_meas":"°C"}}}`, reading),
		action:     "heating", availability: "online", reading: "16",
	} {
		checkToken(t, peer.Publish(topic, 1, true, payload))
		t.Cleanup(func() { peer.Publish(topic, 1, true, "").WaitTimeout(time.Second) })
	}

	web := unusedAddress(t)
	cfg := config.Config{
		Controller: &url.URL{Scheme: "udp", Host: device.GetAddr(), User: url.UserPassword(serial, "1234567890")},
		MQTT:       broker, EnableWrites: true, Bind: "false", WebBind: web, WebData: t.TempDir(),
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- bridge.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != context.Canceled {
				t.Errorf("bridge exited with %v", err)
			}
		case <-time.After(8 * time.Second):
			t.Error("bridge did not stop")
		}
	})
	client := &http.Client{Timeout: 2 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	endpoint := "http://" + web + "/api/external-heating"
	read := func() (heating.Status, bool) {
		response, err := client.Get(endpoint)
		if err != nil {
			return heating.Status{}, false
		}
		defer response.Body.Close()
		var state heating.Status
		return state, json.NewDecoder(response.Body).Decode(&state) == nil
	}
	write := func(settings heating.Settings) {
		body, err := json.Marshal(settings)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Boiler-Mate", "1")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			message, _ := io.ReadAll(response.Body)
			t.Fatalf("save external heating settings = %d %s", response.StatusCode, message)
		}
	}
	waitHeating(t, "dashboard API", func() bool { _, ok := read(); return ok })
	initial, _ := read()
	settings := initial.Settings
	settings.DiscoveryPrefix = discovery
	write(settings)
	waitHeating(t, "retained thermostat and temperature discovery", func() bool {
		state, _ := read()
		return len(state.Inputs) == 2
	})
	settings.Enabled, settings.Thermostats = true, []string{thermostat}
	settings.OutdoorSensor = sensor + "#outdoor"
	cutoff := 20.0
	settings.StopAt = &cutoff
	write(settings)
	waitHeating(t, "retained state from selected inputs", func() bool {
		state, _ := read()
		return state.OutdoorTemperature != nil && *state.OutdoorTemperature == 16 &&
			strings.Contains(state.Decision, "calling for heat")
	})
	if got := device.WriteAttempts(); got != 0 {
		t.Fatalf("already-running boiler got %d control requests", got)
	}

	checkToken(t, peer.Publish(availability, 1, true, "offline"))
	waitHeating(t, "thermostat dropout grace", func() bool {
		state, _ := read()
		return strings.Contains(state.Decision, "missing thermostat")
	})
	checkToken(t, peer.Publish(availability, 1, true, "online"))
	checkToken(t, peer.Publish(action, 1, true, "heating"))
	checkToken(t, peer.Publish(reading, 1, true, "21"))
	waitHeating(t, "outdoor stop request during a thermostat call", func() bool {
		state, _ := read()
		return state.CutoffActive && device.WriteAttempts() == 1
	})
	device.SetRawReply(nbe.GetSetupRangeFunction, 0, "boiler_power_max=10,100,100,0")
	settings.Bands = []heating.Band{{MaxPower: 75}}
	write(settings)
	var final heating.Status
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("heating decision %q, command error %q, power error %q, write attempts %d",
				final.Decision, final.Error, final.PowerError, device.WriteAttempts())
		}
	})
	waitHeating(t, "validated maximum-power write reaching the controller", func() bool {
		final, _ = read()
		return final.BandPower != nil && *final.BandPower == 75 && device.WriteAttempts() == 2 &&
			strings.Contains(final.PowerError, "controller rejected")
	})
}

func waitHeating(t *testing.T, description string, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for !ready() {
		select {
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", description)
		case <-ticker.C:
		}
	}
}

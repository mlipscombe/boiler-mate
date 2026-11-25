package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/mlipscombe/boiler-mate/config"
	"github.com/mlipscombe/boiler-mate/internal/bridge"
	"github.com/mlipscombe/boiler-mate/nbe"
)

func TestBridgeWithMosquitto(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("set INTEGRATION_TESTS=1 to run with Mosquitto")
	}
	t.Run("PIN alone is read-only", func(t *testing.T) { verifyBridge(t, false) })
	t.Run("write opt-in exposes controls", func(t *testing.T) { verifyBridge(t, true) })
}

func verifyBridge(t *testing.T, enableWrites bool) {
	t.Helper()
	broker := os.Getenv("TEST_MQTT_URL")
	if broker == "" {
		broker = "mqtt://127.0.0.1:1883"
	}
	mqttURI, err := url.Parse(broker)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixNano()
	serial := fmt.Sprintf("SIM%d", now)
	prefix := fmt.Sprintf("boiler-mate-integration/%d", now)
	mqttURI.Path = "/" + prefix

	device, err := nbe.NewMockBoiler(serial)
	if err != nil {
		t.Fatal(err)
	}
	if err := device.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(device.Stop)

	brokerSocket := *mqttURI
	brokerSocket.Scheme = "tcp"
	brokerSocket.Path = ""
	observer := paho.NewClient(paho.NewClientOptions().
		AddBroker(brokerSocket.String()).SetClientID(serial + "-observer").SetConnectTimeout(time.Second))
	checkToken(t, observer.Connect())
	t.Cleanup(func() { observer.Disconnect(250) })

	// A retained command is published only to the isolated test prefix.
	// The test controller rejects all settings writes and records attempts.
	checkToken(t, observer.Publish(prefix+"/set/device/power_switch", 1, true, "ON"))
	readings := make(chan paho.Message, 256)
	onMessage := func(_ paho.Client, msg paho.Message) { readings <- msg }
	controllerStatus := prefix + "/device/controller_status"
	setpointConfig := "homeassistant/number/nbe_" + serial + "/boiler_setpoint/config"
	checkToken(t, observer.SubscribeMultiple(map[string]byte{
		prefix + "/device/status":                                    1,
		controllerStatus:                                             1,
		prefix + "/operating_data/boiler_temp":                       1,
		"homeassistant/sensor/nbe_" + serial + "/boiler_temp/config": 1,
		setpointConfig:                                               1,
	}, onMessage))

	ctx, cancel := context.WithCancel(t.Context())
	metrics := unusedAddress(t)
	web := unusedAddress(t)
	cfg := config.Config{
		Bind: metrics, WebBind: web, WebData: t.TempDir(),
		Controller: &url.URL{Scheme: "udp", Host: device.GetAddr(), User: url.UserPassword(serial, "1234567890")},
		MQTT:       mqttURI, HADiscovery: true, EnableWrites: enableWrites,
	}
	done := make(chan error, 1)
	go func() {
		done <- bridge.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer cancel()

	expected := map[string]string{
		prefix + "/device/status":                                    "online",
		controllerStatus:                                             "online",
		prefix + "/operating_data/boiler_temp":                       "62.50",
		"homeassistant/sensor/nbe_" + serial + "/boiler_temp/config": prefix + "/operating_data/boiler_temp",
		setpointConfig:                                               `"step":0.1`,
	}
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for len(expected) > 0 {
		select {
		case message := <-readings:
			if message.Topic() == controllerStatus && string(message.Payload()) == "offline" {
				continue
			}
			if want, ok := expected[message.Topic()]; ok {
				if !strings.Contains(string(message.Payload()), want) {
					t.Fatalf("%s = %q, wanted %q", message.Topic(), message.Payload(), want)
				}
				delete(expected, message.Topic())
			}
		case <-deadline.C:
			t.Fatalf("did not receive expected MQTT messages: %v", expected)
		}
	}

	// Re-subscribing demonstrates that a separate consumer gets retained state.
	retained := make(chan paho.Message, 1)
	topic := prefix + "/operating_data/boiler_temp"
	checkToken(t, observer.Unsubscribe(topic))
	checkToken(t, observer.Subscribe(topic, 1, func(_ paho.Client, msg paho.Message) {
		select {
		case retained <- msg:
		default:
		}
	}))
	select {
	case message := <-retained:
		if !message.Retained() || string(message.Payload()) != "62.50" {
			t.Fatalf("unexpected retained temperature: %v %q", message.Retained(), message.Payload())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retained telemetry was not replayed")
	}

	httpClient := &http.Client{Timeout: 3 * time.Second}
	if got := get(t, httpClient, metrics, "/healthz"); got != `{"healthy":true}` {
		t.Errorf("healthz = %q", got)
	}
	if got := get(t, httpClient, metrics, "/metrics"); !strings.Contains(got, `boiler_mate_operating_data_boiler_temp{serial="`+serial+`"} 62.5`) {
		t.Error("operating temperature metric not found")
	}
	if got := get(t, httpClient, web, "/"); !strings.Contains(got, "Boiler Mate") {
		t.Error("embedded dashboard not found")
	}
	var data struct{ Online, Control bool }
	if err := json.Unmarshal([]byte(get(t, httpClient, web, "/api/status")), &data); err != nil || !data.Online || data.Control != enableWrites {
		t.Fatalf("expected a live dashboard with controller writes %t, got %+v (%v)", enableWrites, data, err)
	}
	if got := get(t, httpClient, web, "/api/history?series=operating_data.boiler_temp"); !strings.Contains(got, `"value":62.5`) {
		t.Errorf("temperature history missing: %s", got)
	}

	if !enableWrites {
		verifyControllerReads(t, observer, prefix)
	}
	if writes := device.WriteAttempts(); writes != 0 {
		t.Fatalf("%d settings writes reached the simulator", writes)
	}
	if enableWrites {
		device.Stop()
		select {
		case message := <-readings:
			if message.Topic() != controllerStatus || string(message.Payload()) != "offline" {
				t.Fatalf("unexpected availability after controller loss: %s: %q", message.Topic(), message.Payload())
			}
		case <-time.After(28 * time.Second):
			t.Fatal("controller loss did not publish offline status")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected shutdown result: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("bridge did not shut down")
	}
	select {
	case message := <-readings:
		if message.Topic() != prefix+"/device/status" || string(message.Payload()) != "offline" {
			t.Errorf("unexpected shutdown MQTT message %s: %q", message.Topic(), message.Payload())
		}
	case <-time.After(3 * time.Second):
		t.Error("bridge did not publish offline status")
	}
}

func verifyControllerReads(t *testing.T, observer paho.Client, prefix string) {
	t.Helper()
	replies := make(chan paho.Message, 32)
	checkToken(t, observer.Subscribe(prefix+"/controller/+", 1, func(_ paho.Client, msg paho.Message) {
		replies <- msg
	}))
	for _, tc := range []struct{ kind, query, topic, want string }{
		{"versions", "", "versions", `"version":"0616"`},
		{"sensors", "", "sensors", `"temperature":21.5`},
		{"events", "", "events", `"type":0`},
		{"events", "at=1732442400", "event_history", `"query":"at=1732442400"`},
		{"values", "names=boiler_temp,state", "values", `"boiler_temp":62.50`},
		{"chart", "from=1732442200&to=1732442400&ids=0,3", "chart", `"values":[0,3]`},
		{"values", "names=a,a", "values", `"error":`},
	} {
		checkToken(t, observer.Publish(prefix+"/get/controller/"+tc.kind, 1, false, tc.query))
		timeout := time.After(4 * time.Second)
	waitReply:
		for {
			select {
			case msg := <-replies:
				if msg.Topic() != prefix+"/controller/"+tc.topic {
					continue
				}
				if !strings.Contains(string(msg.Payload()), tc.want) {
					t.Fatalf("%s returned %s", tc.kind, msg.Payload())
				}
				break waitReply
			case <-timeout:
				t.Fatalf("no MQTT response for %s", tc.kind)
			}
		}
	}
}

func checkToken(t *testing.T, token paho.Token) {
	t.Helper()
	if !token.WaitTimeout(4 * time.Second) {
		t.Fatal("MQTT operation timed out")
	}
	if err := token.Error(); err != nil {
		t.Fatal(err)
	}
}

func unusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func get(t *testing.T, client *http.Client, address, path string) string {
	t.Helper()
	response, err := client.Get("http://" + address + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s (%v)", path, response.StatusCode, data, err)
	}
	return string(data)
}

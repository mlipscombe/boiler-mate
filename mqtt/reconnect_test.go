package mqtt

import (
	"log/slog"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

func TestClientReplaysLatestStateAfterBrokerLoss(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	uri := &url.URL{Scheme: "mqtt", Host: listener.Addr().String()}
	broker, err := PrepareBroker(uri)
	if err != nil {
		t.Fatal(err)
	}
	client := broker.Open(t.Context(), slog.Default(), "reconnect-test", "test/boiler")
	t.Cleanup(client.Close)

	const telemetry = "test/boiler/operating_data/power_pct"
	const discovery = "homeassistant/sensor/test/power_pct/config"
	const command = "test/boiler/set/device/power_switch"
	received := make(chan string, 1)
	if err := client.Subscribe("set/+/+", 1, func(_ *Client, msg Message) {
		received <- string(msg.Payload())
	}); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishRaw(telemetry, "10"); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishJSON(discovery, map[string]string{"stat_t": telemetry}); err != nil {
		t.Fatal(err)
	}

	// Refuse the first connection to exercise a broker outage at startup.
	first := acceptBrokerConnection(t, listener)
	readBrokerConnect(t, first)
	_ = first.Close()

	initial := acceptBrokerConnection(t, listener)
	ackBrokerConnect(t, initial)
	checkBrokerReplay(t, initial, map[string]string{
		"test/boiler/device/status": "online",
		telemetry:                   "10",
		discovery:                   `{"stat_t":"test/boiler/operating_data/power_pct"}`,
	})
	if err := client.PublishRaw(telemetry, "15"); err != nil {
		t.Fatal(err)
	}
	packet, err := packets.ReadPacket(initial)
	if err != nil {
		t.Fatal(err)
	}
	inFlight, ok := packet.(*packets.PublishPacket)
	if !ok || inFlight.TopicName != telemetry || string(inFlight.Payload) != "15" {
		t.Fatalf("expected a live telemetry update before the outage, got %v", packet)
	}
	// Drop the socket before the update is acknowledged.
	_ = initial.Close()

	// The next socket has no broker state and cannot deliver publishes before CONNACK.
	recovered := acceptBrokerConnection(t, listener)
	readBrokerConnect(t, recovered)
	for _, value := range []string{"20", "30"} {
		if err := client.PublishRaw(telemetry, value); err != nil {
			t.Fatal(err)
		}
	}
	writeBrokerPacket(t, recovered, packets.NewControlPacket(packets.Connack))
	checkBrokerReplay(t, recovered, map[string]string{
		"test/boiler/device/status": "online",
		telemetry:                   "30",
		discovery:                   `{"stat_t":"test/boiler/operating_data/power_pct"}`,
	})

	publish := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	publish.TopicName = command
	publish.Payload = []byte("ON")
	writeBrokerPacket(t, recovered, publish)
	select {
	case value := <-received:
		if value != "ON" {
			t.Fatalf("received %q after reconnect, want ON", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command subscription did not resume")
	}
}

func acceptBrokerConnection(t *testing.T, listener *net.TCPListener) net.Conn {
	t.Helper()
	if err := listener.SetDeadline(time.Now().Add(12 * time.Second)); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func readBrokerConnect(t *testing.T, conn net.Conn) {
	t.Helper()
	packet, err := packets.ReadPacket(conn)
	if err != nil {
		t.Fatal(err)
	}
	connect, ok := packet.(*packets.ConnectPacket)
	if !ok || !connect.CleanSession || !connect.WillRetain || string(connect.WillMessage) != "offline" {
		t.Fatalf("expected a clean CONNECT with a retained offline will, got %v", packet)
	}
}

func ackBrokerConnect(t *testing.T, conn net.Conn) {
	t.Helper()
	readBrokerConnect(t, conn)
	writeBrokerPacket(t, conn, packets.NewControlPacket(packets.Connack))
}

func checkBrokerReplay(t *testing.T, conn net.Conn, expected map[string]string) {
	t.Helper()
	packet, err := packets.ReadPacket(conn)
	if err != nil {
		t.Fatal(err)
	}
	sub, ok := packet.(*packets.SubscribePacket)
	if !ok || len(sub.Topics) != 1 || sub.Topics[0] != "test/boiler/set/+/+" {
		t.Fatalf("expected command subscription, got %v", packet)
	}
	suback := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
	suback.MessageID = sub.MessageID
	suback.ReturnCodes = []byte{1}
	writeBrokerPacket(t, conn, suback)

	for len(expected) > 0 {
		packet, err := packets.ReadPacket(conn)
		if err != nil {
			t.Fatal(err)
		}
		pub, ok := packet.(*packets.PublishPacket)
		if !ok || !pub.Retain || pub.Qos != 1 {
			t.Fatalf("expected a retained QoS 1 publish, got %v", packet)
		}
		value, exists := expected[pub.TopicName]
		if !exists || string(pub.Payload) != value {
			t.Fatalf("unexpected publish for %s: %q", pub.TopicName, pub.Payload)
		}
		delete(expected, pub.TopicName)
		ack := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
		ack.MessageID = pub.MessageID
		writeBrokerPacket(t, conn, ack)
	}
}

func writeBrokerPacket(t *testing.T, conn net.Conn, packet packets.ControlPacket) {
	t.Helper()
	if err := packet.Write(conn); err != nil {
		t.Fatal(err)
	}
}

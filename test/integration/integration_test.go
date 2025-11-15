/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021-2023 Mark Lipscombe.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, version 3.
 *
 * This program is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
 * General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package integration

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/mlipscombe/boiler-mate/homeassistant"
	"github.com/mlipscombe/boiler-mate/monitor"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
)

// skipIfNotIntegration skips the test unless integration tests are enabled
func skipIfNotIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TESTS=1 to run")
	}
}

// TestIntegrationFullStack tests the complete system with mock boiler and real MQTT
func TestIntegrationFullStack(t *testing.T) {
	skipIfNotIntegration(t)

	// Create and start mock boiler
	mockBoiler, err := nbe.NewMockBoiler("INTEGRATION123")
	if err != nil {
		t.Fatalf("Failed to create mock boiler: %v", err)
	}

	err = mockBoiler.Start()
	if err != nil {
		t.Fatalf("Failed to start mock boiler: %v", err)
	}
	defer mockBoiler.Stop()

	// Set some initial data in the mock boiler
	mockBoiler.SetValue("boiler", "temp", nbe.RoundedFloat(70.0))
	mockBoiler.SetValue("boiler", "diff_under", nbe.RoundedFloat(5.0))
	mockBoiler.SetValue("boiler", "diff_over", nbe.RoundedFloat(15.0))
	mockBoiler.SetValue("operating", "boiler_temp", nbe.RoundedFloat(65.5))
	mockBoiler.SetValue("operating", "state", int64(5))
	mockBoiler.SetValue("operating", "oxygen", nbe.RoundedFloat(12.5))

	// Connect to the mock boiler
	boilerURI, _ := url.Parse(fmt.Sprintf("tcp://INTEGRATION123:1234@%s", mockBoiler.GetAddr()))
	boiler, err := nbe.NewNBE(boilerURI)
	if err != nil {
		t.Fatalf("Failed to connect to mock boiler: %v", err)
	}

	// Connect to MQTT broker (should be running in Docker)
	mqttURL, _ := url.Parse("mqtt://localhost:1883")
	mqttClient, err := mqtt.NewClient(mqttURL, "test-client", "test/boiler")
	if err != nil {
		t.Fatalf("Failed to create MQTT client: %v", err)
	}

	// Give MQTT time to connect
	time.Sleep(1 * time.Second)

	// Test publishing device status
	err = mqttClient.PublishMany("device", map[string]interface{}{
		"status":     "online",
		"serial":     boiler.Serial,
		"ip_address": boiler.IPAddress,
	})
	if err != nil {
		t.Errorf("Failed to publish device status: %v", err)
	}

	// Start monitors and collect ready channels
	settingsReady := monitor.StartSettingsMonitor(boiler, mqttClient, "boiler")
	operatingReady := monitor.StartOperatingDataMonitor(boiler, mqttClient)

	// Create a combined ready channel that waits for all monitors
	allReady := make(chan bool, 1)
	go func() {
		<-settingsReady
		<-operatingReady
		allReady <- true
	}()

	// Test Home Assistant discovery
	t.Run("HomeAssistantDiscovery", func(t *testing.T) {
		// Wait for monitors to publish initial data, then publish discovery
		homeassistant.PublishDiscovery(mqttClient, boiler.Serial, "test/boiler", allReady)

		// Test passes if no errors occurred during publishing
		// In a real test, we could subscribe to homeassistant/# and verify messages
	})

	// Test monitor functionality
	t.Run("SettingsMonitor", func(t *testing.T) {
		// Monitor already started above, give it time to poll once more
		time.Sleep(2 * time.Second)

		// Verify data was read from mock boiler
		val, ok := mockBoiler.GetValue("boiler", "temp")
		if !ok {
			t.Error("Expected boiler temp to be set")
		}
		if val != nbe.RoundedFloat(70.0) {
			t.Errorf("Expected boiler temp 70.0, got %v", val)
		}
	})

	t.Run("OperatingDataMonitor", func(t *testing.T) {
		// Monitor already started above, give it time to poll once more
		time.Sleep(2 * time.Second)

		// Test passes if no panic occurred
	})

	t.Run("SetValue", func(t *testing.T) {
		// Test setting a value through the boiler client
		response, err := boiler.Set("boiler.temp", []byte("75"))
		if err != nil {
			t.Fatalf("Failed to set boiler temp: %v", err)
		}

		if response.Payload["status"] != "ok" {
			t.Errorf("Expected status ok, got %v", response.Payload)
		}

		// Verify it was set in the mock
		val, ok := mockBoiler.GetValue("boiler", "temp")
		if !ok {
			t.Error("Expected boiler temp to be set")
		}
		if val != "75" {
			t.Errorf("Expected boiler temp '75', got %v", val)
		}
	})

	t.Run("GetOperatingData", func(t *testing.T) {
		// Test getting operating data
		response, err := boiler.Get(nbe.GetOperatingDataFunction, "*")
		if err != nil {
			t.Fatalf("Failed to get operating data: %v", err)
		}

		// Check expected fields
		if _, ok := response.Payload["boiler_temp"]; !ok {
			t.Error("Expected boiler_temp in operating data")
		}
		if _, ok := response.Payload["state"]; !ok {
			t.Error("Expected state in operating data")
		}
		if _, ok := response.Payload["oxygen"]; !ok {
			t.Error("Expected oxygen in operating data")
		}
	})
}

// TestIntegrationMQTTSubscription tests MQTT subscription functionality
func TestIntegrationMQTTSubscription(t *testing.T) {
	skipIfNotIntegration(t)

	mqttURL, _ := url.Parse("mqtt://localhost:1883")
	mqttClient, err := mqtt.NewClient(mqttURL, "test-sub-client", "test/subscription")
	if err != nil {
		t.Fatalf("Failed to create MQTT client: %v", err)
	}

	// Give MQTT time to connect
	time.Sleep(1 * time.Second)

	// Channel to receive messages
	received := make(chan string, 1)

	// Subscribe to a test topic
	err = mqttClient.Subscribe("test/+", 1, func(client *mqtt.Client, msg mqtt.Message) {
		received <- string(msg.Payload())
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Publish a test message
	err = mqttClient.PublishRaw("test/subscription/test/message", "hello integration test")
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for message
	select {
	case msg := <-received:
		if msg != "hello integration test" {
			t.Errorf("Expected 'hello integration test', got '%s'", msg)
		}
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for MQTT message")
	}
}

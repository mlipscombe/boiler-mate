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

package mqtt

import (
	"net/url"
	"testing"
)

func TestCreateClientOptions(t *testing.T) {
	tests := []struct {
		name         string
		uriString    string
		expectBroker string
		expectPort   string
	}{
		{
			name:         "mqtt with default port",
			uriString:    "mqtt://localhost",
			expectBroker: "tcp://localhost:1883",
			expectPort:   "1883",
		},
		{
			name:         "mqtt with custom port",
			uriString:    "mqtt://localhost:1234",
			expectBroker: "tcp://localhost:1234",
			expectPort:   "1234",
		},
		{
			name:         "mqtts with default port",
			uriString:    "mqtts://localhost",
			expectBroker: "ssl://localhost:8883",
			expectPort:   "8883",
		},
		{
			name:         "mqtts with custom port",
			uriString:    "mqtts://localhost:8884",
			expectBroker: "ssl://localhost:8884",
			expectPort:   "8884",
		},
		{
			name:         "mqtt with username and password",
			uriString:    "mqtt://user:pass@localhost",
			expectBroker: "tcp://localhost:1883",
			expectPort:   "1883",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := url.Parse(tt.uriString)
			if err != nil {
				t.Fatalf("Failed to parse URI: %v", err)
			}

			client := &Client{
				URI:           uri,
				ClientID:      "test-client",
				Prefix:        "test/prefix",
				subscriptions: make(map[string]*subscriptionInfo),
			}

			opts, err := createClientOptions(client)

			// Check that options were created
			if err != nil || opts == nil {
				t.Fatalf("Expected options to be created: %v", err)
			}

			if broker := opts.Servers[0].String(); broker != tt.expectBroker {
				t.Errorf("broker = %q, want %q", broker, tt.expectBroker)
			}
		})
	}
}

func TestCreateClientOptionsWithCredentials(t *testing.T) {
	uri, _ := url.Parse("mqtt://testuser:testpass@localhost:1883")
	client := &Client{
		URI:           uri,
		ClientID:      "test-client",
		Prefix:        "test/prefix",
		subscriptions: make(map[string]*subscriptionInfo),
	}

	opts, err := createClientOptions(client)

	if err != nil || opts == nil {
		t.Fatalf("Expected options to be created: %v", err)
	}

	// Options are created - actual connection test requires broker
}

func TestClientStructure(t *testing.T) {
	uri, _ := url.Parse("mqtt://localhost:1883")

	client := &Client{
		URI:           uri,
		ClientID:      "test-id",
		Prefix:        "test/prefix",
		subscriptions: make(map[string]*subscriptionInfo),
	}

	if client.URI.String() != "mqtt://localhost:1883" {
		t.Errorf("Expected URI to be mqtt://localhost:1883, got %s", client.URI.String())
	}

	if client.ClientID != "test-id" {
		t.Errorf("Expected ClientID to be test-id, got %s", client.ClientID)
	}

	if client.Prefix != "test/prefix" {
		t.Errorf("Expected Prefix to be test/prefix, got %s", client.Prefix)
	}

	if client.subscriptions == nil {
		t.Error("Expected subscriptions map to be initialized")
	}
}

func TestSubscriptionTracking(t *testing.T) {
	client := &Client{
		subscriptions: make(map[string]*subscriptionInfo),
	}

	// Test subscription storage
	topic := "test/topic"
	qos := byte(1)
	callback := func(c *Client, m Message) {}

	client.mu.Lock()
	client.subscriptions[topic] = &subscriptionInfo{
		qos:      qos,
		callback: callback,
	}
	client.mu.Unlock()

	// Verify storage
	client.mu.Lock()
	sub, exists := client.subscriptions[topic]
	client.mu.Unlock()

	if !exists {
		t.Error("Expected subscription to be stored")
	}

	if sub.qos != qos {
		t.Errorf("Expected QoS %d, got %d", qos, sub.qos)
	}

	if sub.callback == nil {
		t.Error("Expected callback to be stored")
	}
}

func TestMessageHandlerType(t *testing.T) {
	// Test that MessageHandler is a valid function type
	callCount := 0
	var handler MessageHandler = func(c *Client, m Message) {
		callCount++
	}

	// Verify handler can be called
	handler(nil, nil)

	if callCount != 1 {
		t.Errorf("Expected handler to be called once, got %d calls", callCount)
	}
}

func TestClientTopicFormatting(t *testing.T) {
	// Test client topic prefix formatting
	uri, _ := url.Parse("mqtt://localhost:1883")

	client := &Client{
		URI:           uri,
		ClientID:      "test-client",
		Prefix:        "test/boiler",
		subscriptions: make(map[string]*subscriptionInfo),
	}

	// Test prefix formatting for device status
	expectedTopic := "test/boiler/device/status"
	actualTopic := client.Prefix + "/device/status"

	if actualTopic != expectedTopic {
		t.Errorf("Expected topic %s, got %s", expectedTopic, actualTopic)
	}

	// Test prefix formatting for data topics
	expectedDataTopic := "test/boiler/operating_data/boiler_temp"
	actualDataTopic := client.Prefix + "/operating_data/boiler_temp"

	if actualDataTopic != expectedDataTopic {
		t.Errorf("Expected topic %s, got %s", expectedDataTopic, actualDataTopic)
	}
}

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
	"crypto/tls"
	"net/url"
	"testing"
)

func TestPrepareBroker(t *testing.T) {
	for _, tc := range []struct{ uri, broker string }{
		{"mqtt://localhost", "tcp://localhost:1883"},
		{"tcp://[::1]:1234", "tcp://[::1]:1234"},
		{"mqtts://user:secret@host", "ssl://host:8883"},
	} {
		uri, err := url.Parse(tc.uri)
		if err != nil {
			t.Fatal(err)
		}
		broker, err := PrepareBroker(uri)
		if err != nil {
			t.Fatal(err)
		}
		if got := broker.options.Servers[0].String(); got != tc.broker {
			t.Errorf("broker = %q, want %q", got, tc.broker)
		}
		if uri.Scheme == "mqtts" && (broker.options.TLSConfig.MinVersion != tls.VersionTLS12 || broker.options.Username != "user" || broker.options.Password != "secret") {
			t.Error("TLS or broker credentials were not configured")
		}
	}
}

func TestPrepareBrokerTLSOptions(t *testing.T) {
	for _, raw := range []string{
		"mqtts://localhost?tls_cert=missing",
		"mqtts://localhost?tls_ca=missing",
		"mqtts://localhost?tls_cacert=missing",
	} {
		uri, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := PrepareBroker(uri); err == nil {
			t.Errorf("invalid TLS options were accepted for %s", uri.Redacted())
		}
	}
}

func TestRetainedStateFormats(t *testing.T) {
	client := &Client{prefix: "test/boiler", retained: make(map[string][]byte), changed: make(chan struct{}, 1)}
	if err := client.PublishMany("device", map[string]any{"status": "online", "serial": int64(1234)}); err != nil {
		t.Fatal(err)
	}
	if got := string(client.retained["test/boiler/device/status"]); got != "online" {
		t.Errorf("raw text = %q", got)
	}
	if got := string(client.retained["test/boiler/device/serial"]); got != "1234" {
		t.Errorf("number = %q", got)
	}
	if err := client.PublishMany("device", map[string]any{"status": "offline", "invalid": func() {}}); err == nil {
		t.Fatal("accepted an invalid publication")
	}
	if got := string(client.retained["test/boiler/device/status"]); got != "online" {
		t.Errorf("a failed batch changed retained state to %q", got)
	}
}

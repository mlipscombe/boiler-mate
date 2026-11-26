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

package config

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("BOILER_MATE_MQTT", "mqtt://broker.local/test")
	t.Setenv("BOILER_MATE_HOMEASSISTANT", "yes")
	cfg, err := Load([]string{"--controller", "udp://1234@boiler.local:8483", "--bind", "false", "--mqtt", "mqtts://another.local", "--homeassistant=false"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Controller.Host != "boiler.local:8483" || cfg.MQTT.Host != "another.local" || cfg.HADiscovery || cfg.Bind != "false" {
		t.Fatal("flags and environment were not applied in the expected order")
	}
	cfg, err = Load(nil, io.Discard)
	if err != nil || cfg.MQTT.Host != "broker.local" || !cfg.HADiscovery {
		t.Fatalf("expected environment overrides, got %v", err)
	}
}

func TestWebBindDefaultsAndOverrides(t *testing.T) {
	const key = "BOILER_MATE_WEB_BIND"
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(nil, io.Discard)
	if err != nil || cfg.WebBind != "127.0.0.1:8080" {
		t.Fatalf("default dashboard bind = %q (%v)", cfg.WebBind, err)
	}

	t.Setenv(key, "0.0.0.0:8081")
	cfg, err = Load(nil, io.Discard)
	if err != nil || cfg.WebBind != "0.0.0.0:8081" {
		t.Fatalf("dashboard bind from environment = %q (%v)", cfg.WebBind, err)
	}

	cfg, err = Load([]string{"--web-bind=false"}, io.Discard)
	if err != nil || cfg.WebBind != "false" {
		t.Fatalf("dashboard bind from flag = %q (%v)", cfg.WebBind, err)
	}
}

func TestWritesRequireOptIn(t *testing.T) {
	args := []string{"--controller", "udp://1234:1234567890@boiler.local:8483"}
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"true", true}} {
		t.Setenv("BOILER_MATE_ENABLE_WRITES", tc.value)
		cfg, err := Load(args, io.Discard)
		if err != nil || cfg.EnableWrites != tc.want {
			t.Fatalf("writes with opt-in %q = %t, want %t (%v)", tc.value, cfg.EnableWrites, tc.want, err)
		}
	}
}

func TestLoadRedactsURLs(t *testing.T) {
	for _, args := range [][]string{
		{"--controller", "udp://1234:secret@%bogus:8483"},
		{"--controller", "udp://1234:verylongsecret@boiler.local:8483"},
		{"--mqtt", "unsupported://name:secret@host"},
		{"--mqtt", "mqtt://name:secret@host:65536"},
		{"--mqtt", "mqtt://name:secret@host/%23"},
	} {
		_, err := Load(args, io.Discard)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected a redacted validation error, got %v", err)
		}
	}
}

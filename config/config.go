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
	"flag"
	"os"

	log "github.com/sirupsen/logrus"
)

// Config holds application configuration
type Config struct {
	LogLevel      string
	Bind          string
	ControllerURL string
	MQTTURL       string
	HADiscovery   bool
}

// Load parses command-line flags and environment variables
func Load() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.LogLevel, "log-level", lookupEnvOrString("BOILER_MATE_LOG_LEVEL", "INFO"), "logging level")
	flag.StringVar(&cfg.Bind, "bind", lookupEnvOrString("BOILER_MATE_BIND", "0.0.0.0:2112"), "address to bind for healthz and prometheus metrics endpoints (default 0.0.0.0:2112), or \"false\" to disable")
	flag.StringVar(&cfg.ControllerURL, "controller", lookupEnvOrString("BOILER_MATE_CONTROLLER", "tcp://00000:0123456789@192.168.1.100:8483"), "controller URI, in the format tcp://<serial>:<password>@<host>:<port>")
	flag.StringVar(&cfg.MQTTURL, "mqtt", lookupEnvOrString("BOILER_MATE_MQTT", "mqtt[s]://localhost:1883"), "MQTT URI, in the format mqtt[s]://[<user>:<password>]@<host>:<port>[/<prefix>]")
	flag.BoolVar(&cfg.HADiscovery, "homeassistant", lookupEnvOrBool("BOILER_MATE_HOMEASSISTANT", true), "enable Home Assistant autodiscovery (default: true)")
	flag.Parse()

	return cfg
}

// SetupLogging configures the logging level
func (cfg *Config) SetupLogging() {
	log.SetFormatter(&log.TextFormatter{})
	ll, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)
}

func lookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func lookupEnvOrBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if val == "true" || val == "1" || val == "yes" {
			return true
		}
		return false
	}
	return defaultVal
}

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
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	LogLevel     slog.Level
	Bind         string
	Controller   *url.URL
	MQTT         *url.URL
	HADiscovery  bool
	EnableWrites bool
	WebBind      string
	WebData      string
}

func Load(args []string, output io.Writer) (Config, error) {
	var cfg Config
	flags := flag.NewFlagSet("boiler-mate", flag.ContinueOnError)
	flags.SetOutput(output)
	logLevel := flags.String("log-level", lookupEnvOrString("BOILER_MATE_LOG_LEVEL", "info"), "logging level")
	flags.StringVar(&cfg.Bind, "bind", lookupEnvOrString("BOILER_MATE_BIND", "0.0.0.0:2112"), "health and metrics listen address, or false")
	// Keep the historical controller default without listing a PIN in help.
	controller := flags.String("controller", "", "controller URI, udp://[serial[:password]@]host:port")
	mqtt := flags.String("mqtt", "", "broker URI, mqtt[s]://[user:password@]host[:port][/prefix]")
	flags.BoolVar(&cfg.HADiscovery, "homeassistant", lookupEnvOrBool("BOILER_MATE_HOMEASSISTANT", true), "enable Home Assistant discovery")
	flags.StringVar(&cfg.WebBind, "web-bind", lookupEnvOrString("BOILER_MATE_WEB_BIND", "false"), "web interface listen address, or false")
	flags.StringVar(&cfg.WebData, "web-data", lookupEnvOrString("BOILER_MATE_WEB_DATA", "boiler-mate-data"), "web interface data directory")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}
	if flags.NArg() != 0 {
		return cfg, errors.New("unexpected positional arguments")
	}
	// Apply defaults only when the flag was absent, including explicitly empty flags.
	set := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if !set["controller"] {
		*controller = lookupEnvOrString("BOILER_MATE_CONTROLLER", "tcp://00000:0123456789@192.168.1.100:8483")
	}
	if !set["mqtt"] {
		*mqtt = lookupEnvOrString("BOILER_MATE_MQTT", "mqtt://localhost:1883")
	}
	var err error
	cfg.Controller, err = parseURL(*controller, "controller", "udp", "tcp")
	if err != nil {
		return cfg, err
	}
	if cfg.Controller.Port() == "" {
		return cfg, errors.New("controller URL requires a port")
	}
	if pin, _ := cfg.Controller.User.Password(); len(pin) > 10 {
		return cfg, errors.New("controller password exceeds the NBE PIN length")
	}
	cfg.MQTT, err = parseURL(*mqtt, "MQTT", "mqtt", "mqtts", "tcp")
	if err != nil {
		return cfg, err
	}
	if strings.ContainsAny(cfg.MQTT.Path, "+#\x00") {
		return cfg, errors.New("invalid MQTT topic prefix")
	}
	for _, addr := range []struct{ name, value string }{{"bind", cfg.Bind}, {"web-bind", cfg.WebBind}} {
		if addr.value != "false" {
			if err := validateBind(addr.value); err != nil {
				return cfg, fmt.Errorf("invalid %s address: %w", addr.name, err)
			}
		}
	}
	if cfg.WebBind != "false" && strings.TrimSpace(cfg.WebData) == "" {
		return cfg, errors.New("web data directory is required")
	}
	cfg.LogLevel = parseLevel(*logLevel)
	cfg.EnableWrites = lookupEnvOrBool("BOILER_MATE_ENABLE_WRITES", false)
	return cfg, nil
}

func parseURL(value, name string, schemes ...string) (*url.URL, error) {
	uri, err := url.Parse(value)
	if err != nil || uri.Hostname() == "" || uri.Fragment != "" {
		return nil, fmt.Errorf("invalid %s URL", name)
	}
	allowed := false
	for _, scheme := range schemes {
		allowed = allowed || uri.Scheme == scheme
	}
	if !allowed {
		return nil, fmt.Errorf("unsupported %s URL scheme", name)
	}
	if port := uri.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return nil, fmt.Errorf("invalid %s port", name)
		}
	}
	if _, err := url.ParseQuery(uri.RawQuery); err != nil {
		return nil, fmt.Errorf("invalid %s URL query", name)
	}
	return uri, nil
}

func validateBind(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("expected host:port")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return errors.New("expected a numeric port")
	}
	return nil
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(value) {
	case "trace":
		return slog.LevelDebug - 4
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "fatal":
		return slog.LevelError + 4
	case "panic":
		return slog.LevelError + 8
	default:
		return slog.LevelInfo
	}
}

func lookupEnvOrString(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func lookupEnvOrBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		return value == "true" || value == "1" || value == "yes"
	}
	return fallback
}

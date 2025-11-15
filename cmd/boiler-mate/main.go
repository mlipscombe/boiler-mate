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

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	healthz "github.com/klyve/go-healthz"
	"github.com/mlipscombe/boiler-mate/config"
	"github.com/mlipscombe/boiler-mate/homeassistant"
	"github.com/mlipscombe/boiler-mate/monitor"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// determineMQTTPrefix extracts the MQTT prefix from the URL path, or generates one from the serial
func determineMQTTPrefix(mqttURL *url.URL, serial string) string {
	if len(mqttURL.Path) > 1 {
		return mqttURL.Path[1:]
	}
	return fmt.Sprintf("nbe/%s", serial)
}

// parseSetTopic extracts the key from a set topic (e.g., "prefix/set/category/param" -> "category.param")
func parseSetTopic(topic string) string {
	topicParts := strings.Split(topic, "/")
	if len(topicParts) < 2 {
		return ""
	}
	return fmt.Sprintf("%s.%s", topicParts[len(topicParts)-2], topicParts[len(topicParts)-1])
}

// translatePowerCommand translates device.power_switch commands to misc.start/stop
func translatePowerCommand(key string, value []byte) (string, []byte) {
	if key != "device.power_switch" {
		return key, value
	}

	valueStr := string(value)
	if valueStr == "ON" || valueStr == "1" {
		return "misc.start", []byte("1")
	}
	return "misc.stop", []byte("1")
}

func main() {
	cfg := config.Load()
	cfg.SetupLogging()

	if cfg.Bind != "false" {
		go func(listenAddress string) {
			log.Infof("Starting metrics server on %s", listenAddress)
			instance := healthz.Instance{
				Logger:   log.New(),
				Detailed: true,
			}

			http.Handle("/metrics", promhttp.Handler())
			http.Handle("/healthz", instance.Healthz())
			http.Handle("/liveness", instance.Liveness())

			if err := http.ListenAndServe(listenAddress, nil); err != nil {
				log.Errorf("HTTP server error: %v", err)
			}
		}(cfg.Bind)
	}

	uri, err := url.Parse(cfg.ControllerURL)
	if err != nil {
		panic(err)
	}
	boiler, err := nbe.NewNBE(uri)
	if err != nil {
		panic(err)
	}

	doneChan := make(chan error, 1)
	log.Infof("Connected to boiler at %s (serial: %s)", uri.Host, boiler.Serial)

	mqttUrl, err := url.Parse(cfg.MQTTURL)
	if err != nil {
		log.Fatalf("Invalid MQTT URL: %s", cfg.MQTTURL)
		os.Exit(1)
	}

	mqttPrefix := determineMQTTPrefix(mqttUrl, boiler.Serial)
	mqttClient, err := mqtt.NewClient(mqttUrl, fmt.Sprintf("nbemqtt-%s", boiler.Serial), mqttPrefix)

	if err != nil {
		log.Errorf("Failed to create MQTT client: %s", err)
		os.Exit(1)
	}

	log.Infof("Connected to MQTT broker %s (publishing on \"%s\")", mqttUrl.Host, mqttPrefix)

	if err := mqttClient.Subscribe("set/+/+", 1, func(client *mqtt.Client, msg mqtt.Message) {
		key := parseSetTopic(msg.Topic())
		value := msg.Payload()

		// Translate power switch commands
		key, value = translatePowerCommand(key, value)

		_, err := boiler.SetAsync(key, value, func(response *nbe.NBEResponse) {
			log.Infof("Set %s to %s: %v", key, value, response)
		})
		if err != nil {
			log.Errorf("Failed to set %s to %s: %v", key, value, err)
		}
	}); err != nil {
		log.Errorf("Failed to subscribe to set topics: %v", err)
	}

	go func() {
		if err := mqttClient.PublishMany("device", map[string]interface{}{
			"status":     "online",
			"serial":     boiler.Serial,
			"ip_address": boiler.IPAddress,
		}); err != nil {
			log.Errorf("Failed to publish device status: %v", err)
		}
	}()

	// Start settings monitors for each category and collect ready channels
	var settingsReady []chan bool
	for _, category := range nbe.Settings {
		ready := monitor.StartSettingsMonitor(boiler, mqttClient, category)
		settingsReady = append(settingsReady, ready)
	}

	// Start operating data monitor
	operatingReady := monitor.StartOperatingDataMonitor(boiler, mqttClient)

	// Start advanced data monitor (doesn't return ready channel yet)
	monitor.StartAdvancedDataMonitor(boiler, mqttClient)

	if cfg.HADiscovery {
		go func() {
			// Combine all ready signals
			allReady := make(chan bool, 1)
			go func() {
				// Wait for all settings categories
				for _, ready := range settingsReady {
					<-ready
				}
				// Wait for operating data
				<-operatingReady
				// Signal all ready
				allReady <- true
			}()

			homeassistant.PublishDiscovery(mqttClient, boiler.Serial, mqttPrefix, allReady)
			time.Sleep(2 * time.Minute)
		}()
	}

	err = <-doneChan

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

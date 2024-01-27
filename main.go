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
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	cmp "github.com/google/go-cmp/cmp"
	healthz "github.com/klyve/go-healthz"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

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

func main() {
	var logLevel string
	var bind string
	var mqttUrlOpt string
	var controllerUrlOpt string
	var haDiscovery bool

	flag.StringVar(&logLevel, "log-level", lookupEnvOrString("BOILER_MATE_LOG_LEVEL", "INFO"), "logging level")
	flag.StringVar(&bind, "bind", lookupEnvOrString("BOILER_MATE_BIND", "0.0.0.0:2112"), "address to bind for healthz and prometheus metrics endpoints (default 0.0.0.0:2112), or \"false\" to disable")
	flag.StringVar(&controllerUrlOpt, "controller", lookupEnvOrString("BOILER_MATE_CONTROLLER", "tcp://00000:0123456789@192.168.1.100:8483"), "controller URI, in the format tcp://<serial>:<password>@<host>:<port>")
	flag.StringVar(&mqttUrlOpt, "mqtt", lookupEnvOrString("BOILER_MATE_MQTT", "tcp://localhost:1883"), "MQTT URI, in the format tcp://[<user>:<password>]@<host>:<port>[/<prefix>]")
	flag.BoolVar(&haDiscovery, "homeassistant", lookupEnvOrBool("BOILER_MATE_HOMEASSISTANT", true), "enable Home Assistant autodiscovery (default: true)")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{})
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		ll = log.InfoLevel
	}
	log.SetLevel(ll)

	if bind != "false" {
		go func(listenAddress string) {
			log.Infof("Starting metrics server on %s", listenAddress)
			instance := healthz.Instance{
				Logger:   log.New(),
				Detailed: true,
			}

			http.Handle("/metrics", promhttp.Handler())
			http.Handle("/healthz", instance.Healthz())
			http.Handle("/liveness", instance.Liveness())

			http.ListenAndServe(bind, nil)
		}(bind)
	}

	uri, err := url.Parse(controllerUrlOpt)
	if err != nil {
		panic(err)
	}
	boiler, err := nbe.NewNBE(uri)
	if err != nil {
		panic(err)
	}

	doneChan := make(chan error, 1)
	log.Infof("Connected to boiler at %s (serial: %s)", uri.Host, boiler.Serial)

	mqttUrl, err := url.Parse(mqttUrlOpt)
	if err != nil {
		log.Fatalf("Invalid MQTT URL: %s", mqttUrlOpt)
		os.Exit(1)
	}

	var mqttPrefix string
	if len(mqttUrl.Path) > 1 {
		mqttPrefix = mqttUrl.Path[1:]
	} else {
		mqttPrefix = fmt.Sprintf("nbe/%s", boiler.Serial)
	}

	mqttClient, err := mqtt.NewClient(mqttUrl, fmt.Sprintf("nbemqtt-%s", boiler.Serial), mqttPrefix)

	if err != nil {
		log.Errorf("Failed to create MQTT client: %s", err)
		os.Exit(1)
	}

	log.Infof("Connected to MQTT broker %s (publishing on \"%s\")", mqttUrl.Host, mqttPrefix)

	mqttClient.Subscribe("set/+/+", 1, func(client *mqtt.Client, msg mqtt.Message) {
		topicParts := strings.Split(msg.Topic(), "/")
		key := fmt.Sprintf("%s.%s", topicParts[len(topicParts)-2], topicParts[len(topicParts)-1])
		value := msg.Payload()

		if key == "device.power_switch" {
			valueStr := string(value[:])
			if valueStr == "ON" || valueStr == "1" {
				key = "misc.start"
				value = []byte("1")
			} else {
				key = "misc.stop"
				value = []byte("1")
			}
		}

		boiler.SetAsync(key, value, func(response *nbe.NBEResponse) {
			log.Infof("Set %s to %s: %v", key, value, response)
		})
	})

	go mqttClient.PublishMany("device", map[string]interface{}{
		"status":     "online",
		"serial":     boiler.Serial,
		"ip_address": boiler.IPAddress,
	})

	settings := make(map[string]interface{})
	settingsGauges := make(map[string]interface{})

	for _, category := range nbe.Settings {
		categoryCache := make(map[string]interface{})
		categoryGauges := make(map[string]*prometheus.GaugeVec)
		settings[category] = &categoryCache
		settingsGauges[category] = &categoryGauges

		go func(prefix string, cache *map[string]interface{}, gauges *map[string]*prometheus.GaugeVec) {
			for {
				boiler.GetAsync(nbe.GetSetupFunction, fmt.Sprintf("%s.*", prefix), func(response *nbe.NBEResponse) {
					changeSet := make(map[string]interface{})
					for k, m := range response.Payload {
						dataType := reflect.TypeOf(m).Kind()
						if (*gauges)[k] == nil && (dataType == reflect.Float64 || dataType == reflect.Int64) {
							(*gauges)[k] = prometheus.NewGaugeVec(
								prometheus.GaugeOpts{
									Namespace: "boiler_mate",
									Subsystem: prefix,
									Name:      k,
								},
								[]string{"serial"},
							)
							prometheus.Register((*gauges)[k])
						}
						if !cmp.Equal((*cache)[k], m) {
							changeSet[k] = m
							(*cache)[k] = m
							switch t := m.(type) {
							case nbe.RoundedFloat:
								(*gauges)[k].WithLabelValues(boiler.Serial).Set(float64(t))
							case int64:
								(*gauges)[k].WithLabelValues(boiler.Serial).Set(float64(t))
							}
						}
					}
					mqttClient.PublishMany(prefix, changeSet)
				})
				time.Sleep(10 * time.Second)
			}
		}(category, &categoryCache, &categoryGauges)
	}

	operatingData := make(map[string]interface{})
	operatingGauges := make(map[string]*prometheus.GaugeVec)
	go func(cache *map[string]interface{}, gauges *map[string]*prometheus.GaugeVec) {
		for {
			boiler.GetAsync(nbe.GetOperatingDataFunction, "*", func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for k, m := range response.Payload {
					dataType := reflect.TypeOf(m).Kind()
					if (*gauges)[k] == nil && (dataType == reflect.Float64 || dataType == reflect.Int64) {
						(*gauges)[k] = prometheus.NewGaugeVec(
							prometheus.GaugeOpts{
								Namespace: "boiler_mate",
								Subsystem: "operating_data",
								Name:      k,
							},
							[]string{"serial"},
						)
						prometheus.MustRegister((*gauges)[k])
					}

					if !cmp.Equal((*cache)[k], m) {
						changeSet[k] = m
						(*cache)[k] = m
						switch t := m.(type) {
						case nbe.RoundedFloat:
							(*gauges)[k].WithLabelValues(boiler.Serial).Set(float64(t))
						case int64:
							(*gauges)[k].WithLabelValues(boiler.Serial).Set(float64(t))
						}

						if k == "state" {
							curState, ok := m.(int64)
							if ok {
								changeSet["state_text"] = nbe.PowerStates[curState]
								stateOn := "OFF"
								if curState != 14 {
									stateOn = "ON"
								}
								changeSet["state_on"] = stateOn
							}
						}
					}
				}

				go mqttClient.PublishMany("operating_data", changeSet)
			})

			time.Sleep(5 * time.Second)
		}
	}(&operatingData, &operatingGauges)

	advancedData := make(map[string]interface{})
	advancedGauges := make(map[string]*prometheus.GaugeVec)
	go func(cache *map[string]interface{}, gauges *map[string]*prometheus.GaugeVec) {
		for {
			boiler.GetAsync(nbe.GetAdvancedDataFunction, "*", func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for k, m := range response.Payload {
					dataType := reflect.TypeOf(m).Kind()
					if (*gauges)[k] == nil && (dataType == reflect.Float64 || dataType == reflect.Int64) {
						(*gauges)[k] = prometheus.NewGaugeVec(
							prometheus.GaugeOpts{
								Namespace: "boiler_mate",
								Subsystem: "operating_data",
								Name:      k,
							},
							[]string{"serial"},
						)
						prometheus.MustRegister((*gauges)[k])
					}

					if !cmp.Equal((*cache)[k], m) {
						changeSet[k] = m
						(*cache)[k] = m
						switch t := m.(type) {
						case nbe.RoundedFloat:
							(*gauges)[k].WithLabelValues(boiler.Serial).Set(float64(t))
						case int64:
							(*gauges)[k].WithLabelValues(boiler.Serial).Set(float64(t))
						}
					}
				}
				go mqttClient.PublishMany("advanced_data", changeSet)
			})
			time.Sleep(5 * time.Second)
		}
	}(&advancedData, &advancedGauges)

	if haDiscovery {
		log.Infof("Publishing Home Assistant discovery messages for %s", boiler.Serial)

		devBlock := map[string]interface{}{
			"ids":  []string{fmt.Sprintf("nbe_%s", boiler.Serial)},
			"name": fmt.Sprintf("NBE Boiler (%s)", boiler.Serial),
			"sw":   "boiler-mate",
			"mf":   "NBE",
			"sa":   "",
		}

		go func(prefix string) {
			time.Sleep(5 * time.Second)

			sensors := make(map[string]interface{})
			sensors["ip_address"] = map[string]interface{}{
				"name":            "IP Address",
				"entity_category": "diagnostic",
				"stat_t":          fmt.Sprintf("%s/device/ip_address", prefix),
				"avty_t":          fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":         fmt.Sprintf("nbe_%s_ip_address", boiler.Serial),
				"dev":             devBlock,
			}
			sensors["serial"] = map[string]interface{}{
				"name":            "Serial",
				"entity_category": "diagnostic",
				"stat_t":          fmt.Sprintf("%s/device/serial", prefix),
				"avty_t":          fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":         fmt.Sprintf("nbe_%s_serial", boiler.Serial),
				"dev":             devBlock,
			}
			sensors["boiler_temp"] = map[string]interface{}{
				"name":                          "Boiler Temperature",
				"entity_category":               "diagnostic",
				"device_class":                  "temperature",
				"native_unit_of_measurement":    "°C",
				"suggested_unit_of_measurement": "°C",
				"suggested_display_precision":   2,
				"stat_t":                        fmt.Sprintf("%s/operating_data/boiler_temp", prefix),
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_boiler_temp", boiler.Serial),
				"dev":                           devBlock,
			}
			sensors["oxygen"] = map[string]interface{}{
				"name":                        "Oxygen",
				"entity_category":             "diagnostic",
				"unit_of_measurement":         "%",
				"ic":                          "mdi:air-filter",
				"suggested_display_precision": 2,
				"stat_t":                      fmt.Sprintf("%s/operating_data/oxygen", prefix),
				"avty_t":                      fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                     fmt.Sprintf("nbe_%s_oxygen", boiler.Serial),
				"dev":                         devBlock,
			}
			sensors["status"] = map[string]interface{}{
				"name":            "Status",
				"entity_category": "diagnostic",
				"ic":              "mdi:power",
				"stat_t":          fmt.Sprintf("%s/operating_data/state_text", prefix),
				"avty_t":          fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":         fmt.Sprintf("nbe_%s_status", boiler.Serial),
				"dev":             devBlock,
			}
			sensors["smoke_temp"] = map[string]interface{}{
				"name":                          "Smoke Temperature",
				"entity_category":               "diagnostic",
				"device_class":                  "temperature",
				"native_unit_of_measurement":    "°C",
				"suggested_unit_of_measurement": "°C",
				"suggested_display_precision":   2,
				"stat_t":                        fmt.Sprintf("%s/operating_data/smoke_temp", prefix),
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_smoke_temp", boiler.Serial),
				"dev":                           devBlock,
			}
			sensors["photo_level"] = map[string]interface{}{
				"name":                        "Photo Level",
				"entity_category":             "diagnostic",
				"unit_of_measurement":         "%",
				"ic":                          "mdi:lightbulb",
				"suggested_display_precision": 2,
				"stat_t":                      fmt.Sprintf("%s/operating_data/photo_level", prefix),
				"avty_t":                      fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                     fmt.Sprintf("nbe_%s_photo_level", boiler.Serial),
				"dev":                         devBlock,
			}
			sensors["power_kw"] = map[string]interface{}{
				"name":                          "Power (kW)",
				"entity_category":               "diagnostic",
				"device_class":                  "power",
				"native_unit_of_measurement":    "kW",
				"suggested_unit_of_measurement": "kW",
				"suggested_display_precision":   2,
				"stat_t":                        fmt.Sprintf("%s/operating_data/power_kw", prefix),
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_power_kw", boiler.Serial),
				"dev":                           devBlock,
			}
			sensors["power_pct"] = map[string]interface{}{
				"name":                        "Power (%)",
				"entity_category":             "diagnostic",
				"device_class":                "power",
				"unit_of_measurement":         "%",
				"suggested_display_precision": 2,
				"stat_t":                      fmt.Sprintf("%s/operating_data/power_pct", prefix),
				"avty_t":                      fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                     fmt.Sprintf("nbe_%s_power_pct", boiler.Serial),
				"dev":                         devBlock,
			}

			for k, m := range sensors {
				err := mqttClient.PublishJSON(fmt.Sprintf("homeassistant/sensor/nbe_%s/%s/config", boiler.Serial, k), m)
				if err != nil {
					log.Errorf("Error publishing discovery message for %s: %v", k, err)
				}
			}

			numbers := make(map[string]interface{})
			numbers["boiler_setpoint"] = map[string]interface{}{
				"name":                          "Wanted Temperature",
				"entity_category":               "config",
				"device_class":                  "temperature",
				"native_unit_of_measurement":    "°C",
				"suggested_unit_of_measurement": "°C",
				"mode":                          "box",
				"native_min_value":              0,
				"native_max_value":              85,
				"suggested_display_precision":   1,
				"stat_t":                        fmt.Sprintf("%s/boiler/temp", prefix),
				"cmd_t":                         fmt.Sprintf("%s/set/boiler/temp", prefix),
				"step":                          "1",
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_boiler_setpoint", boiler.Serial),
				"dev":                           devBlock,
			}
			numbers["boiler_power_min"] = map[string]interface{}{
				"name":                        "Minimum Power (%)",
				"entity_category":             "config",
				"unit_of_measurement":         "%",
				"mode":                        "box",
				"native_min_value":            10,
				"native_max_value":            100,
				"suggested_display_precision": 0,
				"stat_t":                      fmt.Sprintf("%s/regulation/boiler_power_min", prefix),
				"cmd_t":                       fmt.Sprintf("%s/set/regulation/boiler_power_min", prefix),
				"step":                        "1",
				"avty_t":                      fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                     fmt.Sprintf("nbe_%s_boiler_power_min", boiler.Serial),
				"dev":                         devBlock,
			}
			numbers["boiler_power_max"] = map[string]interface{}{
				"name":                        "Maximum Power (%)",
				"entity_category":             "config",
				"unit_of_measurement":         "%",
				"mode":                        "box",
				"native_min_value":            10,
				"native_max_value":            100,
				"suggested_display_precision": 0,
				"stat_t":                      fmt.Sprintf("%s/regulation/boiler_power_max", prefix),
				"cmd_t":                       fmt.Sprintf("%s/set/regulation/boiler_power_max", prefix),
				"step":                        "1",
				"avty_t":                      fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                     fmt.Sprintf("nbe_%s_boiler_power_max", boiler.Serial),
				"dev":                         devBlock,
			}
			numbers["diff_under"] = map[string]interface{}{
				"name":                          "Difference Under",
				"entity_category":               "config",
				"device_class":                  "temperature",
				"native_unit_of_measurement":    "°C",
				"suggested_unit_of_measurement": "°C",
				"mode":                          "box",
				"ic":                            "mdi:arrow-collapse-down",
				"native_min_value":              0,
				"native_max_value":              50,
				"suggested_display_precision":   1,
				"stat_t":                        fmt.Sprintf("%s/boiler/diff_under", prefix),
				"cmd_t":                         fmt.Sprintf("%s/set/boiler/diff_under", prefix),
				"step":                          "1",
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_diff_under", boiler.Serial),
				"dev":                           devBlock,
			}
			numbers["diff_over"] = map[string]interface{}{
				"name":                          "Difference Over",
				"entity_category":               "config",
				"device_class":                  "temperature",
				"native_unit_of_measurement":    "°C",
				"suggested_unit_of_measurement": "°C",
				"mode":                          "box",
				"ic":                            "mdi:arrow-collapse-up",
				"native_min_value":              10,
				"native_max_value":              20,
				"suggested_display_precision":   1,
				"stat_t":                        fmt.Sprintf("%s/boiler/diff_over", prefix),
				"cmd_t":                         fmt.Sprintf("%s/set/boiler/diff_over", prefix),
				"step":                          "1",
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_diff_over", boiler.Serial),
				"dev":                           devBlock,
			}
			numbers["hopper_content"] = map[string]interface{}{
				"name":                          "Hopper",
				"entity_category":               "config",
				"device_class":                  "weight",
				"native_unit_of_measurement":    "kg",
				"suggested_unit_of_measurement": "kg",
				"mode":                          "box",
				"ic":                            "mdi:storage-tank",
				"min":                           0,
				"max":                           999,
				"suggested_display_precision":   1,
				"stat_t":                        fmt.Sprintf("%s/hopper/content", prefix),
				"cmd_t":                         fmt.Sprintf("%s/set/hopper/content", prefix),
				"step":                          "1",
				"avty_t":                        fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":                       fmt.Sprintf("nbe_%s_hopper_content", boiler.Serial),
				"dev":                           devBlock,
			}

			for k, m := range numbers {
				err := mqttClient.PublishJSON(fmt.Sprintf("homeassistant/number/nbe_%s/%s/config", boiler.Serial, k), m)
				if err != nil {
					log.Errorf("Error publishing discovery message for %s: %v", k, err)
				}
			}

			buttons := make(map[string]interface{})
			buttons["start_calibrate"] = map[string]interface{}{
				"name":            "Start O2 Sensor Calibration",
				"entity_category": "config",
				"ic":              "mdi:air-filter",
				"stat_t":          fmt.Sprintf("%s/oxygen/start_calibrate", prefix),
				"cmd_t":           fmt.Sprintf("%s/set/oxygen/start_calibrate", prefix),
				"avty_t":          fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":         fmt.Sprintf("nbe_%s_start_calibrate", boiler.Serial),
				"payload_press":   "1",
				"dev":             devBlock,
			}

			for k, m := range buttons {
				err := mqttClient.PublishJSON(fmt.Sprintf("homeassistant/button/nbe_%s/%s/config", boiler.Serial, k), m)
				if err != nil {
					log.Errorf("Error publishing discovery message for %s: %v", k, err)
				}
			}

			switches := make(map[string]interface{})
			switches["power"] = map[string]interface{}{
				"name":            "Power",
				"entity_category": "config",
				"ic":              "mdi:power",
				"state_topic":     fmt.Sprintf("%s/operating_data/state_on", prefix),
				"cmd_t":           fmt.Sprintf("%s/set/device/power_switch", prefix),
				"avty_t":          fmt.Sprintf("%s/device/status", prefix),
				"uniq_id":         fmt.Sprintf("nbe_%s_power", boiler.Serial),
				"dev":             devBlock,
			}

			for k, m := range switches {
				err := mqttClient.PublishJSON(fmt.Sprintf("homeassistant/switch/nbe_%s/%s/config", boiler.Serial, k), m)
				if err != nil {
					log.Errorf("Error publishing discovery message for %s: %v", k, err)
				}
			}

			time.Sleep(2 * time.Minute)
		}(mqttPrefix)
	}

	err = <-doneChan

	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

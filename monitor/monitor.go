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

package monitor

import (
	"fmt"
	"reflect"
	"time"

	cmp "github.com/google/go-cmp/cmp"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus"
)

// StartSettingsMonitor polls settings data and publishes changes
// If ready channel is provided, it will be signaled when first data is published
func StartSettingsMonitor(boiler *nbe.NBE, mqttClient *mqtt.Client, category string) chan bool {
	return StartSettingsMonitorWithReady(boiler, mqttClient, category, true)
}

// StartSettingsMonitorWithReady polls settings data with optional ready notification
func StartSettingsMonitorWithReady(boiler *nbe.NBE, mqttClient *mqtt.Client, category string, notifyReady bool) chan bool {
	cache := make(map[string]interface{})
	gauges := make(map[string]*prometheus.GaugeVec)
	var ready chan bool
	if notifyReady {
		ready = make(chan bool, 1)
	}

	firstPublish := true

	go func() {
		for {
			boiler.GetAsync(nbe.GetSetupFunction, fmt.Sprintf("%s.*", category), func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for key, value := range response.Payload {
					// Register prometheus gauge if numeric and not exists
					if gauges[key] == nil && isNumeric(value) {
						gauges[key] = prometheus.NewGaugeVec(
							prometheus.GaugeOpts{
								Namespace: "boiler_mate",
								Subsystem: category,
								Name:      key,
							},
							[]string{"serial"},
						)
						prometheus.Register(gauges[key])
					}

					// Publish if changed
					if !cmp.Equal(cache[key], value) {
						changeSet[key] = value
						cache[key] = value
						updateGauge(gauges[key], boiler.Serial, value)
					}
				}
				mqttClient.PublishMany(category, changeSet)

				// Signal ready after first successful publish
				if firstPublish && ready != nil {
					select {
					case ready <- true:
					default:
					}
					firstPublish = false
				}
			})
			time.Sleep(10 * time.Second)
		}
	}()

	return ready
}

// StartOperatingDataMonitor polls operating data and publishes changes
// Returns a channel that signals when first data is published
func StartOperatingDataMonitor(boiler *nbe.NBE, mqttClient *mqtt.Client) chan bool {
	cache := make(map[string]interface{})
	gauges := make(map[string]*prometheus.GaugeVec)
	ready := make(chan bool, 1)
	firstPublish := true

	go func() {
		for {
			boiler.GetAsync(nbe.GetOperatingDataFunction, "*", func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for key, value := range response.Payload {
					// Register prometheus gauge if numeric and not exists
					if gauges[key] == nil && isNumeric(value) {
						gauges[key] = prometheus.NewGaugeVec(
							prometheus.GaugeOpts{
								Namespace: "boiler_mate",
								Subsystem: "operating_data",
								Name:      key,
							},
							[]string{"serial"},
						)
						prometheus.MustRegister(gauges[key])
					}

					// Publish if changed
					if !cmp.Equal(cache[key], value) {
						changeSet[key] = value
						cache[key] = value
						updateGauge(gauges[key], boiler.Serial, value)

						// Add state_text and state_on for state field
						if key == "state" {
							if curState, ok := value.(int64); ok {
								changeSet["state_text"] = nbe.PowerStates[curState]
								if curState != 14 {
									changeSet["state_on"] = "ON"
								} else {
									changeSet["state_on"] = "OFF"
								}
							}
						}
					}
				}
				go mqttClient.PublishMany("operating_data", changeSet)

				// Signal ready after first successful publish
				if firstPublish {
					select {
					case ready <- true:
					default:
					}
					firstPublish = false
				}
			})
			time.Sleep(5 * time.Second)
		}
	}()

	return ready
}

// StartAdvancedDataMonitor polls advanced data and publishes changes
func StartAdvancedDataMonitor(boiler *nbe.NBE, mqttClient *mqtt.Client) {
	cache := make(map[string]interface{})
	gauges := make(map[string]*prometheus.GaugeVec)

	go func() {
		for {
			boiler.GetAsync(nbe.GetAdvancedDataFunction, "*", func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for key, value := range response.Payload {
					// Register prometheus gauge if numeric and not exists
					if gauges[key] == nil && isNumeric(value) {
						gauges[key] = prometheus.NewGaugeVec(
							prometheus.GaugeOpts{
								Namespace: "boiler_mate",
								Subsystem: "operating_data",
								Name:      key,
							},
							[]string{"serial"},
						)
						prometheus.MustRegister(gauges[key])
					}

					// Publish if changed
					if !cmp.Equal(cache[key], value) {
						changeSet[key] = value
						cache[key] = value
						updateGauge(gauges[key], boiler.Serial, value)
					}
				}
				go mqttClient.PublishMany("advanced_data", changeSet)
			})
			time.Sleep(5 * time.Second)
		}
	}()
}

func isNumeric(value interface{}) bool {
	if value == nil {
		return false
	}
	dataType := reflect.TypeOf(value).Kind()
	return dataType == reflect.Float64 || dataType == reflect.Int64
}

func updateGauge(gauge *prometheus.GaugeVec, serial string, value interface{}) {
	if gauge == nil {
		return
	}
	switch v := value.(type) {
	case nbe.RoundedFloat:
		gauge.WithLabelValues(serial).Set(float64(v))
	case int64:
		gauge.WithLabelValues(serial).Set(float64(v))
	}
}

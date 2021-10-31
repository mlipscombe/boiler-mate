/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021 Mark Lipscombe.
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

func main() {
	var logLevel string
	var bind string
	var mqttUrlOpt string
	var controllerUrlOpt string

	flag.StringVar(&logLevel, "log-level", lookupEnvOrString("BOILER_MATE_LOG_LEVEL", "INFO"), "logging level")
	flag.StringVar(&bind, "bind", lookupEnvOrString("BOILER_MATE_BIND", "localhost:2112"), "address to bind for healthz and prometheus metrics endpoints (default localhost:2112), or \"false\" to disable")
	flag.StringVar(&controllerUrlOpt, "controller", lookupEnvOrString("BOILER_MATE_CONTROLLER", "tcp://00000:0123456789@192.168.1.100:8483"), "controller URI, in the format tcp://<serial>:<password>@<host>:<port>")
	flag.StringVar(&mqttUrlOpt, "mqtt", lookupEnvOrString("BOILER_MATE_MQTT", "tcp://localhost:1883"), "MQTT URI, in the format tcp://[<user>:<password>]@<host>:<port>[/<prefix>]")

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

		boiler.SetAsync(key, value, func(response *nbe.NBEResponse) {
			log.Infof("Set %s to %s: %v", key, value, response)
		})
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

	err = <-doneChan
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}

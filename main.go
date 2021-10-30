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
	"net/url"
	"os"
	"strings"
	"time"

	cmp "github.com/google/go-cmp/cmp"
	"github.com/mlipscombe/boiler-mate/mqtt"
	"github.com/mlipscombe/boiler-mate/nbe"
	log "github.com/sirupsen/logrus"
)

func lookupEnvOrString(key string, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func main() {
	var debugMode bool
	var mqttUrlOpt string
	var controllerUrlOpt string

	flag.BoolVar(&debugMode, "debug", false, "debug mode")
	flag.StringVar(&controllerUrlOpt, "controller", lookupEnvOrString("NBEMQTT_CONTROLLER", "tcp://00000:0123456789@192.168.1.100:8483"), "controller URI, in the format tcp://<serial>:<password>@<host>:<port>")
	flag.StringVar(&mqttUrlOpt, "mqtt", lookupEnvOrString("NBEMQTT_MQTT", "tcp://localhost:1883"), "MQTT URI, in the format tcp://[<user>:<password>]@<host>:<port>[/<prefix>]")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{})
	if debugMode {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
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
	log.Infof("Connected to boiler at %s (serial: %s)\n", uri.Host, boiler.Serial)

	mqttUrl, err := url.Parse(mqttUrlOpt)
	if err != nil {
		log.Errorf("Invalid MQTT URL: %s\n", mqttUrlOpt)
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
		log.Errorf("Failed to create MQTT client: %s\n", err)
		os.Exit(1)
	}

	log.Infof("Connected to MQTT broker %s (publishing on \"%s\")\n", mqttUrl.Host, mqttPrefix)

	mqttClient.Subscribe("set/+/+", 1, func(client *mqtt.Client, msg mqtt.Message) {
		topicParts := strings.Split(msg.Topic(), "/")
		key := fmt.Sprintf("%s.%s", topicParts[len(topicParts)-2], topicParts[len(topicParts)-1])
		value := msg.Payload()

		boiler.SetAsync(key, value, func(response *nbe.NBEResponse) {
			log.Infof("Set %s to %s: %v\n", key, value, response)
		})
	})

	settings := make(map[string]interface{})

	for _, category := range nbe.Settings {
		categoryCache := make(map[string]interface{})
		settings[category] = &categoryCache

		go func(prefix string, cache *map[string]interface{}) {
			for {
				boiler.GetAsync(nbe.GetSetupFunction, fmt.Sprintf("%s.*", prefix), func(response *nbe.NBEResponse) {
					changeSet := make(map[string]interface{})
					for k, m := range response.Payload {
						if !cmp.Equal((*cache)[k], m) {
							changeSet[k] = m
							(*cache)[k] = m
						}
					}
					mqttClient.PublishMany(prefix, changeSet)
				})
				time.Sleep(10 * time.Second)
			}
		}(category, &categoryCache)
	}

	operatingData := make(map[string]interface{})
	go func(cache *map[string]interface{}) {
		for {
			boiler.GetAsync(nbe.GetOperatingDataFunction, "*", func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for k, m := range response.Payload {
					if !cmp.Equal((*cache)[k], m) {
						changeSet[k] = m
						(*cache)[k] = m
					}
				}
				go mqttClient.PublishMany("operating_data", changeSet)
			})
			time.Sleep(5 * time.Second)
		}
	}(&operatingData)

	advancedData := make(map[string]interface{})
	go func(cache *map[string]interface{}) {
		for {
			boiler.GetAsync(nbe.GetAdvancedDataFunction, "*", func(response *nbe.NBEResponse) {
				changeSet := make(map[string]interface{})
				for k, m := range response.Payload {
					if !cmp.Equal((*cache)[k], m) {
						changeSet[k] = m
						(*cache)[k] = m
					}
				}
				go mqttClient.PublishMany("advanced_data", changeSet)
			})
			time.Sleep(5 * time.Second)
		}
	}(&advancedData)

	err = <-doneChan
	if err != nil {
		log.Fatal(err)
	}
}

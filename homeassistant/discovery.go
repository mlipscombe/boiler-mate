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

package homeassistant

import (
	"fmt"

	"github.com/mlipscombe/boiler-mate/mqtt"
	log "github.com/sirupsen/logrus"
)

// PublishDiscovery sends Home Assistant MQTT discovery messages
// Waits for data to be ready before publishing
func PublishDiscovery(mqttClient *mqtt.Client, serial, prefix string, ready <-chan bool) {
	log.Infof("Publishing Home Assistant discovery messages for %s", serial)

	// Wait for initial data to be ready
	if ready != nil {
		log.Debug("Waiting for initial data before publishing discovery messages...")
		<-ready
		log.Debug("Initial data ready, publishing discovery messages")
	}

	devBlock := createDeviceBlock(serial)

	// Publish all entities
	publishEntities(mqttClient, serial, prefix, devBlock)
}

func createDeviceBlock(serial string) map[string]interface{} {
	return map[string]interface{}{
		"ids":  []string{fmt.Sprintf("nbe_%s", serial)},
		"name": fmt.Sprintf("NBE Boiler (%s)", serial),
		"sw":   "boiler-mate",
		"mf":   "NBE",
		"sa":   "",
	}
}

func publishEntities(mqttClient *mqtt.Client, serial, prefix string, devBlock map[string]interface{}) {
	entities := AllEntities()

	for _, entity := range entities {
		config := entity.Build(serial, prefix, devBlock)
		topic := entity.GetDiscoveryTopic(serial)

		if err := mqttClient.PublishJSON(topic, config); err != nil {
			log.Errorf("Error publishing discovery message for %s (%s): %v", entity.Name, entity.Key, err)
		} else {
			log.Debugf("Published discovery for %s at %s", entity.Name, topic)
		}
	}

	log.Infof("Published %d entity discovery messages", len(entities))
}

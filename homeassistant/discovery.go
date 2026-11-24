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
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mlipscombe/boiler-mate/nbe"
)

type discoveryPublisher interface {
	PublishJSON(topic string, value any) error
}

// PublishDiscovery queues retained discovery. Temperature controls require
// an editable range from the controller.
func PublishDiscovery(publisher discoveryPublisher, serial, prefix string, boilerRanges map[string]any) error {
	device := createDeviceBlock(serial)
	var errs []error
	for _, entity := range AllEntities() {
		if entity.EntityType == Number && entity.Unit == "°C" {
			key := strings.TrimPrefix(entity.StateTopic, "boiler/")
			bounds := nbe.ReadSettingRange(boilerRanges, key)
			// MQTT Number accepts a step of at least 0.001.
			if bounds == nil || bounds.Min >= bounds.Max || bounds.Decimals > 3 {
				continue
			}
			entity.MinValue = bounds.Min
			entity.MaxValue = bounds.Max
			entity.Step = math.Pow10(-bounds.Decimals)
		}
		topic := entity.GetDiscoveryTopic(serial)
		if err := publisher.PublishJSON(topic, entity.Build(serial, prefix, device)); err != nil {
			errs = append(errs, fmt.Errorf("publish discovery for %s: %w", entity.Key, err))
		}
	}
	return errors.Join(errs...)
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

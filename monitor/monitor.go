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
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"sync"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
	"github.com/prometheus/client_golang/prometheus"
)

type reader interface {
	Get(context.Context, nbe.Function, string) (*nbe.NBEResponse, error)
}

type publisher interface {
	PublishMany(topic string, values map[string]any) error
}

type poll struct {
	function nbe.Function
	path     string
	topic    string
	interval time.Duration
	offset   time.Duration
}

// Observer receives successful snapshots, including values that have not changed.
type Observer func(topic string, values map[string]any)

// Run polls the controller until ctx is cancelled and waits for all pollers to stop.
func Run(ctx context.Context, logger *slog.Logger, boiler reader, mqtt publisher, registry prometheus.Registerer, serial string, observe Observer, ready func()) {
	const telemetryInterval = 5 * time.Second
	const settingsInterval = 2 * telemetryInterval
	polls := []poll{
		{nbe.GetOperatingDataFunction, "*", "operating_data", telemetryInterval, 0},
		{nbe.GetAdvancedDataFunction, "*", "advanced_data", telemetryInterval, telemetryInterval / 2},
	}
	// Each settings cycle has four gaps between telemetry reads. Spread the
	// settings requests through those gaps so they don't arrive in one UDP burst.
	settingsPerGap := (len(nbe.Settings) + 3) / 4
	spacing := telemetryInterval / 2 / time.Duration(settingsPerGap+1)
	for i, category := range nbe.Settings {
		offset := time.Duration(i%4)*(telemetryInterval/2) + time.Duration(i/4+1)*spacing
		polls = append(polls, poll{nbe.GetSetupFunction, category + ".*", category, settingsInterval, offset})
	}

	gauges := &metricSet{registry: registry, serial: serial, gauges: make(map[string]metric)}
	var workers sync.WaitGroup
	for _, task := range polls {
		workers.Go(func() {
			runPoll(ctx, logger, boiler, mqtt, gauges, task, observe, ready)
		})
	}
	workers.Wait()
}

func runPoll(ctx context.Context, logger *slog.Logger, boiler reader, mqtt publisher, gauges *metricSet, task poll, observe Observer, ready func()) {
	if task.offset > 0 {
		select {
		case <-time.After(task.offset):
		case <-ctx.Done():
			return
		}
	}
	cache := make(map[string]any)
	ticker := time.NewTicker(task.interval)
	defer ticker.Stop()
	operating := task.function == nbe.GetOperatingDataFunction
	var lastOperatingData time.Time
	var controllerStatus string
	publishStatus := func(status string) {
		if !operating || status == controllerStatus {
			return
		}
		if err := mqtt.PublishMany("device", map[string]any{"controller_status": status}); err != nil {
			logger.Warn("Could not queue controller availability", "error", err)
			return
		}
		controllerStatus = status
	}
	for ctx.Err() == nil {
		response, err := boiler.Get(ctx, task.function, task.path)
		if ctx.Err() != nil {
			return
		}
		if err != nil || len(response.Payload) == 0 {
			if err != nil {
				logger.Debug("Controller poll failed", "topic", task.topic, "error", err)
			}
			if operating && (lastOperatingData.IsZero() || time.Since(lastOperatingData) >= 20*time.Second) {
				publishStatus("offline")
			}
		} else {
			values := response.Payload
			if operating {
				lastOperatingData = time.Now()
				values = withOperatingState(values)
			}
			if observe != nil {
				observe(task.topic, values)
			}
			updates := make(map[string]any)
			for key, value := range values {
				gauges.set(logger, task.topic, key, value)
				if previous, exists := cache[key]; !exists || !equalValue(previous, value) {
					updates[key] = value
				}
			}
			if err := mqtt.PublishMany(task.topic, updates); err != nil {
				logger.Warn("Could not queue MQTT update", "topic", task.topic, "error", err)
			} else {
				maps.Copy(cache, updates)
				publishStatus("online")
				if operating && ready != nil {
					ready()
					ready = nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func equalValue(left, right any) bool {
	if left, ok := left.(nbe.RoundedFloat); ok {
		right, ok := right.(nbe.RoundedFloat)
		return ok && left.Equal(right)
	}
	return reflect.DeepEqual(left, right)
}

func withOperatingState(values map[string]any) map[string]any {
	if len(values) == 0 {
		return values
	}
	state, ok := values["state"].(int64)
	values = maps.Clone(values)
	values["state_on"] = "None"
	if !ok {
		if text, _ := values["state_text"].(string); text == "" {
			values["state_text"] = "Unknown"
		}
		return values
	}
	message := fmt.Sprintf("Unknown (%d)", state)
	if state >= 0 && state < int64(len(nbe.PowerStates)) && nbe.PowerStates[state] != "" {
		message = nbe.PowerStates[state]
	}
	values["state_text"] = message
	// The local protocol reports a phase. Firing and automatic standby
	// establish that the boiler is enabled, while alarms and transitions do not.
	switch state {
	case nbe.PowerStateOff:
		values["state_on"] = "OFF"
	case 1, 2, 3, 4, 5, 6, 7, 9, 10, 22, 23, 24, 25, 30:
		values["state_on"] = "ON"
	}
	return values
}

type metricSet struct {
	mu       sync.Mutex
	registry prometheus.Registerer
	serial   string
	gauges   map[string]metric
}

type metric struct {
	gauge  *prometheus.GaugeVec
	source string
}

func (metrics *metricSet) set(logger *slog.Logger, source, key string, value any) {
	var number float64
	switch value := value.(type) {
	case int64:
		number = float64(value)
	case nbe.RoundedFloat:
		number = float64(value)
	default:
		return
	}

	// Advanced data has historically used the operating_data metric namespace.
	subsystem := source
	if subsystem == "advanced_data" {
		subsystem = "operating_data"
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	name := subsystem + "_" + key
	m, ok := metrics.gauges[name]
	if !ok {
		m = metric{
			gauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "boiler_mate", Subsystem: subsystem, Name: key,
			}, []string{"serial"}),
			source: source,
		}
		if err := metrics.registry.Register(m.gauge); err != nil {
			var registered prometheus.AlreadyRegisteredError
			if errors.As(err, &registered) {
				m.gauge, _ = registered.ExistingCollector.(*prometheus.GaugeVec)
			} else {
				logger.Debug("Could not register metric", "metric", name, "error", err)
				m.gauge = nil
			}
		}
	}
	if m.gauge != nil && (m.source == source || source == "operating_data") {
		m.gauge.WithLabelValues(metrics.serial).Set(number)
		m.source = source
	}
	metrics.gauges[name] = m
}

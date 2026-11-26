// Package hass reads selected Home Assistant entities using the REST API.
package hass

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const EntityLimit = 32

var entityPattern = regexp.MustCompile(`^(climate|sensor)\.[a-z0-9][a-z0-9_]*$`)

func ValidEntity(id string) bool {
	return len(id) <= 80 && entityPattern.MatchString(id)
}

type Config struct {
	URL      *url.URL
	Token    string
	Entities []string
}

type Reading struct {
	ID, Name, Action, Unit string
	Available, HasValue    bool
	Value                  float64
	At                     time.Time
}

type state struct {
	ID         string `json:"entity_id"`
	State      string `json:"state"`
	Attributes struct {
		Name        string   `json:"friendly_name"`
		Action      string   `json:"hvac_action"`
		Temperature *float64 `json:"current_temperature"`
		Unit        string   `json:"unit_of_measurement"`
		TempUnit    string   `json:"temperature_unit"`
	} `json:"attributes"`
}

func (s state) reading(defaultTempUnit string, now time.Time) Reading {
	r := Reading{ID: s.ID, Name: s.Attributes.Name, At: now,
		Available: s.State != "" && s.State != "unknown" && s.State != "unavailable"}
	if strings.HasPrefix(s.ID, "climate.") {
		switch action := s.Attributes.Action; action {
		case "heating", "idle", "off", "cooling", "drying", "fan", "preheating", "defrosting":
			if r.Available {
				r.Action = action
			}
		}
		r.Unit = "°C"
		if value := s.Attributes.Temperature; r.Available && value != nil {
			unit := s.Attributes.TempUnit
			if unit == "" {
				unit = defaultTempUnit
			}
			r.Value, r.HasValue = temperature(*value, unit)
		}
		return r
	}
	value, err := strconv.ParseFloat(s.State, 64)
	valid := r.Available && err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
	switch s.Attributes.Unit {
	case "°C", "°F", "K":
		r.Unit = "°C"
		if valid {
			r.Value, r.HasValue = temperature(value, s.Attributes.Unit)
		}
	case "m³/h":
		r.Unit = "m³/h"
		r.Value, r.HasValue = value, valid
	case "L/min":
		r.Unit = "m³/h"
		r.Value, r.HasValue = value*.06, valid
	case "L/h":
		r.Unit = "m³/h"
		r.Value, r.HasValue = value*.001, valid
	case "L/s":
		r.Unit = "m³/h"
		r.Value, r.HasValue = value*3.6, valid && !math.IsInf(value*3.6, 0)
	case "gal/min":
		r.Unit = "m³/h"
		r.Value, r.HasValue = value*.22712470704, valid
	}
	return r
}

func temperature(value float64, unit string) (float64, bool) {
	switch unit {
	case "°C":
	case "°F":
		value = (value - 32) * 5 / 9
	case "K":
		value -= 273.15
	default:
		return 0, false
	}
	return value, !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -273.15
}

func Run(ctx context.Context, cfg Config, logger *slog.Logger, observe func([]Reading)) {
	client := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	var temperatureUnit string
	var hasClimate bool
	for _, id := range cfg.Entities {
		hasClimate = hasClimate || strings.HasPrefix(id, "climate.")
	}
	for ctx.Err() == nil {
		if hasClimate && temperatureUnit == "" {
			var system struct {
				UnitSystem struct {
					Temperature string `json:"temperature"`
				} `json:"unit_system"`
			}
			if err := get(ctx, client, cfg, "/api/config", &system); err == nil {
				temperatureUnit = system.UnitSystem.Temperature
			}
		}
		observe(poll(ctx, client, cfg, logger, temperatureUnit))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func poll(ctx context.Context, client *http.Client, cfg Config, logger *slog.Logger, temperatureUnit string) []Reading {
	readings := make([]Reading, len(cfg.Entities))
	var workers sync.WaitGroup
	slots := make(chan struct{}, 8)
	for i, id := range cfg.Entities {
		slots <- struct{}{}
		workers.Go(func() {
			defer func() { <-slots }()
			readings[i].ID = id
			var s state
			err := get(ctx, client, cfg, "/api/states/"+id, &s)
			if err != nil || s.ID != id {
				if ctx.Err() == nil {
					logger.Debug("Could not read Home Assistant entity", "entity", id, "error", err)
				}
				return
			}
			readings[i] = s.reading(temperatureUnit, time.Now())
		})
	}
	workers.Wait()
	return readings
}

func get(ctx context.Context, client *http.Client, cfg Config, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.URL.String(), "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(result)
}

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mlipscombe/boiler-mate/internal/hass"
)

func homeAssistantConfig() (hass.Config, error) {
	var cfg hass.Config
	address := strings.TrimSpace(os.Getenv("BOILER_MATE_HA_URL"))
	token := strings.TrimSpace(os.Getenv("BOILER_MATE_HA_TOKEN"))
	entities := strings.TrimSpace(os.Getenv("BOILER_MATE_HA_ENTITIES"))
	if address == "" && token == "" && entities == "" {
		return cfg, nil
	}
	if address == "" || token == "" || entities == "" {
		return cfg, errors.New("set BOILER_MATE_HA_URL, BOILER_MATE_HA_TOKEN, and BOILER_MATE_HA_ENTITIES to enable Home Assistant inputs")
	}
	uri, err := parseURL(address, "Home Assistant", "http", "https")
	if err != nil {
		return cfg, err
	}
	if uri.User != nil || uri.RawQuery != "" || uri.ForceQuery || strings.ContainsAny(token, " \t\r\n") {
		return cfg, errors.New("invalid Home Assistant URL or token")
	}
	seen := make(map[string]bool)
	for _, id := range strings.Split(entities, ",") {
		id = strings.TrimSpace(id)
		if !hass.ValidEntity(id) || seen[id] {
			return cfg, errors.New("list distinct Home Assistant climate or sensor entity IDs in BOILER_MATE_HA_ENTITIES")
		}
		seen[id] = true
		cfg.Entities = append(cfg.Entities, id)
	}
	if len(cfg.Entities) > hass.EntityLimit {
		return cfg, fmt.Errorf("list at most %d Home Assistant entities", hass.EntityLimit)
	}
	cfg.URL, cfg.Token = uri, token
	return cfg, nil
}

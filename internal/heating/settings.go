package heating

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Settings struct {
	Enabled         bool     `json:"enabled"`
	DiscoveryPrefix string   `json:"discoveryPrefix"`
	Thermostats     []string `json:"thermostats"`
	DropoutSeconds  int      `json:"dropoutSeconds"`
	MinCycleSeconds int      `json:"minCycleSeconds"`
	OutdoorSensor   string   `json:"outdoorSensor"`
	Unit            string   `json:"unit"`
	StopAt          *float64 `json:"stopAt"`
	RestartDelta    float64  `json:"restartDelta"`
	SensorTimeout   int      `json:"sensorTimeoutSeconds"`
	Bands           []Band   `json:"bands"`
}

type Band struct {
	From     *float64 `json:"from"`
	MaxPower int      `json:"maxPower"`
}

func defaultSettings() Settings {
	return Settings{
		DiscoveryPrefix: "homeassistant", DropoutSeconds: 120, MinCycleSeconds: 120,
		Unit: "C", RestartDelta: 2, SensorTimeout: 1800,
	}
}

func (s Settings) validate() error {
	if len(s.DiscoveryPrefix) > 128 || !prefixPattern.MatchString(s.DiscoveryPrefix) {
		return errors.New("enter a valid MQTT discovery prefix")
	}
	if s.DropoutSeconds < 5 || s.DropoutSeconds > 86400 {
		return errors.New("set thermostat dropout grace between 5 and 86400 seconds")
	}
	if s.MinCycleSeconds < 30 || s.MinCycleSeconds > 86400 {
		return errors.New("set the minimum cycle interval between 30 and 86400 seconds")
	}
	if s.SensorTimeout < 10 || s.SensorTimeout > 86400 {
		return errors.New("set outdoor sensor timeout between 10 and 86400 seconds")
	}
	if s.Unit != "C" && s.Unit != "F" {
		return errors.New("choose Celsius or Fahrenheit")
	}
	if s.RestartDelta <= 0 || s.RestartDelta > 50 {
		return errors.New("set a restart differential greater than 0 and at most 50 degrees")
	}
	if s.StopAt != nil && !s.validTemperature(*s.StopAt) {
		return errors.New("the outdoor cutoff temperature is out of range")
	}
	if (s.StopAt != nil || len(s.Bands) > 0) && s.OutdoorSensor == "" {
		return errors.New("choose an outdoor sensor to use the cutoff or maximum-power bands")
	}
	if s.Enabled && len(s.Thermostats) == 0 && s.StopAt == nil && len(s.Bands) == 0 {
		return errors.New("choose at least one thermostat, an outdoor cutoff, or a maximum-power band")
	}
	if len(s.Thermostats) > 16 || len(s.Bands) > 16 {
		return errors.New("select at most 16 thermostats and enter at most 16 maximum-power bands")
	}
	seen := make(map[string]bool)
	for _, id := range s.Thermostats {
		if !s.validSource(id, "climate") || seen[id] {
			return errors.New("choose distinct thermostats from MQTT discovery")
		}
		seen[id] = true
	}
	if s.OutdoorSensor != "" && !s.validSource(s.OutdoorSensor, "sensor") {
		return errors.New("choose an outdoor temperature sensor from MQTT discovery")
	}
	for i, band := range s.Bands {
		if band.MaxPower < 1 || band.MaxPower > 100 {
			return errors.New("enter each maximum power as a whole percentage from 1 to 100")
		}
		if i == 0 {
			if band.From != nil {
				return errors.New("the first band covers temperatures below the next band and must have no starting temperature")
			}
			continue
		}
		if band.From == nil || !s.validTemperature(*band.From) || (i > 1 && *band.From <= *s.Bands[i-1].From) {
			return errors.New("enter each band's starting temperature in strictly increasing order")
		}
	}
	return nil
}

var prefixPattern = regexp.MustCompile(`^[^/+#\x00]+(?:/[^/+#\x00]+)*$`)
var componentPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (s Settings) validSource(id, kind string) bool {
	topic, component, split := strings.Cut(id, "#")
	if len(id) > 512 || strings.ContainsAny(topic, "+\x00") {
		return false
	}
	actual := discoveryKind(s.DiscoveryPrefix, topic)
	if split {
		return actual == "device" && componentPattern.MatchString(component)
	}
	return actual == kind
}

func (s Settings) validTemperature(value float64) bool {
	_, valid := temperatureValue("°"+s.Unit, fmt.Sprint(value))
	return valid
}

func (s Settings) displayTemperature(celsius float64) float64 {
	if s.Unit == "F" {
		return celsius*9/5 + 32
	}
	return celsius
}

type savedState struct {
	Serial        string    `json:"serial"`
	Settings      Settings  `json:"settings"`
	OutdoorResume bool      `json:"outdoorResume"`
	LastPowerAt   time.Time `json:"lastPowerAt"`
	LastPowerKey  string    `json:"lastPowerKey"`
}

func load(path, serial string) (savedState, error) {
	state := savedState{Serial: serial, Settings: defaultSettings()}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	var stored savedState
	if err := json.Unmarshal(data, &stored); err != nil {
		return state, err
	}
	if stored.Serial == serial {
		state = stored
	}
	return state, state.Settings.validate()
}

func save(path string, state savedState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".external-heating-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func sameOutdoor(a, b Settings) bool {
	return a.DiscoveryPrefix == b.DiscoveryPrefix && a.OutdoorSensor == b.OutdoorSensor
}

func sameThermostats(a, b Settings) bool {
	return a.DiscoveryPrefix == b.DiscoveryPrefix && slices.Equal(a.Thermostats, b.Thermostats)
}

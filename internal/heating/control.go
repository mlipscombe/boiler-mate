package heating

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/mlipscombe/boiler-mate/nbe"
)

const (
	Start    = "misc.start"
	Stop     = "misc.stop"
	MaxPower = "regulation.boiler_power_max"
)

var ErrSuperseded = errors.New("external heating decision changed")
var ErrReadOnly = errors.New("enable boiler writes and configure a controller PIN before enabling external heating control")
var ErrDisabled = errors.New("enable external heating control before resuming it")

type WriteFunc func(ctx context.Context, key string, value int, current func() bool) error

type Options struct {
	Write             WriteFunc
	DashboardReadings bool
	ThermostatUnit    string
}

type Control struct {
	path              string
	logger            *slog.Logger
	write             WriteFunc
	wake              chan struct{}
	dashboardReadings bool
	thermostatUnit    string

	operation sync.Mutex
	mu        sync.Mutex
	savedState
	watch          func([]string)
	discovery      map[string][]inputSpec
	zwaveDevices   map[string]inputSpec
	zwaveGateways  map[string]struct{}
	active         map[string]*input
	missingSince   map[string]time.Time
	seenThermostat bool
	idleSince      time.Time
	mqttEpoch      uint64

	cutoff                 bool
	band                   int
	boiler                 string
	boilerAt               time.Time
	powerConfirmed         bool
	capAt                  time.Time
	capMinimum, capMaximum float64
	commandRetry, capRetry time.Time
	retryPowerKey          string
	lastError, capError    string
}

type InputStatus struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Topic        string   `json:"topic"`
	Problem      string   `json:"problem,omitempty"`
	Action       string   `json:"action,omitempty"`
	Availability string   `json:"availability,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
}

type Status struct {
	Settings           Settings      `json:"settings"`
	Writable           bool          `json:"writable"`
	ManualOff          bool          `json:"manualOff"`
	Inputs             []InputStatus `json:"inputs"`
	OutdoorTemperature *float64      `json:"outdoorTemperature"`
	CutoffActive       bool          `json:"cutoffActive"`
	BandPower          *int          `json:"bandPower"`
	Decision           string        `json:"decision"`
	Error              string        `json:"error,omitempty"`
	PowerError         string        `json:"powerError,omitempty"`
}

func New(dataDir, serial string, logger *slog.Logger, options Options) *Control {
	path := filepath.Join(dataDir, "external-heating.json")
	defaults := defaultSettings()
	if options.ThermostatUnit == "F" {
		defaults.Unit = "F"
		defaults.RestartDelta = 3.6
	}
	saved, err := load(path, serial, defaults)
	control := &Control{
		path: path, logger: logger, write: options.Write, wake: make(chan struct{}, 1),
		dashboardReadings: options.DashboardReadings, thermostatUnit: "°" + defaults.Unit,
		discovery: make(map[string][]inputSpec), missingSince: make(map[string]time.Time), band: -1,
		zwaveDevices: make(map[string]inputSpec), zwaveGateways: make(map[string]struct{}),
		savedState: saved, cutoff: saved.OutdoorResume,
	}
	if err != nil {
		logger.Warn("Could not load external heating settings", "error", err)
		control.savedState = savedState{Serial: serial, Settings: defaults}
		control.cutoff = false
		control.lastError = "Could not load saved settings. Saving will replace them."
	}
	control.updateDemand(time.Now())
	return control
}

func (c *Control) Enabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Settings.Enabled
}

func (c *Control) Watch(setTopics func([]string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watch = setTopics
	c.updateInputs()
}

func (c *Control) Set(settings Settings) error {
	if err := settings.validate(); err != nil {
		return err
	}
	if settings.Enabled && c.write == nil {
		return ErrReadOnly
	}
	c.operation.Lock()
	defer c.operation.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.Settings
	next := c.savedState
	next.Settings = settings
	if !sameOutdoor(previous, settings) || settings.StopAt == nil || !settings.Enabled {
		next.OutdoorResume = false
	}
	if err := save(c.path, next); err != nil {
		c.logger.Error("Could not save external heating settings", "error", err)
		return ErrSave
	}
	c.savedState = next
	if previous.DiscoveryPrefix != settings.DiscoveryPrefix {
		clear(c.discovery)
	}
	if !sameThermostats(previous, settings) || previous.Enabled != settings.Enabled {
		c.seenThermostat = false
		c.idleSince = time.Time{}
		clear(c.missingSince)
	}
	if !sameOutdoor(previous, settings) || !reflect.DeepEqual(previous.StopAt, settings.StopAt) || previous.Unit != settings.Unit || previous.Enabled != settings.Enabled {
		c.cutoff = false
	}
	c.band = -1
	c.lastError, c.capError = "", ""
	if !reflect.DeepEqual(previous.Bands, settings.Bands) || !sameOutdoor(previous, settings) || previous.Unit != settings.Unit {
		c.capRetry = time.Time{}
	}
	c.updateInputs()
	c.updateDemand(time.Now())
	c.notify()
	return nil
}

var ErrSave = errors.New("could not save external heating settings")

func (c *Control) Receive(topic string, payload []byte, retained bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	switch kind := discoveryKind(c.Settings.DiscoveryPrefix, topic); kind {
	case "climate", "sensor", "device":
		if specs := readDiscovery(topic, kind, payload); len(specs) > 0 {
			c.discovery[topic] = specs
		} else {
			delete(c.discovery, topic)
		}
		c.updateInputs()
	default:
		if c.receiveZWave(topic, payload) {
			c.updateInputs()
		}
		for _, in := range c.active {
			in.receive(topic, payload, retained, now)
		}
	}
	c.updateDemand(now)
	c.notify()
}

func (c *Control) Disconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mqttEpoch++
	clear(c.discovery)
	clear(c.zwaveDevices)
	clear(c.zwaveGateways)
	c.updateInputs()
	c.updateDemand(time.Now())
	c.notify()
}

func (c *Control) updateInputs() {
	selected := make(map[string]*input)
	topics := append(discoveryTopics(c.Settings.DiscoveryPrefix), zwaveTopics...)
	activate := func(spec inputSpec) {
		selectedForControl := c.selected(spec)
		if spec.Problem != "" || (!selectedForControl && !c.dashboardReadings) {
			return
		}
		if spec.Kind == thermostat && spec.Unit == "" {
			spec.Unit = c.thermostatUnit
		}
		in := c.active[spec.ID]
		if in == nil || !sameSignal(in.spec, spec) {
			in = newInput(spec)
		}
		selected[spec.ID] = in
		topics = append(topics, spec.Value.Topic)
		if c.dashboardReadings && spec.RoomTemperature.Topic != "" {
			topics = append(topics, spec.RoomTemperature.Topic)
		}
		for _, source := range spec.Availability {
			topics = append(topics, source.Topic)
		}
	}
	for _, specs := range c.discovery {
		for _, spec := range specs {
			activate(spec)
		}
	}
	for _, spec := range c.zwaveDevices {
		activate(c.zwaveInput(spec))
	}
	c.active = selected
	if c.watch != nil {
		c.watch(topics)
	}
}

func sameSignal(a, b inputSpec) bool {
	a.Name, b.Name = "", ""
	return reflect.DeepEqual(a, b)
}

func (c *Control) selected(spec inputSpec) bool {
	return spec.Kind == thermostat && slices.Contains(c.Settings.Thermostats, spec.ID) ||
		spec.Kind == temperature && c.Settings.OutdoorSensor == spec.ID
}

func (c *Control) updateDemand(now time.Time) {
	demand := false
	for _, id := range c.Settings.Thermostats {
		action := c.active[id].currentAction()
		if action == "" {
			if c.missingSince[id].IsZero() {
				c.missingSince[id] = now
			}
		} else {
			c.seenThermostat = true
			delete(c.missingSince, id)
		}
		demand = demand || action == "heating"
	}
	if demand {
		c.idleSince = time.Time{}
	} else if c.idleSince.IsZero() {
		c.idleSince = now
	}
}

func (c *Control) Observe(topic string, data map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch topic {
	case "operating_data":
		c.boiler, _ = data["state_on"].(string)
		c.boilerAt = time.Now()
		c.powerConfirmed = c.powerConfirmed || c.LastPowerKey == Start && c.boiler == "ON" || c.LastPowerKey == Stop && c.boiler == "OFF"
		c.observePower()
	case "regulation":
		minimum, hasMin := numeric(data["boiler_power_min"])
		maximum, hasMax := numeric(data["boiler_power_max"])
		c.capAt = time.Time{}
		if hasMin && hasMax && minimum >= 0 && minimum <= 100 && maximum >= 0 && maximum <= 100 {
			c.capMinimum, c.capMaximum = minimum, maximum
			c.capAt = time.Now()
		}
	default:
		return
	}
	c.notify()
}

func numeric(value any) (float64, bool) {
	switch value := value.(type) {
	case int64:
		return float64(value), true
	case nbe.RoundedFloat:
		return float64(value), true
	default:
		return 0, false
	}
}

func (c *Control) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

type command struct {
	key         string
	value       int
	outdoorStop bool
}

func (c *Control) Snapshot() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	_, status := c.plan(now)
	status.Settings.Bands = slices.Clone(status.Settings.Bands)
	status.Settings.Thermostats = slices.Clone(status.Settings.Thermostats)
	add := func(spec inputSpec) {
		in := c.active[spec.ID]
		row := InputStatus{
			ID: spec.ID, Name: spec.Name, Kind: spec.Kind, Topic: spec.Value.Topic, Problem: spec.Problem,
			Action: in.currentAction(), Availability: in.availabilityState(),
		}
		var timeout time.Duration
		if spec.Kind == temperature {
			timeout = time.Duration(c.Settings.SensorTimeout) * time.Second
		}
		if celsius, ok := in.currentTemperature(now, timeout); ok {
			value := c.Settings.displayTemperature(celsius)
			row.Temperature = &value
		}
		status.Inputs = append(status.Inputs, row)
	}
	for _, specs := range c.discovery {
		for _, spec := range specs {
			add(spec)
		}
	}
	for _, spec := range c.zwaveDevices {
		add(c.zwaveInput(spec))
	}
	slices.SortFunc(status.Inputs, func(a, b InputStatus) int {
		if a.Name == b.Name {
			return cmp.Compare(a.ID, b.ID)
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return status
}

func (c *Control) plan(now time.Time) (command, Status) {
	s := c.Settings
	status := Status{Settings: s, Writable: c.write != nil, ManualOff: c.ManualOff, Error: c.lastError, PowerError: c.capError}
	celsius, tempKnown := c.active[s.OutdoorSensor].currentTemperature(now, time.Duration(s.SensorTimeout)*time.Second)
	if tempKnown {
		value := s.displayTemperature(celsius)
		status.OutdoorTemperature = &value
		if s.StopAt != nil {
			switch {
			case value >= *s.StopAt:
				c.cutoff = true
			case value <= *s.StopAt-s.RestartDelta:
				c.cutoff = false
			}
		}
		if len(s.Bands) > 0 {
			c.band = powerBand(s, c.band, value)
			power := s.Bands[c.band].MaxPower
			status.BandPower = &power
		}
	}
	status.CutoffActive = s.StopAt != nil && c.cutoff
	if !s.Enabled {
		status.Decision = "External heating control is disabled."
		return command{}, status
	}
	if c.write == nil {
		status.Decision = "Boiler writes are disabled."
		return command{}, status
	}
	if c.ManualOff {
		status.Decision = "The boiler was stopped manually. Resume automatic control when you want it to follow heat demand again."
		return command{}, status
	}
	desired, reason := c.desiredPower(now, status.CutoffActive, tempKnown)
	status.Decision = reason
	freshBoiler := !c.boilerAt.IsZero() && now.Sub(c.boilerAt) < 15*time.Second
	needsChange := desired == Start && c.boiler == "OFF" || desired == Stop && c.boiler == "ON"
	waitingForPower := desired != "" && (!freshBoiler || c.boiler != "ON" && c.boiler != "OFF")
	if waitingForPower {
		status.Decision += " Waiting for a confirmed boiler state."
	}
	canChange := (desired != c.retryPowerKey || !now.Before(c.commandRetry)) && (now.Sub(c.LastPowerAt) >= time.Duration(s.MinCycleSeconds)*time.Second ||
		status.CutoffActive && desired == Stop && (c.LastPowerKey != Stop || c.powerConfirmed))
	if needsChange && !canChange && now.Before(c.LastPowerAt.Add(time.Duration(s.MinCycleSeconds)*time.Second)) {
		status.Decision += " Waiting for the minimum cycle interval."
	}
	if freshBoiler && needsChange && canChange && desired == Stop {
		return command{Stop, 1, status.CutoffActive}, status
	}
	if power := status.BandPower; power != nil {
		if c.capAt.IsZero() || now.Sub(c.capAt) >= 30*time.Second {
			status.PowerError = "Waiting for the controller's current burner limits."
			return command{}, status
		}
		if float64(*power) < c.capMinimum {
			status.PowerError = "This band's maximum is below the controller's minimum burner power."
			return command{}, status
		}
		if float64(*power) != c.capMaximum {
			if !now.Before(c.capRetry) {
				return command{MaxPower, *power, false}, status
			}
			return command{}, status
		}
		status.PowerError = ""
	}
	if freshBoiler && needsChange && canChange {
		return command{Start, 1, false}, status
	}
	return command{}, status
}

func (c *Control) desiredPower(now time.Time, cutoff, tempKnown bool) (key, reason string) {
	if cutoff {
		return Stop, "Outdoor cutoff is active."
	}
	s := c.Settings
	var calling, waiting bool
	for _, id := range s.Thermostats {
		calling = calling || c.active[id].currentAction() == "heating"
		since := c.missingSince[id]
		waiting = waiting || !since.IsZero() && now.Sub(since) < time.Duration(s.DropoutSeconds)*time.Second
	}
	if calling || c.OutdoorResume && len(s.Thermostats) == 0 && s.StopAt != nil {
		if s.StopAt != nil && !tempKnown {
			return "", "Waiting for the outdoor sensor before starting."
		}
		if calling {
			return Start, "At least one thermostat is calling for heat."
		}
		return Start, "Outdoor cutoff has cleared."
	}
	if len(s.Thermostats) == 0 {
		switch {
		case !tempKnown:
			return "", "Waiting for an outdoor temperature reading."
		case s.StopAt != nil:
			return "", "Outdoor temperature is below the cutoff."
		default:
			return "", "Outdoor temperature is controlling maximum burner power."
		}
	}
	if !c.seenThermostat {
		return "", "Waiting for the first thermostat action."
	}
	if waiting {
		return "", "Waiting for a missing thermostat to return."
	}
	if now.Sub(c.idleSince) < 10*time.Second {
		return "", "Checking that heat demand has ended."
	}
	return Stop, "No thermostat is calling for heat."
}

func powerBand(settings Settings, index int, temperature float64) int {
	bands := settings.Bands
	hysteresis := (settings.displayTemperature(1) - settings.displayTemperature(0)) / 2
	if index < 0 || index >= len(bands) {
		index, hysteresis = 0, 0
	}
	for index+1 < len(bands) && temperature >= *bands[index+1].From+hysteresis {
		index++
	}
	for index > 0 && temperature < *bands[index].From-hysteresis {
		index--
	}
	return index
}

func (c *Control) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-c.wake:
		}
		c.apply(ctx)
	}
}

func (c *Control) apply(ctx context.Context) {
	c.operation.Lock()
	defer c.operation.Unlock()
	c.mu.Lock()
	cmd, _ := c.plan(time.Now())
	c.mu.Unlock()
	if cmd.key == "" {
		return
	}
	err := c.write(ctx, cmd.key, cmd.value, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		now := time.Now()
		current, _ := c.plan(now)
		if current != cmd {
			return false
		}
		if cmd.key == Stop {
			// Record the stop before sending it. A telemetry read can see Off
			// before the command reply arrives, including when the reply is lost.
			c.AutoStopAt = now
			c.savePowerState()
		}
		return true
	})
	if errors.Is(err, ErrSuperseded) || ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if cmd.key == MaxPower {
		c.capRetry = now.Add(time.Minute)
		if err != nil {
			c.capError = err.Error()
		} else {
			c.capError = ""
		}
		return
	}
	if err != nil {
		var rejected *nbe.ResponseError
		if cmd.key == Stop && !c.AutoStopAt.IsZero() && errors.As(err, &rejected) {
			c.AutoStopAt = time.Time{}
			c.savePowerState()
		}
		c.lastError = err.Error()
		c.commandRetry = now.Add(30 * time.Second)
		c.retryPowerKey = cmd.key
		return
	}
	c.lastError = ""
	c.retryPowerKey = ""
	c.LastPowerAt, c.LastPowerKey = now, cmd.key
	c.powerConfirmed = false
	if cmd.outdoorStop && !c.ManualOff {
		c.OutdoorResume = true
	}
	if cmd.key == Start {
		c.OutdoorResume = false
		c.AutoStopAt = time.Time{}
	}
	c.savePowerState()
}

package heating

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	thermostat  = "thermostat"
	temperature = "temperature"
)

func discoveryTopics(prefix string) []string {
	return []string{
		prefix + "/climate/+/config", prefix + "/climate/+/+/config",
		prefix + "/sensor/+/config", prefix + "/sensor/+/+/config",
		prefix + "/device/+/config", prefix + "/device/+/+/config",
	}
}

func discoveryKind(prefix, topic string) string {
	path, ok := strings.CutPrefix(topic, prefix+"/")
	if !ok {
		return ""
	}
	parts := strings.Split(path, "/")
	if (len(parts) == 3 || len(parts) == 4) && parts[len(parts)-1] == "config" {
		return parts[0]
	}
	return ""
}

type discoveryPayload map[string]json.RawMessage

func (p discoveryPayload) get(key, alias string) json.RawMessage {
	if value, ok := p[key]; ok {
		return value
	}
	return p[alias]
}

func (p discoveryPayload) str(key, alias string) string {
	var value string
	_ = json.Unmarshal(p.get(key, alias), &value)
	return value
}

type valueSource struct {
	Topic             string
	Field             string
	ZigbeeAction      bool
	TemperatureFormat string
}

type availabilitySource struct {
	valueSource
	Online, Offline string
}

type inputSpec struct {
	ID, Name, Kind, Unit string
	Value                valueSource
	Availability         []availabilitySource
	Mode                 string
	ExpireAfter          time.Duration
	Problem              string
}

func readDiscovery(topic, kind string, payload []byte) []inputSpec {
	var root discoveryPayload
	if json.Unmarshal(payload, &root) != nil {
		return nil
	}
	if kind != "device" {
		if input, ok := readInput(topic, kind, root); ok {
			return []inputSpec{input}
		}
		return nil
	}

	var components map[string]discoveryPayload
	_ = json.Unmarshal(root.get("components", "cmps"), &components)
	var inputs []inputSpec
	for key, component := range components {
		kind := component.str("platform", "p")
		if kind != "sensor" && kind != "climate" {
			continue
		}
		// Only these shared options can apply to the signals we consume.
		for _, keys := range [][2]string{{"~", "~"}, {"device", "dev"}, {"state_topic", "stat_t"},
			{"availability", "avty"}, {"availability_mode", "avty_mode"}, {"availability_topic", "avty_t"},
			{"availability_template", "avty_tpl"}, {"payload_available", "pl_avail"}, {"payload_not_available", "pl_not_avail"}} {
			if component.get(keys[0], keys[1]) == nil {
				if value := root.get(keys[0], keys[1]); value != nil {
					component[keys[0]] = value
				}
			}
		}
		if input, ok := readInput(topic+"#"+key, kind, component); ok {
			inputs = append(inputs, input)
		}
	}
	return inputs
}

func readInput(id, kind string, payload discoveryPayload) (inputSpec, bool) {
	input := inputSpec{ID: id}
	var device discoveryPayload
	_ = json.Unmarshal(payload.get("device", "dev"), &device)
	deviceName, name := device.str("name", "name"), payload.str("name", "name")
	switch {
	case deviceName != "" && name != "" && name != deviceName:
		input.Name = deviceName + " · " + name
	case name != "":
		input.Name = name
	case deviceName != "":
		input.Name = deviceName
	default:
		input.Name = id
	}
	base := payload.str("~", "~")
	var err error
	switch kind {
	case "climate":
		input.Kind = thermostat
		input.Value, err = readValueSource(base, payload.str("action_topic", "act_t"), payload.str("action_template", "act_tpl"), true)
	case "sensor":
		class := payload.str("device_class", "dev_cla")
		input.Unit = payload.str("unit_of_measurement", "unit_of_meas")
		temperatureUnit := slices.Contains([]string{"°C", "°F", "K"}, input.Unit)
		if class != "temperature" && !(class == "" && temperatureUnit) {
			return inputSpec{}, false
		}
		input.Kind = temperature
		if !temperatureUnit {
			input.Problem = "Temperature sensor must report °C, °F, or K."
		}
		var seconds int64
		_ = json.Unmarshal(payload.get("expire_after", "exp_aft"), &seconds)
		if seconds > 0 && seconds <= int64((1<<63-1)/time.Second) {
			input.ExpireAfter = time.Duration(seconds) * time.Second
		}
		input.Value, err = readTemperatureSource(base, payload.str("state_topic", "stat_t"), payload.str("value_template", "val_tpl"))
	default:
		return inputSpec{}, false
	}
	if err != nil {
		input.Problem = err.Error()
	}
	input.Mode = payload.str("availability_mode", "avty_mode")
	if input.Mode == "" {
		input.Mode = "latest"
	}
	if !slices.Contains([]string{"all", "any", "latest"}, input.Mode) {
		input.Problem = "Unsupported MQTT availability mode."
	}
	var sources []discoveryPayload
	if source := payload.get("availability", "avty"); source != nil {
		if json.Unmarshal(source, &sources) != nil {
			input.Problem = "Invalid MQTT availability configuration."
		}
	} else if payload.str("availability_topic", "avty_t") != "" {
		sources = []discoveryPayload{{
			"topic": payload.get("availability_topic", "avty_t"), "value_template": payload.get("availability_template", "avty_tpl"),
			"payload_available": payload.get("payload_available", "pl_avail"), "payload_not_available": payload.get("payload_not_available", "pl_not_avail"),
		}}
	}
	for _, source := range sources {
		value, err := readValueSource(base, source.str("topic", "t"), source.str("value_template", "val_tpl"), false)
		if err != nil {
			input.Problem = "Unsupported MQTT availability topic or template."
			break
		}
		online, offline := source.str("payload_available", "pl_avail"), source.str("payload_not_available", "pl_not_avail")
		if online == "" {
			online = "online"
		}
		if offline == "" {
			offline = "offline"
		}
		input.Availability = append(input.Availability, availabilitySource{value, online, offline})
	}
	return input, true
}

const jsonFieldPattern = `value_json(?:\.([A-Za-z0-9_]+)|\["([A-Za-z0-9_]+)"\]|\['([A-Za-z0-9_]+)'\])`

var (
	fieldTemplate       = regexp.MustCompile(`^\{\{\s*` + jsonFieldPattern + `\s*\}\}$`)
	zigbeeTemplate      = regexp.MustCompile(`^\{%\s*set values\s*=\s*\{\s*None\s*:\s*None\s*,\s*'idle'\s*:\s*'idle'\s*,\s*'heat'\s*:\s*'heating'\s*,\s*'cool'\s*:\s*'cooling'\s*,\s*'fan_only'\s*:\s*'fan'\s*\}\s*%\}\s*\{\{\s*values\[` + jsonFieldPattern + `\]\s*\}\}$`)
	rawTemplate         = regexp.MustCompile(`^\{\{\s*value\s*\}\}$`)
	fieldGuard          = regexp.MustCompile(`^\{%\s*if\s+"([A-Za-z0-9_]+)"\s+in value_json\s*%\}\s*(.*?)\s*\{%\s*endif\s*%\}$`)
	temperatureTemplate = regexp.MustCompile(`^\{\{\s*value\s*\|\s*float\s*(?:\|\s*round\(\s*([0-6])\s*\)\s*)?\}\}$`)
	unsupportedTemplate = errors.New("The device advertises an unsupported MQTT value template.")
)

func readTemperatureSource(base, topic, template string) (valueSource, error) {
	match := temperatureTemplate.FindStringSubmatch(strings.TrimSpace(template))
	if match == nil {
		return readValueSource(base, topic, template, false)
	}
	source, err := readValueSource(base, topic, "", false)
	source.TemperatureFormat = match[1]
	return source, err
}

func readValueSource(base, topic, template string, action bool) (valueSource, error) {
	if base != "" && strings.HasPrefix(topic, "~") {
		topic = base + topic[1:]
	} else if base != "" && strings.HasSuffix(topic, "~") {
		topic = topic[:len(topic)-1] + base
	}
	if topic == "" || strings.ContainsAny(topic, "\x00+#") || !utf8.ValidString(topic) || len(topic) > 65535 {
		return valueSource{}, errors.New("No usable MQTT state topic is advertised.")
	}
	source := valueSource{Topic: topic}
	template = strings.TrimSpace(template)
	if template == "" || rawTemplate.MatchString(template) {
		return source, nil
	}
	guard := fieldGuard.FindStringSubmatch(template)
	if guard != nil {
		template = guard[2]
	}
	match := fieldTemplate.FindStringSubmatch(template)
	if action && match == nil {
		match = zigbeeTemplate.FindStringSubmatch(template)
		source.ZigbeeAction = match != nil
	}
	if match == nil {
		return valueSource{}, unsupportedTemplate
	}
	for _, field := range match[1:] {
		if field != "" {
			source.Field = field
		}
	}
	if guard != nil && guard[1] != source.Field {
		return valueSource{}, unsupportedTemplate
	}
	return source, nil
}

func (s valueSource) value(payload []byte) (string, bool) {
	if s.Field == "" {
		value := strings.TrimSpace(string(payload))
		if s.TemperatureFormat != "" {
			if number, err := strconv.ParseFloat(value, 64); err == nil {
				precision := int(s.TemperatureFormat[0] - '0')
				value = strconv.FormatFloat(number, 'f', precision, 64)
			}
		}
		return value, true
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(payload, &obj) != nil || obj[s.Field] == nil {
		return "", false
	}
	field := obj[s.Field]
	if len(field) > 0 && field[0] == '"' {
		var value string
		if json.Unmarshal(field, &value) != nil {
			return "", false
		}
		return value, true
	}
	return string(field), true
}

func actionValue(source valueSource, value string) (state string, ok bool) {
	if source.ZigbeeAction {
		switch value {
		case "heat":
			value = "heating"
		case "cool":
			value = "cooling"
		case "fan_only":
			value = "fan"
		}
	}
	switch value {
	case "heating", "idle", "off", "cooling", "drying", "fan":
		return value, true
	case "None", "null", "unavailable", "unknown":
		return "", true
	default:
		return "", false
	}
}

func temperatureValue(unit, value string) (float64, bool) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case "°F":
		number = (number - 32) * 5 / 9
	case "K":
		number -= 273.15
	}
	return number, number >= -100 && number <= 100
}

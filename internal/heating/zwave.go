package heating

import (
	"strconv"
	"strings"
)

const zwavePrefix = "zwave"

var zwaveTopics = []string{
	zwavePrefix + "/+/thermostat_operating_state/+/state",
	zwavePrefix + "/+/+/thermostat_operating_state/+/state",
	zwavePrefix + "/_CLIENTS/+/status",
}

func zwaveNode(topic string) (node, endpoint string, ok bool) {
	path, ok := strings.CutPrefix(topic, zwavePrefix+"/")
	if !ok {
		return "", "", false
	}
	parts := strings.Split(path, "/")
	switch len(parts) {
	case 4, 5:
	default:
		return "", "", false
	}
	index := len(parts) - 3
	if parts[index] != "thermostat_operating_state" || parts[index+2] != "state" || parts[index-1] == "" || parts[0] == "" {
		return "", "", false
	}
	endpoint, ok = strings.CutPrefix(parts[index+1], "endpoint_")
	if _, err := strconv.ParseUint(endpoint, 10, 8); !ok || err != nil {
		return "", "", false
	}
	return zwavePrefix + "/" + strings.Join(parts[:index], "/"), endpoint, true
}

func zwaveGatewayStatus(topic string) bool {
	path, ok := strings.CutPrefix(topic, zwavePrefix+"/_CLIENTS/ZWAVE_GATEWAY-")
	name, matched := strings.CutSuffix(path, "/status")
	return ok && matched && name != "" && !strings.Contains(name, "/")
}

func zwaveThermostat(topic, node, endpoint string) inputSpec {
	name := node[strings.LastIndexByte(node, '/')+1:]
	name = strings.ReplaceAll(name, "_", " ")
	if endpoint != "0" {
		name += " · Endpoint " + endpoint
	}
	return inputSpec{
		ID: topic, Name: name, Kind: thermostat, Mode: "all", ZWave: true,
		Value: valueSource{Topic: topic, Field: "value", FallbackRaw: true},
	}
}

func zwaveAvailability(topic string) availabilitySource {
	return availabilitySource{
		valueSource: valueSource{Topic: topic, Field: "value", FallbackRaw: true},
		Online:      "true", Offline: "false",
	}
}

// Z-Wave Thermostat Operating State reports active stages independently of the
// thermostat mode. Pending heat and pending cool map to idle in Z-Wave JS UI.
func zwaveAction(value string) (string, bool) {
	switch value {
	case "1", "7", "8", "10", "11":
		return "heating", true
	case "0", "4", "5":
		return "idle", true
	case "2", "9":
		return "cooling", true
	case "3", "6":
		return "fan", true
	case "null", "unknown", "unavailable":
		return "", true
	default:
		_, err := strconv.Atoi(value)
		return "", err == nil
	}
}

func (c *Control) receiveZWave(topic string, payload []byte) bool {
	if zwaveGatewayStatus(topic) {
		_, known := c.zwaveGateways[topic]
		if len(payload) == 0 {
			delete(c.zwaveGateways, topic)
			return known
		}
		state, valid := zwaveAvailability(topic).value(payload)
		if !known && valid && (state == "true" || state == "false") {
			c.zwaveGateways[topic] = struct{}{}
			return true
		}
		return false
	}
	if node, endpoint, ok := zwaveNode(topic); ok {
		_, known := c.zwaveDevices[topic]
		if len(payload) == 0 {
			delete(c.zwaveDevices, topic)
			return known
		}
		if !known {
			spec := zwaveThermostat(topic, node, endpoint)
			value, valid := spec.Value.value(payload)
			if _, isState := zwaveAction(value); valid && isState {
				c.zwaveDevices[topic] = spec
				return true
			}
		}
	}
	return false
}

func (c *Control) zwaveInput(spec inputSpec) inputSpec {
	if len(c.zwaveGateways) != 1 {
		spec.Problem = "Heat demand requires one Z-Wave JS UI gateway on the zwave prefix."
		return spec
	}
	node, _, _ := zwaveNode(spec.Value.Topic)
	spec.Availability = []availabilitySource{
		zwaveAvailability(node + "/status"), zwaveAvailability(zwavePrefix + "/driver/status"),
	}
	for gateway := range c.zwaveGateways {
		spec.Availability = append(spec.Availability, zwaveAvailability(gateway))
	}
	return spec
}

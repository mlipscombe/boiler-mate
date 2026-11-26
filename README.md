# Boiler Mate

Boiler Mate connects to a wood-pellet boiler with an NBE controller over the local network. It publishes controller readings to MQTT and supports Home Assistant discovery. The bridge and dashboard work without a StokerCloud account.

## Upgrading to 1.0.0

Boiler Mate now starts a [web server and dashboard](#web-dashboard) for one boiler. It listens on port `8080` by default. For an MQTT-only deployment, set `BOILER_MATE_WEB_BIND=false` to disable the dashboard and keep MQTT and Home Assistant monitoring.

Boiler commands now require an explicit opt-in. If you already control your boiler through MQTT or Home Assistant, follow [Enable boiler control](#enable-boiler-control) when upgrading, even if your existing controller URL already includes a PIN.

## Supported controllers

The project targets NBE V7, V10, and V13 controllers used in systems such as NBE Blackstar, NBE RTB, Kedel, and Opop. Available sensors, settings, and optional circuits depend on controller firmware and installed equipment.

Some V7 and V10 installations require the App Operation firmware option to enable local access. See the [NBE controller resources](https://www.nbe.dk/program-download/).

## Run the bridge

Build with Go 1.27.4 or use the published [container image](https://github.com/mlipscombe/boiler-mate/pkgs/container/boiler-mate).

```sh
go build -o boiler-mate ./cmd/boiler-mate
./boiler-mate \
  --controller 'udp://1234@192.168.1.100:8483' \
  --mqtt 'mqtt://broker.local:1883'
```

Set controller and broker URLs through environment variables to keep passwords out of the command line. Reads work with the serial alone. The historical `tcp://` controller URI spelling is accepted and uses UDP.

| Flag | Environment variable | Default |
| --- | --- | --- |
| `--controller` | `BOILER_MATE_CONTROLLER` | Configure for your controller |
| `--mqtt` | `BOILER_MATE_MQTT` | `mqtt://localhost:1883` |
| `--bind` | `BOILER_MATE_BIND` | `0.0.0.0:2112` |
| `--homeassistant` | `BOILER_MATE_HOMEASSISTANT` | `true` |
| `--log-level` | `BOILER_MATE_LOG_LEVEL` | `info` |
| `--web-bind` | `BOILER_MATE_WEB_BIND` | `127.0.0.1:8080` for the binary and `0.0.0.0:8080` in Docker |
| `--web-data` | `BOILER_MATE_WEB_DATA` | `boiler-mate-data` for the binary and `/data` in Docker |
| — | `BOILER_MATE_ENABLE_WRITES` | `false` |
| `--outdoor-weather-fahrenheit` | `BOILER_MATE_OUTDOOR_WEATHER_FAHRENHEIT` | `false` |

The MQTT prefix defaults to `nbe/<serial>`. Add a path to the broker URL to use a custom prefix. MQTT TLS uses `mqtts://` on port 8883 by default. Optional query parameters are `tls_ca` or `tls_cacert` for a private CA, and `tls_cert` plus `tls_key` for mutual TLS. The legacy `insecure=true` option explicitly disables certificate verification.

Metrics and process probes are served at `/metrics`, `/healthz`, and `/liveness`. Set `--bind=false` to disable that listener.

## Web dashboard

When running the binary directly, open `http://127.0.0.1:8080` on the machine running Boiler Mate. To change the dashboard's listen address or port, set `--web-bind` or `BOILER_MATE_WEB_BIND`. The metrics server uses its own listener. The dashboard needs a writable data directory. Set `--web-data` or `BOILER_MATE_WEB_DATA` to use a different directory.

The Docker image listens on port `8080` on all container interfaces. With Docker bridge networking, publish it with `-p 127.0.0.1:8080:8080` for access from the Docker host. With host networking, set `BOILER_MATE_WEB_BIND` explicitly to the host address you want to listen on. Mount a persistent volume at `/data` writable by UID 10001 to keep dashboard history across container replacements.

Boiler Mate serves the dashboard and API over HTTP without a built-in login. Restrict direct access to a trusted local network, for example by binding to the host's LAN address and limiting access with a firewall. For access beyond that network, put the entire dashboard, including `/api/`, behind an HTTPS reverse proxy that requires authentication. Keep the backend listener reachable only by the proxy so all requests pass through authentication. [Authentik forward auth setup](https://docs.goauthentik.io/add-secure-apps/providers/proxy/forward_auth)

The dashboard includes live telemetry, burner controls, controller settings and help, boiler and hot-water schedules, diagnostics, pellet consumption, signal charts with CSV export, and events. Settings and weekly schedules use the controller's reported ranges and clock. Chart history and events are recorded by this instance. The app reads historical pellet consumption directly from compatible controllers.

Set `BOILER_MATE_OUTDOOR_WEATHER_FAHRENHEIT=true` when the online outdoor weather feed reports Fahrenheit values, as some US installations do. Boiler Mate converts the feed's temperature to Celsius for the dashboard and its history.

Enable desktop notices under **Events → Desktop Notifications** on each browser that should receive them. On browsers with Web Push, notices can arrive after the dashboard tab closes. The server needs outbound HTTPS to the browser's push service and sends encrypted notice payloads. The web data directory stores the push identity and subscriptions alongside dashboard history.

Open **Consumption → Purchase Ledger** to track pellets on hand and fuel cost. Enter the number bought, weight and price per bag, pallet, or other unit, and the brand if known. Each purchase can use pounds or kilograms. For opening stock, enter the weight currently on hand and leave its price blank if unknown. Cost reports use the oldest available stock first and count usage from the first successful controller counter reading. Configure a low-stock threshold in the ledger and enable **Pellet supply** under desktop notifications to receive the alert on that device.

### Enable boiler control

Boiler Mate disables controller writes by default. To allow commands, add these environment variables to your service or container configuration and replace the example serial, PIN, and address. If your service passes `--controller`, remove that flag so it uses `BOILER_MATE_CONTROLLER`. Restart Boiler Mate to apply the changes.

```dotenv
BOILER_MATE_ENABLE_WRITES=true
BOILER_MATE_CONTROLLER=udp://1234:0123456789@192.168.1.100:8483
```

The setting enables commands from both the dashboard and MQTT, including Home Assistant. Use broker access controls to limit who can publish to `<prefix>/set/#`. The controller PIN authorizes communication with the boiler. Access to the dashboard and to MQTT command topics is controlled separately.

### External heating

Open **Integration → Zones** to choose thermostats and link pump readings on each zone’s card. Turn on **Enable zone control** to start and stop the boiler from thermostat demand. Open **Integration → Outdoor Climate** to select an outdoor sensor and configure a cutoff, maximum-power bands, or both. Turn on **Enable outdoor climate control** to apply those rules. Each switch can be saved independently. When both are on, the outdoor cutoff takes priority over thermostat demand. Settings are stored in the dashboard data directory and continue to apply across restarts, including if you later turn off the dashboard. Controller writes and a PIN are required for automatic control.

The start and stop commands operate the entire boiler, including domestic hot water production.

- **Thermostats.** Any thermostat reporting `heating` can start the boiler. After ten seconds without a call, Boiler Mate stops it once any missing thermostat's dropout grace has expired. The default grace and minimum interval between automatic start and stop commands are both two minutes. Before receiving its first usable thermostat action, Boiler Mate leaves boiler power alone. For devices that report only state changes, configure the publisher's MQTT availability so Boiler Mate can detect an outage.
- **Manual stop.** A stop from the dashboard, MQTT, or the controller pauses automatic control. The hold survives a restart. Start the boiler manually or choose **Resume automatic control** to let the configured rules take over again. A boiler waiting because it reached its target temperature is still enabled.
- **Outdoor cutoff.** At or above the cutoff, Boiler Mate stops the boiler even if a thermostat is calling for heat. The cutoff clears after the reading falls by the restart differential. If the sensor goes missing or its reading expires, the last cutoff stays in effect and new starts wait for a usable reading. With zone control off, clearing the cutoff restarts the boiler only if Boiler Mate previously stopped it for that cutoff.
- **Maximum-power bands.** The first band covers the coldest temperatures. Subsequent bands start at their specified temperatures and apply up to the next band. For example, enter 90% for the coldest band, 70% from 35 °F, and 50% from 50 °F. Boiler Mate allows a 0.5 °C margin at band boundaries to avoid repeated changes. It checks the controller's maximum-power range and current minimum burner power before writing. If the sensor goes missing or the bands are disabled, the last value remains in the controller. You can adjust it under **Boiler → Regulation**.

Boiler Mate reads Home Assistant MQTT discovery on the same broker. It supports MQTT climate entities with an action topic and MQTT temperature sensors reporting °C, °F, or K. Plain payloads, simple JSON field templates, and Zigbee2MQTT's standard climate action mapping are supported. Retain discovery messages and current readings for use after a reconnect, and publish availability separately when the device supports it. Devices with unsupported discovery configurations show the reason on the page. [Home Assistant MQTT discovery](https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery) · [MQTT climate actions](https://www.home-assistant.io/integrations/climate.mqtt/#action_topic)

**Zones** also discovers thermostats directly from a Z-Wave JS UI gateway using named MQTT topics under the `zwave` prefix. It uses the thermostat's operating state and the node, driver, and gateway availability topics. [Z-Wave JS UI MQTT guide](https://github.com/zwave-js/zwave-js-ui/blob/master/docs/guide/mqtt.md)

Room and outdoor readings use the units selected under **Outdoor Climate**. If the selected outdoor sensor is waiting for its next report, the dashboard can show a last-seen reading from the past 24 hours and its timestamp. The cutoff and power bands continue to use current readings. For thermostats that omit the unit from their MQTT data, set `BOILER_MATE_MQTT_THERMOSTAT_UNIT` to `C` (default) or `F` to match the readings they publish. New installations also use that unit as the initial display preference.

**Overview** shows the temperature and current action for each configured thermostat. It uses the configured outdoor sensor when its reading is current and labels fresh controller weather as a fallback if the sensor is unavailable.

### Home Assistant inputs

Use direct MQTT discovery for devices that already publish usable MQTT readings, including Z-Wave JS UI thermostats. For other devices, such as an ESPHome circulator, configure the optional Home Assistant connection with these environment variables.

| Variable | Value |
| --- | --- |
| `BOILER_MATE_HA_URL` | Home Assistant base URL, for example `https://homeassistant.example.com` |
| `BOILER_MATE_HA_TOKEN` | A Home Assistant access token supplied through the deployment's secret store |
| `BOILER_MATE_HA_ENTITIES` | Comma-separated list of up to 32 `climate.*` or `sensor.*` entities that need Home Assistant, for example `sensor.circulator_media_temperature,sensor.circulator_flow` |

Boiler Mate reads the listed states every 15 seconds. It uses each climate entity's reported HVAC action for heat demand and offers temperature sensors as outdoor sources under **Integration → Outdoor Climate**. Keep MQTT devices out of the allowlist to give each physical device one source. The connection issues only read requests. The token inherits its Home Assistant account's permissions. [Home Assistant REST API](https://contributors.home-assistant.io/core/internals/rest-api/)

On each card under **Integration → Zones**, choose a **Pump temperature** sensor, a **Pump flow** sensor, or both, then select **Save changes**. **View pump chart** opens the assigned readings alongside boiler temperature and power. Assignments are for monitoring and have no effect on heat-demand control. Boiler Mate records these sensor histories once per minute in °C and m³/h. Chart recording begins when Boiler Mate receives the readings.

The pump chart also shows when the linked thermostat was calling for heat. Boiler Mate keeps up to 30 days of demand history for assigned zones. The track shows an unknown period whenever the thermostat's action cannot be confirmed or demand history has yet to be recorded.

### Efficiency suggestions

Open **Monitor → Efficiency** to see what Boiler Mate has observed over the last seven days. Select the thermostats you want it to follow under **Integration → Zones**. Automatic control can be off while suggestions are collected.

Boiler Mate looks for repeated short burns that reach the boiler’s temperature stop while the same loops still call, oscillation during steady firing with usable P and I readings, and repeated runs that stay below target at the controller’s maximum output. Suggestions show the calling loops, measured evidence, and what to check. Startup, hot-water priority, signal gaps, and changes in operating conditions can interrupt an observation. Suggestions are advisory and require no controller writes.

Under **Efficiency → Weather & Fuel**, Boiler Mate keeps two years of outdoor observations. It prefers the selected local sensor when the reading is current and otherwise uses the boiler’s remote weather feed. Reports integrate the degrees below an adjustable heating base over the observed hours. The default is 65 °F. Equivalent heating degree days are degree hours divided by 24. Check the source and coverage columns when comparing periods.

The fuel trend pairs weather with completed periods from the pellet ledger. Boiler Mate reports kilograms per equivalent degree day when weather coverage and the pellet counter support the comparison. Brand comparisons use periods attributed to one brand by the ledger. Pellet use can include domestic hot water, and the ledger estimates which purchase was consumed.

### Controller reads in the dashboard and MQTT

Open **Charts → Controller History** for chart IDs stored by the boiler, **Events → Controller Log** for its event log, **Live Data → Selected Readings** for a one-time group of operating values, or **System → Firmware Versions / Wireless Sensors** for controller lists. Firmware and hardware determine which reads succeed. Chart IDs have controller-specific meanings.

Subscribe to `<prefix>/controller/+` for retained JSON results. Each result includes `read_at`, the URL-encoded `query`, and `data` for a successful read or `error` for a failed read. To request a read, publish a non-retained message to `<prefix>/get/controller/<name>`. These reads work with controller writes disabled.

| Name | Request payload | Result topic under `<prefix>/controller/` |
| --- | --- | --- |
| `versions` | Empty | `versions` |
| `sensors` | Empty | `sensors` |
| `events` | Empty for recent events, or `at=1732442400` for a Unix timestamp | `events` or `event_history` for a dated request |
| `values` | `names=boiler_temp,state` | `values` |
| `chart` | `from=1732442200&to=1732442400&step=1&ids=0,3` | `chart` |

Chart bounds are Unix timestamps on the same controller-local day. Step is in minutes and IDs are in ascending order. The bridge refreshes firmware versions hourly and recent events and room sensors approximately once a minute. Chart and selected-value reads run on request. When a controller declines a compact selected-value read, the bridge extracts the requested fields from one fresh operating-data snapshot. Dashboard reads also publish their results to MQTT. Home Assistant discovery adds firmware-version and wireless-sensor counts with the full results in their attributes. The dashboard reuses a successful firmware list for one hour. Refresh requests a fresh copy. If that read fails, the page keeps its displayed list, and MQTT includes the error alongside any still-fresh cached list and its original read time. Without a current list the count is unknown.

### Frontend development

The binary embeds the precompiled Tailwind CSS and JavaScript. After editing the frontend, rebuild the stylesheet with Node.js 26 and commit the generated CSS alongside the source.

```sh
npm ci
npm run build:css
npm run check:js
```

## Testing

```sh
make fmt-check vet
go test -race ./...
make test-integration
```

The integration suite uses a read-only simulated boiler and Mosquitto. To use a dedicated test broker instead of Compose, set `TEST_MQTT_URL` and run `make test-integration`. The test exercises retained telemetry, discovery, metrics, and dashboard reads. The simulator rejects settings writes.

## License

Boiler Mate is distributed under the GNU GPL v3. Thanks to [Anders Nylund](https://github.com/motoz) for documenting the NBE protocol.

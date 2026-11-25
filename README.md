# Boiler Mate

Boiler Mate connects to a wood-pellet boiler with an NBE controller over the local network. It publishes controller readings to MQTT, supports Home Assistant discovery, and can serve a standalone dashboard for one boiler. The bridge and dashboard work without a StokerCloud account.

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

Set controller and broker URLs through environment variables to keep passwords out of the command line. Reads work with the serial alone. Set `BOILER_MATE_ENABLE_WRITES=true` and configure a controller PIN to enable dashboard and MQTT commands. The historical `tcp://` controller URI spelling is accepted and uses UDP.

| Flag | Environment variable | Default |
| --- | --- | --- |
| `--controller` | `BOILER_MATE_CONTROLLER` | Configure for your controller |
| `--mqtt` | `BOILER_MATE_MQTT` | `mqtt://localhost:1883` |
| `--bind` | `BOILER_MATE_BIND` | `0.0.0.0:2112` |
| `--homeassistant` | `BOILER_MATE_HOMEASSISTANT` | `true` |
| `--log-level` | `BOILER_MATE_LOG_LEVEL` | `info` |
| `--web-bind` | `BOILER_MATE_WEB_BIND` | `false` |
| `--web-data` | `BOILER_MATE_WEB_DATA` | `boiler-mate-data` locally and `/data` in Docker |
| `--outdoor-weather-fahrenheit` | `BOILER_MATE_OUTDOOR_WEATHER_FAHRENHEIT` | `false` |

The MQTT prefix defaults to `nbe/<serial>`. Add a path to the broker URL to use a custom prefix. MQTT TLS uses `mqtts://` on port 8883 by default. Optional query parameters are `tls_ca` or `tls_cacert` for a private CA, and `tls_cert` plus `tls_key` for mutual TLS. The legacy `insecure=true` option explicitly disables certificate verification.

Metrics and process probes are served at `/metrics`, `/healthz`, and `/liveness`. Set `--bind=false` to disable that listener.

## Dashboard

The dashboard test branch publishes `ghcr.io/mlipscombe/boiler-mate:dashboard-test` alongside an immutable commit tag for each build. Use the test image in the boiler’s deployment when evaluating the dashboard.

```sh
./boiler-mate --web-bind 127.0.0.1:8080 --web-data ./boiler-mate-data
```

Open `http://127.0.0.1:8080`. Boiler Mate defaults to read-only, even when the controller URI contains a PIN. Set `BOILER_MATE_ENABLE_WRITES=true` to opt in to dashboard and MQTT writes. The controller still requires its PIN to accept commands.

Boiler Mate delegates access control to its reverse proxy. Protect the entire dashboard, including `/api/`, behind [Authentik forward auth](https://docs.goauthentik.io/add-secure-apps/providers/proxy/forward_auth) or equivalent, and keep the web listener private to that proxy. In a container, mount a persistent volume at `/data` owned by UID 10001 and set `BOILER_MATE_WEB_BIND=0.0.0.0:8080` to enable the listener.

The dashboard includes live telemetry, burner controls, controller settings and help, boiler and hot-water schedules, diagnostics, pellet consumption, signal charts with CSV export, and events. Settings and weekly schedules use the controller's reported ranges and clock. Chart history and events are recorded by this instance. The app reads historical pellet consumption directly from compatible controllers.

Set `BOILER_MATE_OUTDOOR_WEATHER_FAHRENHEIT=true` when the online outdoor weather feed reports Fahrenheit values, as some US installations do. Boiler Mate converts the feed's temperature to Celsius for the dashboard and its history.

Enable desktop notices under **Events → Desktop notifications** on each browser that should receive them. On browsers with Web Push, notices can arrive after the dashboard tab closes. The server needs outbound HTTPS to the browser's push service and sends encrypted notice payloads. The web data directory stores the push identity and subscriptions alongside dashboard history.

### Controller reads in the dashboard and MQTT

Open **Charts → Controller history** for chart IDs stored by the boiler, **Events → Controller log** for its event log, **Live data → Selected readings** for a one-time group of operating values, or **System → Firmware versions / Wireless sensors** for controller lists. Firmware and hardware determine which reads succeed. Chart IDs have controller-specific meanings.

Subscribe to `<prefix>/controller/+` for retained JSON results. Each result includes `read_at`, the URL-encoded `query`, and either `data` or `error`. To request a read, publish a non-retained message to `<prefix>/get/controller/<name>`. These reads work with controller writes disabled.

| Name | Request payload | Result topic under `<prefix>/controller/` |
| --- | --- | --- |
| `versions` | Empty | `versions` |
| `sensors` | Empty | `sensors` |
| `events` | Empty for recent events, or `at=1732442400` for a Unix timestamp | `events` or `event_history` for a dated request |
| `values` | `names=boiler_temp,state` | `values` |
| `chart` | `from=1732442200&to=1732442400&step=1&ids=0,3` | `chart` |

Chart bounds are Unix timestamps on the same controller-local day. Step is in minutes and IDs are in ascending order. The bridge refreshes firmware versions hourly and recent events and room sensors approximately once a minute. Chart and selected-value reads run on request. When a controller declines a compact selected-value read, the bridge extracts the requested fields from one fresh operating-data snapshot. Dashboard reads also publish their results to MQTT. Home Assistant discovery adds firmware-version and wireless-sensor counts with the full results in their attributes. A failed list read clears its count to unknown.

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

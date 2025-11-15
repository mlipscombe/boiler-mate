# boiler-mate

boiler-mate acts as a simple MQTT bridge from wood pellet boilers compatible with
NBE V7/10/13 controllers, used by various manufacturers.

## Supported Boilers

Any boiler with a controller that is managed through [https://stokercloud.dk]
should be compatible as long as it has the correct firmware firmware installed.

Possibly compatible manufacturers/models include:

- NBE Blackstar
- NBE RTB
- Kedel
- Opop

## Controller Firmware

If your boiler has a V7 or V10 controller, you need the firmware version that
removes the user interface on the touch screen, but enables access on the local
network.

To upgrade, go to [https://www.nbe.dk/program-download/] and select "Activate Controller
for App Operation" and follow the steps.

## How to Run

Synopsis:

```
    Usage of boiler-mate:
        --debug
            debug mode
        --bind string
            address to bind for healthz and prometheus metrics endpoint, or "false"
            to disable (default "0.0.0.0:2112")
        --controller string
            controller URI, in the format tcp://<serial>:<password>@<host>:<port>
        --mqtt string
            MQTT URI, in the format mqtt[s]://[<user>:<password>]@<host>:<port>[/<prefix>][?tls_cert=<cert_file>][&tls_key=<key_file>][&tls_ca=<ca_file>]
            (default "mqtt://localhost:1883")
```

Example:

```
    # Run with basic options
    boiler-mate --host udp://3629:0587451614@192.168.1.100:8483 --mqtt mqtt://10.10.11.20:1883

    # Run with TLS
    boiler-mate --host udp://3629:0587451614@192.168.1.100:8483 --mqtt mqtts://10.10.11.20:8883

    # Run with mutual TLS
    boiler-mate --host udp://3629:0587451614@192.168.1.100:8483 --mqtt mqtts://10.10.11.20:8883?tls_cert=client.crt&tls_key=client.key
```

Each command-line option can also be specified by an equivilent environment
variable, prefixed with `BOILER_MATE_`. For example, to set the MQTT URI to
`mqtt://mqtt:1833`, you can set the environment variable `BOILER_MATE_MQTT=mqtt://mqtt:1833`.

The boiler's password is required to write settings, but not to read them. You can
find controller's serial number and password in the top right corner of the display
on the unit.

If an MQTT prefix is not specified, messages will be published to the `nbe/<serial>`
topic.

## Development

### Building from Source

```bash
# Build binary
go build -o boiler-mate ./cmd/boiler-mate

# Or use Make
make binary
```

### Running Tests

```bash
# Run unit tests (fast)
go test ./...
make test

# Run integration tests (requires Docker)
make test-integration

# Run all tests with coverage
make test-coverage

# Run tests with race detection
make test-race
```

### Project Structure

```
boiler-mate/
├── cmd/boiler-mate/     # Main application
├── config/              # Configuration management
├── homeassistant/       # Home Assistant MQTT discovery
├── monitor/             # Data monitoring and publishing
├── mqtt/                # MQTT client wrapper
├── nbe/                 # NBE protocol implementation
└── test/integration/    # Integration tests
```

See [PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md) for detailed documentation.

## CI/CD

[![CI/CD](https://github.com/mlipscombe/boiler-mate/workflows/CI%2FCD/badge.svg)](https://github.com/mlipscombe/boiler-mate/actions)

The project uses GitHub Actions for continuous integration and deployment:

- **Unit Tests** - Run on every push and PR
- **Integration Tests** - Full system tests with real MQTT broker
- **Build Verification** - Ensures code compiles
- **Linting** - Code quality checks
- **Docker Multi-Arch Builds** - Automatic builds for amd64 and arm64

See [.github/workflows/README.md](.github/workflows/README.md) for pipeline details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test-all`
5. Submit a pull request

All PRs must pass CI/CD checks before merging.

## Thanks & Acknowledgement

Special thanks to [Anders Nylund](https://github.com/motoz) for documenting the
NBE protocol.

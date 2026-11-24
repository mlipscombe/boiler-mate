#!/bin/bash
set -euo pipefail

compose=(docker compose -f docker-compose.test.yml)
if [[ -z "${TEST_MQTT_URL:-}" ]]; then
    trap '"${compose[@]}" down' EXIT
    "${compose[@]}" up -d --wait
fi

INTEGRATION_TESTS=1 go test -race -timeout 45s ./test/integration

#!/bin/bash
set -e

echo "Starting integration test environment..."

# Start Docker Compose services
docker compose -f docker-compose.test.yml up -d

# Wait for MQTT broker to be healthy
echo "Waiting for MQTT broker to be ready..."
timeout=30
elapsed=0
while ! docker exec boiler-mate-test-mqtt mosquitto_sub -t '$SYS/#' -C 1 -W 1 > /dev/null 2>&1; do
    if [ $elapsed -ge $timeout ]; then
        echo "Timeout waiting for MQTT broker"
        docker compose -f docker-compose.test.yml logs
        docker compose -f docker-compose.test.yml down
        exit 1
    fi
    sleep 1
    elapsed=$((elapsed + 1))
done

echo "MQTT broker is ready!"

# Run integration tests
echo "Running integration tests..."
INTEGRATION_TESTS=1 go test -v -timeout 30s ./test/integration

# Capture test exit code
TEST_EXIT_CODE=$?

# Cleanup
echo "Cleaning up test environment..."
docker compose -f docker-compose.test.yml down

exit $TEST_EXIT_CODE

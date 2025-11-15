#!/bin/bash
set -e

echo "Starting integration test environment..."

# Start Docker Compose services and wait for health checks
docker compose -f docker-compose.test.yml up -d --wait

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

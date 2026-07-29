#!/usr/bin/env bash
#
# Docker-based integration test for Squid ACL behavior.
# Validates that the deny-first ACL rules (matching generateSquidConfig in
# internal/ecs/deploy.go) correctly block unsafe ports, localhost, link-local,
# RFC 1918, and cloud metadata endpoints while allowing safe HTTPS CONNECT.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="agent-proxy-squid-test"
CONTAINER_NAME="squid-acl-test-$$"

cleanup() {
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== Building Docker image ==="
docker build -t "$IMAGE_NAME" "$SCRIPT_DIR"

echo ""
echo "=== Validating tunnel config (squid -k parse) ==="
docker run --rm "$IMAGE_NAME" squid -k parse -f /etc/squid/squid-tunnel.conf

echo ""
echo "=== Validating direct config (squid -k parse) ==="
docker run --rm "$IMAGE_NAME" squid -k parse -f /etc/squid/squid-direct.conf

echo ""
echo "=== Starting squid (tunnel config) ==="
docker run -d --name "$CONTAINER_NAME" "$IMAGE_NAME" squid -N -f /etc/squid/squid-tunnel.conf

# Wait for squid to accept connections
for i in $(seq 1 30); do
    if docker exec "$CONTAINER_NAME" curl -s --max-time 2 -o /dev/null -x http://127.0.0.1:3128 http://example.com/ 2>/dev/null; then
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "FAIL: squid did not start within 30 seconds"
        docker logs "$CONTAINER_NAME"
        exit 1
    fi
    sleep 1
done
echo "squid is ready"

echo ""
docker exec "$CONTAINER_NAME" /usr/local/bin/run-acl-tests.sh
rc=$?

if [ "$rc" -ne 0 ]; then
    echo ""
    echo "=== Squid access log ==="
    docker exec "$CONTAINER_NAME" cat /var/log/squid/access.log 2>/dev/null || true
    exit 1
fi

echo "All tests passed!"

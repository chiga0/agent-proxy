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
    if docker exec "$CONTAINER_NAME" curl -s --max-time 1 -x http://127.0.0.1:3128 -o /dev/null 2>/dev/null; then
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

PASS=0
FAIL=0

assert_status() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$actual" = "$expected" ]; then
        echo "  PASS: $desc (HTTP $actual)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc (expected HTTP $expected, got HTTP $actual)"
        FAIL=$((FAIL + 1))
    fi
}

echo ""
echo "=== Running ACL tests ==="

# 1. CONNECT to port 443 should succeed (Safe_ports + SSL_ports allow)
code=$(docker exec "$CONTAINER_NAME" curl -s --max-time 10 -o /dev/null -w '%{http_code}' \
    -x http://127.0.0.1:3128 -p https://example.com/ 2>/dev/null || echo "000")
assert_status "CONNECT to port 443 allowed" "200" "$code"

# 2. CONNECT to port 22 should fail (not in Safe_ports)
code=$(docker exec "$CONTAINER_NAME" curl -s --max-time 10 -o /dev/null -w '%{http_code}' \
    -x http://127.0.0.1:3128 -p https://example.com:22/ 2>/dev/null || echo "000")
assert_status "CONNECT to port 22 denied" "403" "$code"

# 3. CONNECT to 127.0.0.1 should fail (to_localhost)
code=$(docker exec "$CONTAINER_NAME" curl -s --max-time 10 -o /dev/null -w '%{http_code}' \
    -x http://127.0.0.1:3128 -p https://127.0.0.1:443/ 2>/dev/null || echo "000")
assert_status "CONNECT to 127.0.0.1 denied" "403" "$code"

# 4. CONNECT to 169.254.169.254 should fail (to_linklocal + to_metadata)
code=$(docker exec "$CONTAINER_NAME" curl -s --max-time 10 -o /dev/null -w '%{http_code}' \
    -x http://127.0.0.1:3128 -p https://169.254.169.254/ 2>/dev/null || echo "000")
assert_status "CONNECT to 169.254.169.254 denied" "403" "$code"

# 5. CONNECT to 100.100.100.200 should fail (to_metadata)
code=$(docker exec "$CONTAINER_NAME" curl -s --max-time 10 -o /dev/null -w '%{http_code}' \
    -x http://127.0.0.1:3128 -p https://100.100.100.200/ 2>/dev/null || echo "000")
assert_status "CONNECT to 100.100.100.200 denied" "403" "$code"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
    echo ""
    echo "=== Squid access log ==="
    docker exec "$CONTAINER_NAME" cat /var/log/squid/access.log 2>/dev/null || true
    exit 1
fi

echo "All tests passed!"

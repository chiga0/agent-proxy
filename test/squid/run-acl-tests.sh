#!/bin/bash
# Runs inside the Docker container. Tests Squid ACL behavior via curl.
#
# For HTTPS (CONNECT) requests, curl's %{http_code} reflects the final
# end-to-end HTTP response, NOT the proxy's CONNECT response. A denied
# CONNECT returns 403 from the proxy but curl reports 000. So we parse
# the proxy's CONNECT response from curl -v stderr instead.
set -u

PROXY="127.0.0.1:3128"
PASS=0
FAIL=0

# assert_http: test a plain HTTP request through the proxy (no CONNECT tunnel)
assert_http() {
    local desc="$1" expected="$2" url="$3"
    local code
    code=$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' -x "http://$PROXY" "$url" 2>/dev/null)
    if [ "$code" = "$expected" ]; then
        echo "  PASS: $desc (HTTP $code)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc (expected HTTP $expected, got HTTP $code)"
        FAIL=$((FAIL + 1))
    fi
}

# assert_connect: test an HTTPS CONNECT request through the proxy
# Parses the proxy's "HTTP/1.x NNN" response from curl verbose output.
assert_connect() {
    local desc="$1" expected="$2" url="$3"
    local verbose
    verbose=$(curl -sv --max-time 10 -o /dev/null -x "http://$PROXY" "$url" 2>&1 || true)
    local proxy_code
    proxy_code=$(echo "$verbose" | sed -n 's/^< HTTP\/1\.[01] \([0-9]*\).*/\1/p' | head -1)
    if [ "$proxy_code" = "$expected" ]; then
        echo "  PASS: $desc (CONNECT $proxy_code)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $desc (expected CONNECT $expected, got CONNECT ${proxy_code:-none})"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Running ACL tests ==="

# 1. HTTP GET on port 80 should succeed (Safe_ports allows 80, trusted_ip allows 127.0.0.1)
assert_http "GET port 80 allowed" "200" "http://example.com/"

# 2. CONNECT to port 443 should succeed (Safe_ports + SSL_ports allow)
assert_connect "CONNECT to port 443 allowed" "200" "https://example.com/"

# 3. CONNECT to port 22 should fail (not in Safe_ports)
assert_connect "CONNECT to port 22 denied" "403" "https://example.com:22/"

# 4. CONNECT to 127.0.0.1 should fail (to_localhost)
assert_connect "CONNECT to 127.0.0.1 denied" "403" "https://127.0.0.1:443/"

# 5. CONNECT to 169.254.169.254 should fail (to_linklocal + to_metadata)
assert_connect "CONNECT to 169.254.169.254 denied" "403" "https://169.254.169.254/"

# 6. CONNECT to 100.100.100.200 should fail (to_metadata)
assert_connect "CONNECT to 100.100.100.200 denied" "403" "https://100.100.100.200/"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
exit "$FAIL"

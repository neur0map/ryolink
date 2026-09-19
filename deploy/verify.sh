#!/usr/bin/env bash
# Usage: bash deploy/verify.sh [hostname]
# Checks that a ryolink deployment is healthy.
set -euo pipefail

HOST="${1:-ryoku.dev}"
OK=0
FAIL=0

pass() { echo "  [ok]  $*"; ((OK++)) || true; }
fail() { echo "  [!!]  $*"; ((FAIL++)) || true; }

echo "=== ryolink deployment check: $HOST ==="
echo

# 1. SSH port 22 responds with an SSH banner (the ryolink server)
echo "--- SSH port 22 (ryolink) ---"
BANNER=$(timeout 5 bash -c "exec 3<>/dev/tcp/${HOST}/22 && head -c 40 <&3" 2>/dev/null || true)
if echo "$BANNER" | grep -qi "SSH"; then
  pass "Port 22 returns SSH banner"
else
  fail "Port 22 did not return SSH banner (ryolink server may be down)"
fi

# 2. Store landing page over HTTPS (served by ryolink through Caddy)
echo "--- HTTPS store front ---"
if curl -fsSL --max-time 8 "https://${HOST}/" 2>/dev/null | grep -qi "ryoku"; then
  pass "https://${HOST}/ serves the ryoku store front"
else
  fail "https://${HOST}/ unreachable or not the store page (Caddy or store.enabled?)"
fi

# 3. Catalog API
echo "--- /api/items ---"
if curl -fsSL --max-time 8 "https://${HOST}/api/items" 2>/dev/null | grep -q '"items"'; then
  pass "catalog API responds"
else
  fail "/api/items did not answer (store disabled?)"
fi

# 4. TLS certificate
echo "--- TLS certificate ---"
EXPIRY=$(echo | openssl s_client -servername "$HOST" -connect "${HOST}:443" 2>/dev/null \
  | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
if [ -n "$EXPIRY" ]; then
  pass "TLS cert valid through: $EXPIRY"
else
  fail "Could not retrieve TLS certificate"
fi

# 5. Radio endpoints (present but quiet when web_audio is off — not a failure)
echo "--- radio /now-playing ---"
if curl -fsSL --max-time 5 "https://${HOST}/now-playing" 2>/dev/null | grep -q '"playing"'; then
  pass "/now-playing responds"
else
  echo "  [--]  /now-playing not answering (web_audio off — not an error)"
fi

echo
echo "=== Results: ${OK} passed, ${FAIL} failed ==="
[ "$FAIL" -eq 0 ]

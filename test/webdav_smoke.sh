#!/usr/bin/env bash
# End-to-end smoke test for the gpix WebDAV gateway using curl.
#
# Start a server first (no Google credentials needed):
#   go run ./cmd/gpix-gateway-test -dav 127.0.0.1:8081 -user gpix -pass gpix
#
# Then run:
#   DAV_URL=http://127.0.0.1:8081 DAV_USER=gpix DAV_PASS=gpix ./test/webdav_smoke.sh
set -u

DAV_URL="${DAV_URL:-http://127.0.0.1:8081}"
DAV_USER="${DAV_USER:-gpix}"
DAV_PASS="${DAV_PASS:-gpix}"
KEY="gpix-smoke-$$.txt"
TMP="$(mktemp)"
OUT="$(mktemp)"
fail=0

note() { printf '[%s] %s\n' "$1" "$2"; }
check() { if [ "$1" = "0" ]; then note "ok " "$2"; else note "FAIL" "$2"; fail=$((fail+1)); fi; }

printf 'hello from the gpix webdav gateway\n%.0s' {1..50} > "$TMP"

# OPTIONS advertises DAV
code=$(curl -s -o /dev/null -w '%{http_code}' -u "$DAV_USER:$DAV_PASS" -X OPTIONS "$DAV_URL/")
[ "$code" = "200" ]; check $? "OPTIONS -> 200 (got $code)"

# Unauthorized is rejected
code=$(curl -s -o /dev/null -w '%{http_code}' -X PROPFIND -H 'Depth: 0' "$DAV_URL/")
[ "$code" = "401" ]; check $? "PROPFIND without auth -> 401 (got $code)"

# PUT
code=$(curl -s -o /dev/null -w '%{http_code}' -u "$DAV_USER:$DAV_PASS" -T "$TMP" "$DAV_URL/$KEY")
{ [ "$code" = "201" ] || [ "$code" = "204" ]; }; check $? "PUT -> 201/204 (got $code)"

# PROPFIND lists the new file
body=$(curl -s -u "$DAV_USER:$DAV_PASS" -X PROPFIND -H 'Depth: 1' "$DAV_URL/")
echo "$body" | grep -q "$KEY"; check $? "PROPFIND lists the uploaded file"

# GET round-trips
curl -s -u "$DAV_USER:$DAV_PASS" "$DAV_URL/$KEY" -o "$OUT"
cmp -s "$TMP" "$OUT"; check $? "GET round-trips the bytes"

# DELETE
code=$(curl -s -o /dev/null -w '%{http_code}' -u "$DAV_USER:$DAV_PASS" -X DELETE "$DAV_URL/$KEY")
{ [ "$code" = "204" ] || [ "$code" = "200" ]; }; check $? "DELETE -> 204/200 (got $code)"

# Gone afterwards
code=$(curl -s -o /dev/null -w '%{http_code}' -u "$DAV_USER:$DAV_PASS" "$DAV_URL/$KEY")
[ "$code" = "404" ]; check $? "GET after delete -> 404 (got $code)"

rm -f "$TMP" "$OUT"
echo "----------------------------------------"
if [ "$fail" -ne 0 ]; then echo "$fail check(s) FAILED"; exit 1; fi
echo "all checks passed ✓"

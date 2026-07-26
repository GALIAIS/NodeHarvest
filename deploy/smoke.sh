#!/bin/bash
set -euo pipefail

BASE_URL="${BASE_URL:?set BASE_URL}"
BASE_URL="${BASE_URL%/}"
HEADER=()
if [ -n "${SUB_TOKEN:-}" ]; then HEADER=(-H "Authorization: Bearer $SUB_TOKEN"); fi

curl --fail --silent --show-error "$BASE_URL/api/health" >/dev/null
curl --fail --silent --show-error "$BASE_URL/api/ready" >/dev/null
curl --fail --silent --show-error "$BASE_URL/api/version" >/dev/null
curl --fail --silent --show-error "${HEADER[@]}" "$BASE_URL/sub/meta" >/dev/null
curl --fail --silent --show-error "${HEADER[@]}" "$BASE_URL/sub/base64" >/dev/null
echo "smoke passed: $BASE_URL"

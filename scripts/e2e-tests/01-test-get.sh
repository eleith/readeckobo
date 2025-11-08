#!/bin/sh

if [ -z "$1" ]; then
  echo "Usage: $0 <USER_TOKEN>"
  exit 1
fi

USER_TOKEN="$1"
BASE_URL="http://localhost:8080"

echo "Testing: POST /api/kobo/get"
curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{"access_token": "'"$USER_TOKEN"'"}' \
  "$BASE_URL/api/kobo/get"
echo "\n"

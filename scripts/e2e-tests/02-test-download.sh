#!/bin/sh

if [ -z "$1" ] || [ -z "$2" ]; then
  echo "Usage: $0 <USER_TOKEN> <ARTICLE_URL>"
  exit 1
fi

USER_TOKEN="$1"
ARTICLE_URL="$2"
BASE_URL="http://localhost:8080"

echo "Testing: POST /api/kobo/download"
curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{"access_token": "'$USER_TOKEN'", "url": "'$ARTICLE_URL'"}' \
  "$BASE_URL/api/kobo/download"
echo "\n"

#!/bin/sh

if [ -z "$1" ]; then
  echo "Usage: $0 <IMAGE_URL>"
  exit 1
fi

IMAGE_URL="$1"
BASE_URL="http://localhost:8080"

echo "Testing: GET /api/convert-image"
curl -s -D - "$BASE_URL/api/convert-image?url=$IMAGE_URL" -o /dev/null
echo "\n"


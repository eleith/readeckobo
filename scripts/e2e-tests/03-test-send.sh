#!/bin/sh

if [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
  echo "Usage: $0 <USER_TOKEN> <ACTION> <ITEM_ID_OR_URL>"
  echo "Actions: archive, readd, favorite, unfavorite, delete, add"
  exit 1
fi

USER_TOKEN="$1"
ACTION="$2"
ITEM_OR_URL="$3"
BASE_URL="http://localhost:8080"

PAYLOAD=""
if [ "$ACTION" = "add" ]; then
  PAYLOAD='{"access_token": "'$USER_TOKEN'", "actions": [{"action": "'$ACTION'", "url": "'$ITEM_OR_URL'"}]}'
else
  PAYLOAD='{"access_token": "'$USER_TOKEN'", "actions": [{"action": "'$ACTION'", "item_id": "'$ITEM_OR_URL'"}]}'
fi

echo "Testing: POST /api/kobo/send (Action: $ACTION)"
curl -s -X POST \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  "$BASE_URL/api/kobo/send"
echo "\n"

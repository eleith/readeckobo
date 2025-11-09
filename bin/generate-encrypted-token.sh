#!/bin/sh

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <YOUR_KOBO_SERIAL>" >&2
    echo "Find your Kobo serial number under Settings -> Device Information." >&2
    exit 1
fi

KOBO_SERIAL="$1"
# Generate a new UUID for the device
DEVICE_TOKEN=$(uuidgen)
STATIC_SALT="88b3a2e13"

# --- Encryption Logic (derived from kobeck) ---
KEY=$(echo -n "${STATIC_SALT}${KOBO_SERIAL}" | sha256sum | cut -b -16)
HEX_KEY=$(echo -n "${KEY}" | xxd -p -l 16)
ENCRYPTED_TOKEN=$(echo -n "$DEVICE_TOKEN" | openssl enc -aes-128-ecb -base64 -A -K "$HEX_KEY" -nosalt)

# --- Output Instructions ---
echo ""
echo "✅ New token generated successfully."
echo "----------------------------------------------------------------"

echo ""

echo "1. Add the PLAIN TEXT token to your readeckobo 'config.yaml' file:"

echo ""

echo "users:"
echo "  - token: \"$DEVICE_TOKEN\""
echo "    readeck_access_token: \"<YOUR-READECK-API-TOKEN>\""

echo ""

echo "----------------------------------------------------------------"

echo ""

echo "2. Add the ENCRYPTED token to your Kobo's '.kobo/Kobo/Kobo eReader.conf' file:"

echo ""

echo "[Instapaper]"
echo "AccessToken=@ByteArray($ENCRYPTED_TOKEN)"

echo ""

echo "----------------------------------------------------------------"

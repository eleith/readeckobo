#!/bin/sh

# This script generates a comma-separated list of Go packages to include in coverage.
# It excludes packages matching patterns provided as arguments.

EXCLUDE_PATTERNS=""

# Add any additional exclusion patterns from arguments
for arg in "$@"; do
  EXCLUDE_PATTERNS="${EXCLUDE_PATTERNS} ${arg}"
done

PACKAGE_LIST=$(go list ./...)

for pattern in ${EXCLUDE_PATTERNS}; do
  PACKAGE_LIST=$(echo "${PACKAGE_LIST}" | grep -v "${pattern}")
done

echo "${PACKAGE_LIST}" | paste -sd, -
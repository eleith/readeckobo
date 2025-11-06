#!/bin/sh

set -eu

# Default to package view, but allow function view
MODE="packages"
if [ "${1:-}" = "--mode=functions" ]; then
  MODE="functions"
fi

# Always get the coverage threshold from the last argument
COVERAGE_THRESHOLD=$(echo "$@" | awk '{print $NF}')

# --- Test Execution ---
EXCLUDED_PACKAGES="readeckobo/internal/testutil readeckobo/scripts"
COVERAGE_PACKAGES_COMMA=$(./scripts/list-packages.sh $EXCLUDED_PACKAGES)
COVERAGE_PACKAGES_SPACE=$(echo $COVERAGE_PACKAGES_COMMA | tr ',' ' ')

# Run tests once to generate the coverage.out file
go test -mod=vendor -coverprofile=coverage.out -coverpkg=$COVERAGE_PACKAGES_COMMA $COVERAGE_PACKAGES_SPACE > /dev/null 2>&1

# --- Reporting ---
echo "Test Coverage Report"
echo "--------------------"

if [ "$MODE" = "functions" ]; then
  # Detailed, function-by-function view
  go tool cover -func=coverage.out | grep -v -E "$(echo $EXCLUDED_PACKAGES | tr ' ' '|')"
else
  # Clean, package-by-package view
  go test -mod=vendor -coverpkg=$COVERAGE_PACKAGES_COMMA $COVERAGE_PACKAGES_SPACE | sed 's/ in .*//'
fi

# --- Threshold Check ---
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')

echo "\n--------------------"
echo "Total test coverage: $COVERAGE%"

if awk -v cov="$COVERAGE" -v min="$COVERAGE_THRESHOLD" 'BEGIN {exit (cov < min)}'; then
    echo "Success: Test coverage is at or above the ${COVERAGE_THRESHOLD}% threshold."
else
    echo "Error: Test coverage is below the ${COVERAGE_THRESHOLD}% threshold."
    exit 1
fi

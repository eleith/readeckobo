# Build the application
build:
	go build -o readeckobo ./cmd/readeckobo

# Run tests
test:
	go test ./...

test-coverage:
	./scripts/check-coverage.sh 60

test-coverage-full:
	./scripts/check-coverage.sh --mode=functions 60

# Run linter
lint:
	golangci-lint run

# Tidy and vendor dependencies
vendor:
	go mod tidy
	go mod vendor

# Run all checks
ci: lint test

# Default target
all: build

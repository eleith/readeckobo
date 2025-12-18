# Agents.md

## Project Context
**readeckobo** is a proxy server written in Go. Its primary purpose is to act as a bridge between **Kobo e-readers** and a self-hosted **Readeck** instance.

Kobo e-readers have built-in support for **Instapaper**. This application emulates the Instapaper API so that Kobo devices can sync articles from Readeck seamlessly, without requiring custom firmware on the device.

## Architecture Overview
The project follows a standard Go project layout:

- **`cmd/readeckobo/`**: Entry point (`main.go`). Initializes configuration, logger, and starts the server.
- **`internal/app/`**: Core business logic. Contains handlers for the Kobo (Instapaper) API endpoints (`/api/1/oauth/access_token`, `/api/1/bookmarks/list`, etc.) and image conversion logic.
- **`internal/readeck/`**: Client implementation for communicating with the Readeck API. Handles authentication and data fetching.
- **`internal/webserver/`**: HTTP server setup, routing, and middleware.
- **`internal/config/`**: Configuration management using `koanf`.
- **`internal/models/`**: Shared data structures for Kobo and Readeck API payloads.

## Deployment & Infrastructure
- **Proxy Interception**: The system relies on a reverse proxy (like Nginx) to intercept requests from the Kobo device intended for `www.instapaper.com` and redirect them to this service.
- **Configuration**: See `nginx.conf.snippet` for the rewrite rules.
- **Docker**: The application is containerized (`Dockerfile`, `docker-compose.yml`).

## Development Guidelines for AI Agents

### 1. Code Style & Conventions
- Follow standard Go idioms (Effective Go).
- **No external mock libraries**: Use manual mocking strategies (e.g., `MockRoundTripper` in `internal/app/app_test.go`) rather than introducing dependencies like `mockgen` or `testify` unless explicitly requested.
- Use **Table-Driven Tests** for unit testing logic.

### 2. Testing Protocol
- **Unit Tests**: Required for all logic changes.
  - Location: `*_test.go` files next to the code.
  - pattern: Mock external HTTP calls to Readeck or Kobo to ensure isolation.
- **E2E Tests**: `scripts/e2e-tests/` contains shell scripts for manual human verification.
  - Agents should be aware of these flows but are not expected to run them autonomously unless the environment is fully configured (which is rare in a CLI session).
  - Agents *should* maintain the validity of these scripts if endpoint signatures change.

### 3. Key Dependencies & Logic
- **Image Conversion**: Kobo devices often require specific image formats (JPEG, grayscale optimizations). This logic resides in `internal/app`.
- **API Emulation**: The application strictly adheres to the Instapaper API format expected by Kobo devices.
  - *Constraint*: Do not change the JSON structure of responses sent to Kobo, as the device firmware is brittle.

### 4. Refactoring & Maintenance
- When refactoring, ensure strict separation between the `webserver` (transport layer) and `app` (business logic).
- Configuration changes in `config.yaml` or `internal/config` must be reflected in `config.yaml.example`.

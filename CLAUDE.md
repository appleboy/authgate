# CLAUDE.md

This file provides guidance to Claude Code (<https://claude.ai/code>) when working with code in this repository.

## Project Overview

AuthGate is an OAuth 2.0 Device Authorization Grant (RFC 8628) server built with Go and Gin. It enables CLI tools to authenticate users without embedding client secrets.

## Common Commands

```bash
# Build
make build              # Build to bin/authgate
go build -o authgate .  # Quick build

# Run
./authgate              # Starts on :8080

# Test & Lint
make test               # Run tests with coverage
make lint               # Run golangci-lint
make fmt                # Format code

# Cross-compile
make build_linux_amd64
make build_linux_arm64
```

## Architecture

**Flow**: CLI requests device code → User visits /device, logs in, enters code → CLI polls /oauth/token → Gets JWT

**Layers**:

- `handlers/` - HTTP handlers (auth, device, token)
- `services/` - Business logic (user auth, device code generation, JWT creation)
- `store/` - SQLite/GORM data access
- `models/` - Data structures (User, OAuthClient, DeviceCode, AccessToken)
- `middleware/` - Session-based auth middleware

**Key endpoints**:

- `POST /oauth/device/code` - CLI requests device+user codes
- `POST /oauth/token` - CLI polls for JWT (grant_type=urn:ietf:params:oauth:grant-type:device_code)
- `GET /device` - User enters code (requires login)

## Environment Variables

| Variable       | Default                 | Description                     |
| -------------- | ----------------------- | ------------------------------- |
| SERVER_ADDR    | :8080                   | Listen address                  |
| BASE_URL       | `http://localhost:8080` | Public URL for verification_uri |
| JWT_SECRET     | (default)               | JWT signing key                 |
| SESSION_SECRET | (default)               | Cookie encryption key           |
| DATABASE_PATH  | oauth.db                | SQLite database path            |

## Default Test Data

- User: `admin` / `password123`
- Client: `AuthGate CLI` (client_id is UUID, shown in server startup logs)

## Example CLI Client

`_example/authgate-cli/` contains a demo CLI that demonstrates the device flow:

```bash
cd _example/authgate-cli && go run main.go
```

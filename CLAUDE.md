# CLAUDE.md

This file provides guidance to [Claude Code](https://claude.ai/code) when working with code in this repository.

## Project Overview

AuthGate is an OAuth 2.0 Device Authorization Grant (RFC 8628) server built with Go and Gin. It enables CLI tools to authenticate users without embedding client secrets.

## Common Commands

```bash
# Build
make build              # Build to bin/authgate with version info in LDFLAGS

# Run
./bin/authgate -v       # Show version information
./bin/authgate -h       # Show help
./bin/authgate server   # Start the OAuth server

# Test & Lint
make test               # Run tests with coverage report (outputs coverage.txt)
make lint               # Run golangci-lint (auto-installs if missing)
make fmt                # Format code with golangci-lint fmt

# Cross-compile (outputs to release/<os>/<arch>/)
make build_linux_amd64  # CGO_ENABLED=0 for static binary
make build_linux_arm64  # CGO_ENABLED=0 for static binary

# Clean
make clean              # Remove bin/, release/, coverage.txt

# Docker
docker build -f docker/Dockerfile -t authgate .
```

## Architecture

**Device Authorization Flow**:

1. CLI calls `POST /oauth/device/code` with client_id → receives device_code + user_code + verification_uri
2. User visits verification_uri (`/device`) in browser, must login first if not authenticated
3. User submits user_code via `POST /device/verify` → device code marked as authorized
4. CLI polls `POST /oauth/token` with device_code every 5s → receives JWT when authorized

**Layers** (dependency injection pattern):

- `main.go` - Wires up store → auth providers → token providers → services → handlers, configures Gin router with session middleware
- `config/` - Loads .env via godotenv, provides Config struct with defaults
- `store/` - GORM-based data access layer, supports SQLite and PostgreSQL via driver factory pattern
  - `driver.go` - Database driver factory using map-based pattern (no if-else)
  - `sqlite.go` - Store implementation and database operations (driver-agnostic)
- `auth/` - Authentication providers (LocalAuthProvider, HTTPAPIAuthProvider) with pluggable design
- `token/` - Token providers (LocalTokenProvider, HTTPTokenProvider) with pluggable design
  - `types.go` - Shared data structures (TokenResult, TokenValidationResult)
  - `errors.go` - Provider-level error definitions
  - `local.go` - Local JWT provider (HMAC-SHA256)
  - `http_api.go` - External HTTP API token provider
- `services/` - Business logic (UserService, DeviceService, TokenService), depends on Store, Auth providers, and Token providers
- `handlers/` - HTTP handlers (AuthHandler, DeviceHandler, TokenHandler), depends on Services
- `models/` - GORM models (User, OAuthClient, DeviceCode, AccessToken)
- `middleware/` - Gin middleware (auth.go: RequireAuth checks session for user_id)

**Authentication Architecture**:

- **Pluggable Providers**: Supports local (database) and external HTTP API authentication
- **Hybrid Mode**: Each user authenticates based on their `auth_source` field
- **Auth Mode**: Configured via `AUTH_MODE` env var (`local` or `http_api`), defaults to `local`
- **User Sync**: External auth automatically creates/updates users in local database
- **No Interfaces**: Direct struct dependency injection (project convention)
- **Authentication Flow**:
  1. UserService looks up user by username
  2. If user exists: route to provider based on user's `auth_source` field
     - `auth_source=local`: LocalAuthProvider (bcrypt against database)
     - `auth_source=http_api`: HTTPAPIAuthProvider (call external API)
  3. If user doesn't exist and `AUTH_MODE=http_api`: try external auth and create user
  4. Default admin user always uses local authentication (failsafe)
- **User Fields**: ExternalID, AuthSource, Email, FullName added for external auth support
- **Key Benefit**: Admin can always login locally even if external service is down

**Token Provider Architecture**:

- **Pluggable Providers**: Supports local JWT generation/validation and external HTTP API token services
- **Global Mode**: Configured via `TOKEN_PROVIDER_MODE` env var (`local` or `http_api`), defaults to `local`
- **Local Storage**: Token records always stored in local database for management (revocation, listing, auditing)
- **No Interfaces**: Direct struct dependency injection (project convention, following auth provider pattern)
- **Token Generation Flow**:
  1. TokenService receives token generation request (from ExchangeDeviceCode)
  2. Selects provider based on `TOKEN_PROVIDER_MODE` configuration
     - `local`: LocalTokenProvider generates JWT with HMAC-SHA256 using `JWT_SECRET`
     - `http_api`: HTTPTokenProvider calls external API to generate JWT
  3. Saves token record to local database (regardless of provider)
  4. Returns AccessToken to client
- **Token Validation Flow**:
  1. TokenService receives validation request (from TokenInfo endpoint)
  2. Selects provider based on `TOKEN_PROVIDER_MODE` configuration
     - `local`: LocalTokenProvider validates JWT signature with `JWT_SECRET`
     - `http_api`: HTTPTokenProvider calls external API to validate JWT
  3. Returns TokenValidationResult with claims
- **Provider Types**:
  - `LocalTokenProvider`: Uses golang-jwt/jwt library for HMAC-SHA256 signing
  - `HTTPTokenProvider`: Delegates to external HTTP API, supports custom signing algorithms (RS256, ES256, etc.)
- **API Contract**: HTTPTokenProvider expects `/generate` and `/validate` endpoints with specific JSON format
- **Key Benefit**: Centralized token services, advanced key management, compliance requirements while maintaining local token management

**Key Implementation Details**:

- Device codes expire after 30min (configurable via Config.DeviceCodeExpiration)
- User codes are 8-char uppercase alphanumeric (generated by generateUserCode in services/device.go)
- User codes normalized: uppercase + dashes removed before lookup
- JWTs signed with HMAC-SHA256, expire after 1 hour (Config.JWTExpiration)
- Sessions stored in encrypted cookies (gin-contrib/sessions), 7-day expiry
- Polling interval is 5 seconds (Config.PollingInterval)
- Templates and static files embedded via go:embed in main.go

**Key Endpoints**:

- `GET /health` - Health check with database connection test
- `POST /oauth/device/code` - CLI requests device+user codes (accepts form or JSON)
- `POST /oauth/token` - CLI polls for JWT (grant_type=urn:ietf:params:oauth:grant-type:device_code)
- `GET /oauth/tokeninfo` - Verify JWT validity
- `GET /device` - User authorization page (protected, requires login)
- `POST /device/verify` - User submits code to authorize device (protected)
- `GET|POST /login` - User authentication
- `GET /logout` - Clear session

**Error Handling**: Services return typed errors (ErrInvalidClient, ErrDeviceCodeNotFound, etc.), handlers convert to RFC 8628 OAuth error responses

## Environment Variables

| Variable                      | Default                 | Description                                                  |
| ----------------------------- | ----------------------- | ------------------------------------------------------------ |
| SERVER_ADDR                   | :8080                   | Listen address                                               |
| BASE_URL                      | `http://localhost:8080` | Public URL for verification_uri                              |
| JWT_SECRET                    | (default)               | JWT signing key (used when TOKEN_PROVIDER_MODE=local)        |
| SESSION_SECRET                | (default)               | Cookie encryption key                                        |
| DATABASE_DRIVER               | sqlite                  | Database driver ("sqlite" or "postgres")                     |
| DATABASE_DSN                  | oauth.db                | Connection string (path for SQLite, DSN for PostgreSQL)      |
| **AUTH_MODE**                 | local                   | Authentication mode: `local` or `http_api`                   |
| HTTP_API_URL                  | (none)                  | External auth API endpoint (required when AUTH_MODE=http_api)|
| HTTP_API_TIMEOUT              | 10s                     | HTTP API request timeout                                     |
| HTTP_API_INSECURE_SKIP_VERIFY | false                   | Skip TLS verification (dev/testing only)                     |
| **TOKEN_PROVIDER_MODE**       | local                   | Token provider mode: `local` or `http_api`                   |
| TOKEN_API_URL                 | (none)                  | External token API endpoint (required when TOKEN_PROVIDER_MODE=http_api) |
| TOKEN_API_TIMEOUT             | 10s                     | Token API request timeout                                    |
| TOKEN_API_INSECURE_SKIP_VERIFY| false                   | Skip TLS verification for token API (dev/testing only)       |

## Default Test Data

Seeded automatically on first run (store/sqlite.go:seedData):

- User: `admin` / `<random_password>` (16-character random password, logged at startup, bcrypt hashed)
- Client: `AuthGate CLI` (client_id is auto-generated UUID, logged at startup)

## Example CLI Client

`_example/authgate-cli/` contains a demo CLI that demonstrates the device flow:

```bash
cd _example/authgate-cli
cp .env.example .env      # Add CLIENT_ID from server logs
go run main.go
```

## External Authentication Configuration

### HTTP API Authentication

To use external HTTP API for authentication, configure these environment variables:

```bash
AUTH_MODE=http_api
HTTP_API_URL=https://your-auth-api.com/verify
HTTP_API_TIMEOUT=10s
HTTP_API_INSECURE_SKIP_VERIFY=false
```

**Expected API Contract:**

Request (POST to HTTP_API_URL):

```json
{
  "username": "john",
  "password": "secret123"
}
```

Response:

```json
{
  "success": true,
  "user_id": "external-user-id",
  "email": "john@example.com",
  "full_name": "John Doe"
}
```

**Response Requirements:**

- `success` (required): Boolean indicating authentication result
- `user_id` (required when success=true): Non-empty string uniquely identifying the user in external system
- `email` (optional): User's email address
- `full_name` (optional): User's display name
- `message` (optional): Error message when success=false or HTTP status is non-2xx

**Behavior:**

- First login auto-creates user in local database with `auth_source="http_api"`
- Subsequent logins update user info (email, full_name)
- Users get default "user" role (admins must be promoted manually)
- External users stored with `auth_source="http_api"` and `external_id` set
- Each user authenticates based on their own `auth_source` field (hybrid mode)
- Default admin user (`auth_source="local"`) can always login even if external API is down
- Missing or empty `user_id` when `success=true` will cause authentication to fail
- **Username conflicts**: If external username matches existing user, login fails with error
  - User sees: "Username conflict with existing user. Please contact administrator."
  - Administrator must either: (1) rename existing user, (2) update external API username, or (3) manually merge accounts

### Local Authentication (Default)

No additional configuration needed. Users authenticate against local SQLite database:

```bash
AUTH_MODE=local  # or omit AUTH_MODE entirely
```

### Hybrid Mode Advantages

The system supports **per-user authentication routing** based on the `auth_source` field:

- **Failsafe Admin Access**: Default admin user always uses local auth, providing emergency access
- **Mixed User Base**: Can have both local and external users in the same system
- **Zero Downtime Migration**: Gradually migrate users from local to external auth
- **Service Independence**: External service outage doesn't lock out local users

**Example Scenario:**

1. Server starts with `AUTH_MODE=http_api`
2. Default admin user created with `auth_source=local` (can always login)
3. External users authenticate via HTTP API, created with `auth_source=http_api`
4. Each user authenticates via their designated provider
5. If external API fails, admin can still login to manage the system

## External Token Provider Configuration

### HTTP API Token Provider

To use external HTTP API for token generation and validation, configure these environment variables:

```bash
TOKEN_PROVIDER_MODE=http_api
TOKEN_API_URL=https://token-service.example.com/api
TOKEN_API_TIMEOUT=10s
TOKEN_API_INSECURE_SKIP_VERIFY=false
```

**Expected API Contract:**

**Token Generation Endpoint:** `POST {TOKEN_API_URL}/generate`

Request:
```json
{
  "user_id": "user-uuid",
  "client_id": "client-uuid",
  "scopes": "read write",
  "expires_in": 3600
}
```

Response (Success):
```json
{
  "success": true,
  "access_token": "eyJhbGc...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "claims": {
    "custom_claim": "value"
  }
}
```

Response (Error):
```json
{
  "success": false,
  "message": "Invalid user_id or client_id"
}
```

**Token Validation Endpoint:** `POST {TOKEN_API_URL}/validate`

Request:
```json
{
  "token": "eyJhbGc..."
}
```

Response (Valid):
```json
{
  "valid": true,
  "user_id": "user-uuid",
  "client_id": "client-uuid",
  "scopes": "read write",
  "expires_at": 1736899200,
  "claims": {
    "custom_claim": "value"
  }
}
```

Response (Invalid):
```json
{
  "valid": false,
  "message": "Token expired or signature invalid"
}
```

**Response Requirements:**

Generation Response:
- `success` (required): Boolean indicating generation result
- `access_token` (required when success=true): Non-empty JWT string
- `token_type` (optional): Token type, defaults to "Bearer"
- `expires_in` (optional): Expiration duration in seconds
- `claims` (optional): Additional JWT claims
- `message` (optional): Error message when success=false

Validation Response:
- `valid` (required): Boolean indicating validation result
- `user_id` (required when valid=true): User identifier from token
- `client_id` (required when valid=true): Client identifier from token
- `scopes` (required when valid=true): Granted scopes
- `expires_at` (required when valid=true): Unix timestamp of expiration
- `claims` (optional): Additional JWT claims
- `message` (optional): Error message when valid=false

**Behavior:**

- Token generation/validation delegated to external service
- Token records still saved to local database for management
- Supports custom signing algorithms (RS256, ES256, etc.)
- Local database tracks: token ID, token string, user_id, client_id, scopes, expiration
- Revocation handled locally (external service doesn't need revocation endpoint)
- Token listing handled locally (external service doesn't need listing endpoint)

### Local Token Provider (Default)

No additional configuration needed. Tokens generated/validated using local JWT secret:

```bash
TOKEN_PROVIDER_MODE=local  # or omit TOKEN_PROVIDER_MODE entirely
JWT_SECRET=your-256-bit-secret-change-in-production
```

### Token Provider Benefits

**Local Mode:**
- Simple setup, no external dependencies
- Fast token operations
- Self-contained deployment

**HTTP API Mode:**
- Centralized token services across multiple applications
- Advanced key management and rotation
- Custom signing algorithms (RS256, ES256)
- Compliance requirements for token generation
- Integration with existing IAM/PKI systems

**Why Local Storage is Retained:**
- Revocation: Users can revoke tokens via `/account/sessions` or `/oauth/revoke`
- Management: Users can list active sessions
- Auditing: Track when and for which clients tokens were issued
- Client Association: Link tokens to OAuth clients for display in UI

## Coding Conventions

- Use `http.StatusOK`, `http.StatusBadRequest`, etc. instead of numeric status codes
- Services return typed errors, handlers convert to appropriate HTTP responses
- GORM models use `gorm.Model` for CreatedAt/UpdatedAt/DeletedAt
- Handlers accept both form-encoded and JSON request bodies where applicable
- All static assets and templates are embedded via `//go:embed` for single-binary deployment
- Database connection health check available via `store.Health()` method
- **IMPORTANT**: Before committing changes:
  1. **Write tests**: All new features and bug fixes MUST include corresponding unit tests
  2. **Format code**: Run `make fmt` to automatically fix code formatting issues and ensure consistency
  3. **Pass linting**: Run `make lint` to verify all code passes linting without errors

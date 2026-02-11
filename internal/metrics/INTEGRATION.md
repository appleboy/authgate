# Metrics Integration Guide

This document provides examples of how to integrate Prometheus metrics into AuthGate services and handlers.

## Overview

The metrics system consists of:

- **metrics.go**: Core metrics definitions and initialization
- **http.go**: HTTP middleware and helper methods for recording metrics
- **main.go**: Initialization and `/metrics` endpoint registration

## Current Status

✅ **Fully Implemented**

**Core Infrastructure:**

- HTTP request metrics (automatic via middleware)
- Metrics initialization with singleton pattern
- `/metrics` endpoint for Prometheus scraping
- Helper methods for recording OAuth, Auth, and Token metrics

**Service Integration:**

- ✅ Device service integration (device code generation, authorization)
- ✅ Token service integration (issuance, refresh, revocation, validation)
- ✅ Auth handler integration (login, logout with session tracking)
- ✅ OAuth handler integration (callback success/failure tracking)
- ✅ Periodic gauge updates (active tokens, device codes)

## Quick Start

The metrics system is already initialized in `main.go` and the HTTP metrics middleware is active. You can access metrics at:

```bash
curl http://localhost:8080/metrics
```

## HTTP Metrics (✅ Auto-enabled)

HTTP request metrics are automatically collected via the `HTTPMetricsMiddleware`:

```go
// Automatically tracked for all routes:
- http_requests_total{method, path, status}
- http_request_duration_seconds{method, path}
- http_requests_in_flight
```

Example output:

```
http_requests_total{method="POST",path="/oauth/token",status="200"} 42
http_request_duration_seconds_bucket{method="POST",path="/oauth/token",le="0.1"} 40
http_requests_in_flight 3
```

## Integration Examples

### 1. Device Service Integration

To record device code metrics, update `internal/services/device.go`:

```go
import "github.com/appleboy/authgate/internal/metrics"

type DeviceService struct {
    store        *store.Store
    config       *config.Config
    auditService *AuditService
    metrics      *metrics.Metrics  // Add this field
}

func NewDeviceService(
    s *store.Store,
    cfg *config.Config,
    auditService *AuditService,
    m *metrics.Metrics,  // Add parameter
) *DeviceService {
    return &DeviceService{
        store:        s,
        config:       cfg,
        auditService: auditService,
        metrics:      m,  // Store metrics instance
    }
}

func (s *DeviceService) GenerateDeviceCode(
    ctx context.Context,
    clientID, scope string,
) (*models.DeviceCode, error) {
    // ... existing validation code ...

    if err := s.store.CreateDeviceCode(deviceCode); err != nil {
        // Record failure
        if s.metrics != nil {
            s.metrics.RecordOAuthDeviceCodeGenerated(false)
        }
        return nil, err
    }

    // Record success
    if s.metrics != nil {
        s.metrics.RecordOAuthDeviceCodeGenerated(true)
    }

    // ... existing audit logging ...

    return deviceCode, nil
}

func (s *DeviceService) VerifyUserCode(
    ctx context.Context,
    userCode string,
    userID uint,
) error {
    // ... existing validation code ...

    // Calculate authorization duration
    authDuration := time.Since(dc.CreatedAt)

    // Record authorization
    if s.metrics != nil {
        s.metrics.RecordOAuthDeviceCodeAuthorized(authDuration)
    }

    // ... rest of the code ...
}
```

### 2. Token Service Integration

To record token metrics, update `internal/services/token.go`:

```go
import "github.com/appleboy/authgate/internal/metrics"

type TokenService struct {
    store                *store.Store
    config               *config.Config
    deviceService        *DeviceService
    localTokenProvider   *token.LocalTokenProvider
    httpTokenProvider    *token.HTTPTokenProvider
    tokenProviderMode    string
    auditService         *AuditService
    metrics              *metrics.Metrics  // Add this field
}

func (s *TokenService) IssueTokenForDeviceCode(
    ctx context.Context,
    deviceCode string,
) (*TokenResponse, error) {
    start := time.Now()

    // ... existing validation code ...

    // Validate device code
    dc, err := s.deviceService.GetDeviceCode(deviceCode)
    if err != nil {
        if s.metrics != nil {
            result := "invalid"
            if err == ErrDeviceCodeExpired {
                result = "expired"
            }
            s.metrics.RecordOAuthDeviceCodeValidation(result)
        }
        return nil, err
    }

    if !dc.Authorized {
        if s.metrics != nil {
            s.metrics.RecordOAuthDeviceCodeValidation("pending")
        }
        return nil, ErrAuthorizationPending
    }

    // Record successful validation
    if s.metrics != nil {
        s.metrics.RecordOAuthDeviceCodeValidation("success")
    }

    // ... generate tokens ...

    // Record token issuance
    if s.metrics != nil {
        duration := time.Since(start)
        provider := s.tokenProviderMode
        s.metrics.RecordTokenIssued("access", "device_code", duration, provider)
        if refreshToken != "" {
            s.metrics.RecordTokenIssued("refresh", "device_code", duration, provider)
        }
    }

    return &TokenResponse{
        AccessToken:  accessToken,
        TokenType:    "Bearer",
        ExpiresIn:    int(accessExpiry.Seconds()),
        RefreshToken: refreshToken,
    }, nil
}

func (s *TokenService) RefreshToken(
    ctx context.Context,
    refreshTokenPlaintext string,
) (*TokenResponse, error) {
    // ... existing code ...

    // Record refresh attempt
    if err != nil {
        if s.metrics != nil {
            s.metrics.RecordTokenRefresh(false)
        }
        return nil, err
    }

    if s.metrics != nil {
        s.metrics.RecordTokenRefresh(true)
    }

    // ... rest of the code ...
}

func (s *TokenService) RevokeToken(
    ctx context.Context,
    tokenPlaintext, tokenTypeHint string,
    reason string,
) error {
    // ... existing revocation code ...

    // Record revocation
    if s.metrics != nil {
        tokenType := "access" // or "refresh" based on token category
        s.metrics.RecordTokenRevoked(tokenType, reason)
    }

    return nil
}
```

### 3. Auth Handler Integration

To record authentication metrics, update `internal/handlers/auth.go`:

```go
import "github.com/appleboy/authgate/internal/metrics"

type AuthHandler struct {
    userService            *services.UserService
    baseURL                string
    sessionFingerprint     bool
    sessionFingerprintIP   bool
    metrics                *metrics.Metrics  // Add this field
}

func (h *AuthHandler) Login(c *gin.Context, oauthProviders map[string]*auth.OAuthProvider) {
    start := time.Now()

    // ... parse credentials ...

    // Attempt authentication
    user, err := h.userService.AuthenticateUser(c, username, password)

    authSource := "local" // or determine from user.AuthSource

    if err != nil {
        // Record failed login
        if h.metrics != nil {
            h.metrics.RecordLogin(authSource, false)
            duration := time.Since(start)
            h.metrics.RecordAuthAttempt("local", false, duration)
        }

        // ... existing error handling ...
        return
    }

    // Record successful login
    if h.metrics != nil {
        h.metrics.RecordLogin(authSource, true)
        duration := time.Since(start)
        h.metrics.RecordAuthAttempt("local", true, duration)
    }

    // ... rest of the code ...
}

func (h *AuthHandler) Logout(c *gin.Context) {
    session := sessions.Default(c)

    // Calculate session duration if available
    var sessionDuration time.Duration
    if createdAt, ok := session.Get("created_at").(time.Time); ok {
        sessionDuration = time.Since(createdAt)
    }

    session.Clear()
    session.Options(sessions.Options{MaxAge: -1})
    _ = session.Save()

    // Record logout
    if h.metrics != nil {
        h.metrics.RecordLogout(sessionDuration)
    }

    c.Redirect(http.StatusFound, "/login")
}
```

### 4. OAuth Handler Integration

For OAuth callbacks, update `internal/handlers/oauth_handler.go`:

```go
func (h *OAuthHandler) OAuthCallback(c *gin.Context) {
    provider := c.Param("provider")

    // ... existing OAuth flow ...

    if err != nil {
        if h.metrics != nil {
            h.metrics.RecordOAuthCallback(provider, false)
        }
        // ... error handling ...
        return
    }

    if h.metrics != nil {
        h.metrics.RecordOAuthCallback(provider, true)
    }

    // ... rest of the code ...
}
```

## Updating main.go

✅ **Already Implemented** - Metrics are fully wired into all services and handlers:

```go
// Metrics initialization:
prometheusMetrics := metrics.Init()

// Service initialization:
deviceService := services.NewDeviceService(db, cfg, auditService, prometheusMetrics)
tokenService := services.NewTokenService(
    db,
    cfg,
    deviceService,
    localTokenProvider,
    httpTokenProvider,
    cfg.TokenProviderMode,
    auditService,
    prometheusMetrics,
)

// Handler initialization:
authHandler := handlers.NewAuthHandler(
    userService,
    cfg.BaseURL,
    cfg.SessionFingerprint,
    cfg.SessionFingerprintIP,
    prometheusMetrics,
)

oauthHandler := handlers.NewOAuthHandler(
    oauthProviders,
    userService,
    oauthHTTPClient,
    cfg.SessionFingerprint,
    cfg.SessionFingerprintIP,
    prometheusMetrics,
)
```

## Periodic Gauge Updates

✅ **Already Implemented** - Gauge metrics are automatically updated every 30 seconds via a background job in `main.go`:

```go
// Metrics gauge update job (runs every 30 seconds)
if cfg.MetricsEnabled {
    m.AddRunningJob(func(ctx context.Context) error {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()

        // Update immediately on startup
        updateGaugeMetrics(db, prometheusMetrics)

        for {
            select {
            case <-ticker.C:
                updateGaugeMetrics(db, prometheusMetrics)
            case <-ctx.Done():
                return nil
            }
        }
    })
}

// Helper function
func updateGaugeMetrics(db *store.Store, m *metrics.Metrics) {
    // Update active tokens count
    activeAccessTokens, err := db.CountActiveTokensByCategory("access")
    if err != nil {
        log.Printf("Failed to count access tokens: %v", err)
    } else {
        m.SetActiveTokensCount("access", int(activeAccessTokens))
    }

    activeRefreshTokens, err := db.CountActiveTokensByCategory("refresh")
    if err != nil {
        log.Printf("Failed to count refresh tokens: %v", err)
    } else {
        m.SetActiveTokensCount("refresh", int(activeRefreshTokens))
    }

    // Update active device codes count
    totalDeviceCodes, pendingDeviceCodes, err := db.CountDeviceCodes()
    if err != nil {
        log.Printf("Failed to count device codes: %v", err)
    } else {
        m.SetActiveDeviceCodesCount(int(totalDeviceCodes), int(pendingDeviceCodes))
    }
}
```

## Grafana Dashboard Example

Example PromQL queries for Grafana:

```promql
# Request rate by endpoint
rate(http_requests_total[5m])

# Error rate
rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m])

# P95 latency
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# Active device codes
oauth_device_codes_active

# Token issuance rate
rate(oauth_tokens_issued_total[5m])

# Failed authentication rate
rate(auth_login_total{result="failure"}[5m])
```

## Testing Metrics

Start the server and generate some traffic:

```bash
# Start server
./bin/authgate server

# Check metrics endpoint
curl http://localhost:8080/metrics | grep oauth

# Generate some metrics:
# 1. Request device code
curl -X POST http://localhost:8080/oauth/device/code \
  -H "Content-Type: application/json" \
  -d '{"client_id":"your-client-id","scope":"read:user"}'

# 2. Login
curl -X POST http://localhost:8080/login \
  -d "username=admin&password=yourpassword"

# Check metrics again
curl http://localhost:8080/metrics | grep -E "(oauth|auth|http_request)"
```

## Best Practices

1. **Always check for nil**: Metrics instance might be nil in tests

   ```go
   if s.metrics != nil {
       s.metrics.RecordSomething()
   }
   ```

2. **Record timing at operation boundaries**: Start timer at the beginning, record at the end

   ```go
   start := time.Now()
   // ... do work ...
   if s.metrics != nil {
       s.metrics.RecordAuthAttempt("local", success, time.Since(start))
   }
   ```

3. **Use meaningful labels**: Keep cardinality low (no user IDs, tokens, etc.)

   ```go
   // Good: bounded set of values
   s.metrics.RecordLogin("local", true)

   // Bad: unbounded cardinality
   // s.metrics.RecordLogin(username, true)  // DON'T DO THIS
   ```

4. **Record both success and failure**: Always track outcomes

   ```go
   if err != nil {
       s.metrics.RecordTokenRefresh(false)
       return err
   }
   s.metrics.RecordTokenRefresh(true)
   ```

## Implementation Complete ✅

The metrics integration is now fully implemented:

- ✅ DeviceService includes metrics parameter and records device code operations
- ✅ TokenService includes metrics parameter and records token lifecycle events
- ✅ AuthHandler includes metrics parameter and records authentication events
- ✅ OAuthHandler includes metrics parameter and records OAuth callbacks
- ✅ Periodic gauge update job added to main.go (runs every 30 seconds)
- ✅ Store methods added for counting active resources:
  - `CountActiveTokensByCategory(category string)` - Counts active tokens by type
  - `CountDeviceCodes()` - Returns total and pending device codes

**Optional Next Steps:**

1. Create Grafana dashboard (see docs/MONITORING.md)
2. Configure Prometheus scraping
3. Set up alerting rules

## Configuration

Metrics are controlled via environment variables:

```bash
# Enable/disable metrics (default: false)
METRICS_ENABLED=true

# Optional: Require Bearer token for /metrics endpoint
METRICS_TOKEN=your-secret-token

# Access metrics at:
# http://localhost:8080/metrics
# Or with authentication:
# curl -H "Authorization: Bearer your-secret-token" http://localhost:8080/metrics
```

**Production Recommendations:**

- Enable `METRICS_TOKEN` for authentication
- Restrict `/metrics` endpoint to internal network
- Configure Prometheus with appropriate scrape intervals
- Set up Grafana dashboards for visualization
- Configure alerting rules for critical metrics

## See Also

- [docs/MONITORING.md](../../docs/MONITORING.md) - Monitoring best practices
- [Prometheus Documentation](https://prometheus.io/docs/)
- [Gin Prometheus Middleware](https://github.com/zsais/go-gin-prometheus)

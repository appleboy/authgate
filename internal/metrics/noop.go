package metrics

import (
	"time"

	"github.com/go-authgate/authgate/internal/core"
)

// NoopMetrics is a no-operation implementation of Recorder
// All methods are empty and do nothing, providing zero overhead when metrics are disabled
type NoopMetrics struct{}

// Ensure NoopMetrics implements Recorder interface at compile time
var _ core.Recorder = (*NoopMetrics)(nil)

// NewNoopMetrics creates a new no-operation metrics recorder
func NewNoopMetrics() core.Recorder {
	return &NoopMetrics{}
}

// OAuth Device Flow - noop implementations
func (n *NoopMetrics) RecordOAuthDeviceCodeGenerated(success bool)                     {}
func (n *NoopMetrics) RecordOAuthDeviceCodeAuthorized(authorizationTime time.Duration) {}
func (n *NoopMetrics) RecordOAuthDeviceCodeValidation(result string)                   {}

// Token Operations - noop implementations
func (n *NoopMetrics) RecordTokenIssued(
	tokenType, grantType string,
	generationTime time.Duration,
	provider string,
) {
}

func (n *NoopMetrics) RecordTokenRevoked(
	tokenType, reason string,
) {
}

func (n *NoopMetrics) RecordTokenRefresh(
	success bool,
) {
}

func (n *NoopMetrics) RecordTokenValidation(
	result string,
	duration time.Duration,
	provider string,
) {
}

// Authentication - noop implementations
func (n *NoopMetrics) RecordAuthAttempt(method string, success bool, duration time.Duration) {}
func (n *NoopMetrics) RecordLogin(authSource string, success bool)                           {}
func (n *NoopMetrics) RecordLogout(sessionDuration time.Duration)                            {}
func (n *NoopMetrics) RecordOAuthCallback(provider string, success bool)                     {}

// Gauge Setters - noop implementations
func (n *NoopMetrics) SetActiveTokensCount(tokenType string, count int) {}
func (n *NoopMetrics) SetActiveDeviceCodesCount(total, pending int)     {}

// Database Operations - noop implementations
func (n *NoopMetrics) RecordDatabaseQueryError(operation string) {}

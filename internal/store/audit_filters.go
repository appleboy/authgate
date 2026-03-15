package store

import "github.com/go-authgate/authgate/internal/store/types"

// Re-export types from store/types for backward compatibility.
type (
	AuditLogFilters = types.AuditLogFilters
	AuditLogStats   = types.AuditLogStats
)

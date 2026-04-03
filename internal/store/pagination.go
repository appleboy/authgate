package store

import "github.com/go-authgate/authgate/internal/store/types"

// Re-export types from store/types for backward compatibility.
type (
	PaginationParams = types.PaginationParams
	PaginationResult = types.PaginationResult
	DashboardCounts  = types.DashboardCounts
)

// Re-export functions.
var (
	NewPaginationParams = types.NewPaginationParams
	CalculatePagination = types.CalculatePagination
)

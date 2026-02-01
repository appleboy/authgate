package store

import (
	"testing"
)

func TestNewPaginationParams(t *testing.T) {
	tests := []struct {
		name         string
		page         int
		pageSize     int
		search       string
		expectedPage int
		expectedSize int
	}{
		{
			name:         "valid parameters",
			page:         2,
			pageSize:     20,
			search:       "test",
			expectedPage: 2,
			expectedSize: 20,
		},
		{
			name:         "invalid page number defaults to 1",
			page:         0,
			pageSize:     10,
			search:       "",
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "negative page number defaults to 1",
			page:         -5,
			pageSize:     10,
			search:       "",
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "invalid page size defaults to 10",
			page:         1,
			pageSize:     0,
			search:       "",
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "page size exceeds max, capped at 50",
			page:         1,
			pageSize:     100,
			search:       "",
			expectedPage: 1,
			expectedSize: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := NewPaginationParams(tt.page, tt.pageSize, tt.search)

			if params.Page != tt.expectedPage {
				t.Errorf("expected page %d, got %d", tt.expectedPage, params.Page)
			}
			if params.PageSize != tt.expectedSize {
				t.Errorf("expected page size %d, got %d", tt.expectedSize, params.PageSize)
			}
			if params.Search != tt.search {
				t.Errorf("expected search '%s', got '%s'", tt.search, params.Search)
			}
		})
	}
}

func TestCalculatePagination(t *testing.T) {
	tests := []struct {
		name              string
		total             int64
		currentPage       int
		pageSize          int
		expectedTotal     int64
		expectedTotalPage int
		expectedCurrent   int
		expectedHasPrev   bool
		expectedHasNext   bool
		expectedPrevPage  int
		expectedNextPage  int
	}{
		{
			name:              "first page of multiple pages",
			total:             100,
			currentPage:       1,
			pageSize:          10,
			expectedTotal:     100,
			expectedTotalPage: 10,
			expectedCurrent:   1,
			expectedHasPrev:   false,
			expectedHasNext:   true,
			expectedPrevPage:  1,
			expectedNextPage:  2,
		},
		{
			name:              "middle page",
			total:             100,
			currentPage:       5,
			pageSize:          10,
			expectedTotal:     100,
			expectedTotalPage: 10,
			expectedCurrent:   5,
			expectedHasPrev:   true,
			expectedHasNext:   true,
			expectedPrevPage:  4,
			expectedNextPage:  6,
		},
		{
			name:              "last page",
			total:             100,
			currentPage:       10,
			pageSize:          10,
			expectedTotal:     100,
			expectedTotalPage: 10,
			expectedCurrent:   10,
			expectedHasPrev:   true,
			expectedHasNext:   false,
			expectedPrevPage:  9,
			expectedNextPage:  10,
		},
		{
			name:              "single page",
			total:             5,
			currentPage:       1,
			pageSize:          10,
			expectedTotal:     5,
			expectedTotalPage: 1,
			expectedCurrent:   1,
			expectedHasPrev:   false,
			expectedHasNext:   false,
			expectedPrevPage:  1,
			expectedNextPage:  1,
		},
		{
			name:              "empty result",
			total:             0,
			currentPage:       1,
			pageSize:          10,
			expectedTotal:     0,
			expectedTotalPage: 0,
			expectedCurrent:   1,
			expectedHasPrev:   false,
			expectedHasNext:   false,
			expectedPrevPage:  1,
			expectedNextPage:  0,
		},
		{
			name:              "page beyond total pages gets adjusted",
			total:             25,
			currentPage:       10,
			pageSize:          10,
			expectedTotal:     25,
			expectedTotalPage: 3,
			expectedCurrent:   3,
			expectedHasPrev:   true,
			expectedHasNext:   false,
			expectedPrevPage:  2,
			expectedNextPage:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePagination(tt.total, tt.currentPage, tt.pageSize)

			if result.Total != tt.expectedTotal {
				t.Errorf("expected total %d, got %d", tt.expectedTotal, result.Total)
			}
			if result.TotalPages != tt.expectedTotalPage {
				t.Errorf("expected total pages %d, got %d", tt.expectedTotalPage, result.TotalPages)
			}
			if result.CurrentPage != tt.expectedCurrent {
				t.Errorf("expected current page %d, got %d", tt.expectedCurrent, result.CurrentPage)
			}
			if result.HasPrev != tt.expectedHasPrev {
				t.Errorf("expected has prev %v, got %v", tt.expectedHasPrev, result.HasPrev)
			}
			if result.HasNext != tt.expectedHasNext {
				t.Errorf("expected has next %v, got %v", tt.expectedHasNext, result.HasNext)
			}
			if result.PrevPage != tt.expectedPrevPage {
				t.Errorf("expected prev page %d, got %d", tt.expectedPrevPage, result.PrevPage)
			}
			if result.NextPage != tt.expectedNextPage {
				t.Errorf("expected next page %d, got %d", tt.expectedNextPage, result.NextPage)
			}
		})
	}
}

// Package pagination provides utilities for paginating slices.
package pagination

// Result holds pagination metadata.
type Result struct {
	TotalCount int
	Truncated  bool
}

// ApplyPagination returns the paginated slice and metadata.
// It handles bounds checking to prevent panics and clamps offset/limit.
func ApplyPagination[T any](items []T, offset, limit int) ([]T, Result) {
	totalCount := len(items)
	start := offset
	end := offset + limit
	truncated := end < totalCount

	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	return items[start:end], Result{
		TotalCount: totalCount,
		Truncated:  truncated,
	}
}

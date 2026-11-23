package util

import (
	"fmt"
	"strconv"
)

// DefaultPageSize is the page size used when the caller sends none.
const DefaultPageSize = 100

// PageParams holds the offset and limit of a list query.
type PageParams struct {
	Offset int
	Limit  int
}

// ParsePageParams converts the "page" and "size" query parameters of a list
// request into an offset and a limit. Pages start at 1.
func ParsePageParams(page string, size string) (PageParams, error) {
	params := PageParams{Offset: 0, Limit: DefaultPageSize}

	if size != "" {
		limit, _ := strconv.Atoi(size)
		params.Limit = limit
	}

	if page != "" {
		number, err := strconv.Atoi(page)
		if err != nil {
			return params, fmt.Errorf("invalid page %s", page)
		}
		params.Offset = number * params.Limit
	}

	return params, nil
}

// TotalPages returns the number of pages needed to list total items.
func TotalPages(total int, size int) int {
	return total / size
}

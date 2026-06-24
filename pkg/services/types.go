package services

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// ListArguments are arguments relevant for listing objects.
// This struct is common to all service List funcs in this package
type ListArguments struct {
	Search   string
	Preloads []string
	OrderBy  []string
	Fields   []string
	Page     int
	Size     int64
}

// MaxListSize defines the PostgreSQL WHERE IN clause parameter limit (~65500).
// Note: This is currently unreachable via HTTP requests since MaxPageSize caps at 100,
// but is kept as a defensive check for direct service layer usage and to document the
// technical database constraint.
const MaxListSize = 65500

// MaxPageSize is the maximum allowed page size for pagination via HTTP requests.
// Set to 100 to prevent excessive resource usage and ensure reasonable response times.
const MaxPageSize = 100

// ParseFieldsParameter extracts and parses the ?fields query parameter.
// Returns a slice of field names with "id" always included when valid fields are provided.
// Returns nil if no valid fields are specified (empty or whitespace-only parameter).
func ParseFieldsParameter(params url.Values) []string {
	if v := strings.TrimSpace(params.Get("fields")); v != "" {
		fields := strings.Split(v, ",")
		result := make([]string, 0, len(fields)+1)
		idPresent := false
		for _, field := range fields {
			trimmed := strings.TrimSpace(field)
			if trimmed == "" {
				continue
			}
			if trimmed == "id" {
				idPresent = true
			}
			result = append(result, trimmed)
		}
		// If no valid fields were provided (e.g., "fields=" or "fields=   "), return nil
		if len(result) == 0 {
			return nil
		}
		// Always include id field when user provided valid fields
		if !idPresent {
			result = append(result, "id")
		}
		return result
	}
	return nil
}

// NewListArguments Create ListArguments from url query parameters with sane defaults
// Returns an error if page or size parameters are invalid (negative, non-numeric, or out of range)
func NewListArguments(params url.Values) (*ListArguments, *errors.ServiceError) {
	listArgs := &ListArguments{
		Page:   1,
		Size:   20,
		Search: "",
	}

	// Validate page parameter
	if v := strings.Trim(params.Get("page"), " "); v != "" {
		page, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New(
				errors.CodeValidationFormat,
				"Invalid page parameter: must be a positive integer",
			)
		}
		if page < 1 {
			return nil, errors.New(
				errors.CodeValidationRange,
				"Invalid page parameter: %d is less than 1",
				page,
			)
		}
		listArgs.Page = page
	}

	// Validate size parameter (support both "size" legacy and "pageSize" OpenAPI spec)
	var sizeParam string
	var sizeValue string
	if v := strings.Trim(params.Get("pageSize"), " "); v != "" {
		sizeParam = "pageSize"
		sizeValue = v
	} else if v := strings.Trim(params.Get("size"), " "); v != "" {
		sizeParam = "size"
		sizeValue = v
	}

	if sizeValue != "" {
		size, err := strconv.ParseInt(sizeValue, 10, 64)
		if err != nil {
			return nil, errors.New(
				errors.CodeValidationFormat,
				"Invalid %s parameter: must be a positive integer",
				sizeParam,
			)
		}
		if size < 1 {
			return nil, errors.New(
				errors.CodeValidationRange,
				"Invalid %s parameter: %d is less than 1",
				sizeParam, size,
			)
		}
		if size > MaxPageSize {
			return nil, errors.New(
				errors.CodeValidationRange,
				"Invalid %s parameter: %d exceeds maximum allowed value of %d",
				sizeParam, size, MaxPageSize,
			)
		}
		listArgs.Size = size
	}

	if v := strings.Trim(params.Get("search"), " "); v != "" {
		listArgs.Search = v
	}
	if v := strings.Trim(params.Get("orderBy"), " "); v != "" {
		rawFields := strings.Split(v, ",")
		// Filter out empty tokens from malformed input like "name,,created_time"
		for _, field := range rawFields {
			if trimmed := strings.TrimSpace(field); trimmed != "" {
				listArgs.OrderBy = append(listArgs.OrderBy, strings.Join(strings.Fields(trimmed), " "))
			}
		}
	}

	// Set default sorting to created_time desc if orderBy not provided
	if len(listArgs.OrderBy) == 0 {
		listArgs.OrderBy = []string{"created_time desc"}
	}

	// Parse fields parameter using shared logic
	listArgs.Fields = ParseFieldsParameter(params)

	return listArgs, nil
}

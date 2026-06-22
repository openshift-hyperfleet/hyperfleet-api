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

// ~65500 is the maximum number of parameters that can be provided to a postgres WHERE IN clause
// Use it as a sane max
const MaxListSize = 65500

// MaxPageSize is the maximum allowed page size for pagination
// Set to 100 to prevent excessive resource usage
const MaxPageSize = 100

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
				listArgs.OrderBy = append(listArgs.OrderBy, trimmed)
			}
		}
	}

	// Validate and apply order parameter (asc/desc direction)
	if v := strings.Trim(params.Get("order"), " "); v != "" {
		if v != "asc" && v != "desc" {
			return nil, errors.New(
				errors.CodeValidationFormat,
				"Invalid order parameter: must be 'asc' or 'desc', got '%s'",
				v,
			)
		}
		// Apply order direction to all orderBy fields that don't already have a direction
		for i, field := range listArgs.OrderBy {
			trimmedField := strings.TrimSpace(field)
			if trimmedField == "" {
				// Skip empty tokens from malformed input like "name,,created_time"
				continue
			}
			parts := strings.Split(trimmedField, " ")
			if len(parts) == 1 {
				// Field has no direction specified, apply the order parameter
				listArgs.OrderBy[i] = trimmedField + " " + v
			}
			// If field already has direction (e.g., "name asc"), leave it unchanged
		}
	}

	// Set default sorting to created_time desc if orderBy not provided
	if len(listArgs.OrderBy) == 0 {
		listArgs.OrderBy = []string{"created_time desc"}
	}

	if v := strings.Trim(params.Get("fields"), " "); v != "" {
		fields := strings.Split(v, ",")
		idNotPresent := true
		for i := 0; i < len(fields); i++ {
			field := strings.Trim(fields[i], " ")
			if field == "" { // skip leading/trailing commas and spaces
				continue
			}
			if field == "id" {
				idNotPresent = false
			}
			listArgs.Fields = append(listArgs.Fields, field)
		}
		if idNotPresent {
			listArgs.Fields = append(listArgs.Fields, "id")
		}
	}

	return listArgs, nil
}

package db

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/jinzhu/inflection"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// orderAllowedFields defines the whitelist of fields that are allowed to be ordered.
// This prevents SQL injection and restricts invalid order queries.
var orderAllowedFields = map[string]bool{
	"id":           true,
	"name":         true,
	"created_time": true,
	"updated_time": true,
	"deleted_time": true,
	"kind":         true,
	"created_by":   true,
	"updated_by":   true,
	"deleted_by":   true,
	"generation":   true,
	"href":         true,
}

// orderPattern matches valid order syntax: field name (letters, digits, underscore) followed by optional asc/desc.
// This regex rejects SQL injection attempts (semicolons, parentheses, dashes, comments, etc).
var orderPattern = regexp.MustCompile(`^[a-z_][a-z_]*(\s+(asc|desc))?$`)

// ArgsToOrder validates and cleans order arguments against the allowed fields whitelist.
// Returns a cleaned list of order clauses in the format ["field direction", ...]
// Empty or whitespace-only strings are silently skipped.
func ArgsToOrder(args []string) (cleanedOrderList []string, err *errors.ServiceError) {
	for _, val := range args {
		// Accept args with trailing and leading spaces
		trimVal := strings.TrimSpace(val)

		// Skip empty strings silently
		if trimVal == "" {
			continue
		}

		// Check for SQL injection attempts before parsing
		if !orderPattern.MatchString(trimVal) {
			return nil, errors.BadRequest("invalid order format '%s': expected 'field' or 'field asc|desc'", val)
		}

		// Each value should be "<field-name>" or "<field-name> asc|desc"
		splitVal := strings.Fields(trimVal)
		lenVal := len(splitVal)

		var field, direction string

		switch lenVal {
		case 2:
			field = splitVal[0]
			direction = splitVal[1]
			if direction != "asc" && direction != "desc" {
				return nil, errors.BadRequest("invalid sort direction '%s': must be 'asc' or 'desc'", direction)
			}
		case 1:
			field = splitVal[0]
			direction = "asc"
		default:
			return nil, errors.BadRequest("invalid order format '%s': expected 'field' or 'field asc|desc'", val)
		}

		// Validate field against orderAllowedFields
		if !orderAllowedFields[field] {
			return nil, errors.BadRequest("field '%s' is not allowed for ordering", field)
		}

		cleanedValue := fmt.Sprintf("%s %s", field, direction)
		cleanedOrderList = append(cleanedOrderList, cleanedValue)
	}

	return cleanedOrderList, nil
}

func GetTableName(g2 *gorm.DB) string {
	if g2.Statement.Parse(g2.Statement.Model) != nil {
		return "xxx"
	}
	if g2.Statement.Schema != nil {
		return g2.Statement.Schema.Table
	} else {
		name := reflect.TypeOf(g2.Statement.Model).Elem().Name()
		return inflection.Plural(strings.ToLower(name))
	}
}

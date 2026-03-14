package db

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jinzhu/inflection"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/yaacov/tree-search-language/pkg/tsl"
	"gorm.io/gorm"
)

// Label key validation pattern: only lowercase letters, digits, and underscores to prevent SQL injection
var labelKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// validateLabelKey validates a label key to prevent SQL injection
// through field name interpolation. Only allows lowercase letters, digits, and underscores.
func validateLabelKey(key string) *errors.ServiceError {
	if key == "" {
		return errors.BadRequest("label key cannot be empty")
	}

	if !labelKeyPattern.MatchString(key) {
		return errors.BadRequest(
			"label key '%s' is invalid: must contain only lowercase letters, digits, and underscores", key,
		)
	}

	return nil
}

// Check if a field name starts with properties.
func startsWithProperties(s string) bool {
	return strings.HasPrefix(s, "properties.")
}

// hasProperty return true if node has a property identifier on left hand side.
func hasProperty(n tsl.Node) bool {
	// Get the left side operator.
	l, ok := n.Left.(tsl.Node)
	if !ok {
		return false
	}

	// If left side hand is not a `properties` identifier, return.
	leftStr, ok := l.Left.(string)
	if !ok || l.Func != tsl.IdentOp || !startsWithProperties(leftStr) {
		return false
	}

	return true
}

// Field mapping rules for user-friendly syntax to database columns
// Note: status.phase was removed - use status.conditions.Ready='True' or status.conditions.Available='True' instead
var statusFieldMappings = map[string]string{
	"status.conditions": "status_conditions",
}

// getField gets the sql field associated with a name.
func getField(name string, disallowedFields map[string]string) (field string, err *errors.ServiceError) {
	// We want to accept names with trailing and leading spaces
	trimmedName := strings.Trim(name, " ")

	// Check for properties ->> '<some field name>'
	if strings.HasPrefix(trimmedName, "properties ->>") {
		field = trimmedName
		return
	}

	// Map user-friendly labels.xxx syntax to JSONB query: labels->>'xxx'
	if strings.HasPrefix(trimmedName, "labels.") {
		key := strings.TrimPrefix(trimmedName, "labels.")

		// Validate label key to prevent SQL injection
		if validationErr := validateLabelKey(key); validationErr != nil {
			err = validationErr
			return
		}

		field = fmt.Sprintf("labels->>'%s'", key)
		return
	}

	// Map user-friendly status.xxx syntax to database columns
	if mapped, ok := statusFieldMappings[trimmedName]; ok {
		trimmedName = mapped
	}

	// Check for nested field, e.g., subscription_labels.key
	checkName := trimmedName
	fieldParts := strings.Split(trimmedName, ".")
	if len(fieldParts) > 2 {
		err = errors.BadRequest("%s is not a valid field name", name)
		return
	}
	if len(fieldParts) > 1 {
		checkName = fieldParts[1]
	}

	// Check for disallowed fields
	_, ok := disallowedFields[checkName]
	if ok {
		err = errors.BadRequest("%s is not a valid field name", name)
		return
	}
	field = trimmedName
	return
}

// propertiesNodeConverter converts a node with a properties identifier
// to a properties node.
//
// For example, it will convert:
// ( properties.<name> = <value> ) to
// ( properties ->> <name> = <value> )
func propertiesNodeConverter(n tsl.Node) tsl.Node {

	// Get the left side operator.
	l, ok := n.Left.(tsl.Node)
	if !ok {
		return n
	}

	// Get the property name.
	leftStr, ok := l.Left.(string)
	if !ok || len(leftStr) <= 11 {
		return n
	}
	propertyName := leftStr[11:]

	// Build a new node that converts:
	// ( properties.<name> = <value> ) to
	// ( properties ->> <name> = <value> )
	propertyNode := tsl.Node{
		Func: n.Func,
		Left: tsl.Node{
			Func: tsl.IdentOp,
			Left: fmt.Sprintf("properties ->> '%s'", propertyName),
		},
		Right: n.Right,
	}

	return propertyNode
}

// Condition type validation pattern: PascalCase condition types (e.g., Ready, Available, Progressing)
var conditionTypePattern = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)

// Condition status validation: must be True, False, or Unknown
var validConditionStatuses = map[string]bool{
	"True":    true,
	"False":   true,
	"Unknown": true,
}

// conditionSubfieldPattern matches 4-part condition paths like status.conditions.Ready.last_updated_time
// and encodes them to 3-part paths (status.conditions.Ready__last_updated_time) so the TSL parser can handle them.
// The TSL grammar only supports up to 3-part identifiers (database.table.column).
// The (^|[\s(]) anchor ensures we don't match things like "xstatus.conditions.Ready.last_updated_time".
// Group 1 captures the leading boundary (start-of-string, whitespace, or opening paren) to preserve it.
var conditionSubfieldPattern = regexp.MustCompile(
	`(^|[\s(])(status\.conditions\.[A-Z][a-zA-Z0-9]*)\.([a-z][a-z_]+)`,
)

// PreprocessConditionSubfields rewrites 4-part condition paths into 3-part paths
// before TSL parsing. The TSL parser supports at most 3 dot-separated segments.
// Only replaces in unquoted segments to avoid mutating string literals.
//
// Example: status.conditions.Ready.last_updated_time < '2026-03-06T00:00:00Z'
// becomes: status.conditions.Ready__last_updated_time < '2026-03-06T00:00:00Z'
func PreprocessConditionSubfields(search string) string {
	var result strings.Builder
	result.Grow(len(search))
	inQuote := false
	quoteChar := byte(0)
	segStart := 0

	for i := 0; i < len(search); i++ {
		ch := search[i]
		if inQuote {
			if ch == quoteChar {
				// Flush quoted segment as-is (no replacement)
				result.WriteString(search[segStart : i+1])
				segStart = i + 1
				inQuote = false
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			// Flush unquoted segment with replacement applied
			result.WriteString(
				conditionSubfieldPattern.ReplaceAllString(search[segStart:i], "${1}${2}__${3}"),
			)
			segStart = i
			inQuote = true
			quoteChar = ch
		}
	}
	// Flush remaining segment — only apply replacement if outside a quote
	if inQuote {
		result.WriteString(search[segStart:])
	} else {
		result.WriteString(
			conditionSubfieldPattern.ReplaceAllString(search[segStart:], "${1}${2}__${3}"),
		)
	}
	return result.String()
}

// conditionTimeSubfields are condition subfields that store timestamps and require TIMESTAMPTZ casting.
// Note: created_time is intentionally excluded — it reflects when the condition was first created
// and is not useful for Sentinel polling or staleness queries.
var conditionTimeSubfields = map[string]bool{
	"last_updated_time":    true,
	"last_transition_time": true,
}

// conditionIntSubfields are condition subfields that store integers and require INTEGER casting
var conditionIntSubfields = map[string]bool{
	"observed_generation": true,
}

// comparisonOperators maps TSL operator constants to SQL operator strings
var comparisonOperators = map[string]string{
	tsl.EqOp:    "=",
	tsl.NotEqOp: "!=",
	tsl.LtOp:    "<",
	tsl.LteOp:   "<=",
	tsl.GtOp:    ">",
	tsl.GteOp:   ">=",
}

// startsWithConditions checks if a field starts with status.conditions.
func startsWithConditions(s string) bool {
	return strings.HasPrefix(s, "status.conditions.")
}

// hasCondition returns true if node has a status.conditions.<Type> identifier on left hand side.
func hasCondition(n tsl.Node) bool {
	// Get the left side operator.
	l, ok := n.Left.(tsl.Node)
	if !ok {
		return false
	}

	// If left side is not a `status.conditions.` identifier, return.
	leftStr, ok := l.Left.(string)
	if !ok || l.Func != tsl.IdentOp || !startsWithConditions(leftStr) {
		return false
	}

	return true
}

// conditionsNodeConverter handles condition queries in two forms:
//
// 3-part path (status query): status.conditions.<ConditionType>='<Status>'
//
//	Transforms to: jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Ready")') ->> 'status' = 'True'
//
// 4-part path (subfield query):
//
//	status.conditions.<ConditionType>.<Subfield> <op> '<Value>'
//	Time subfields use CAST(... AS TIMESTAMPTZ)
//	Integer subfields use CAST(... AS INTEGER)
func conditionsNodeConverter(n tsl.Node) (interface{}, *errors.ServiceError) {
	// Get the left side operator.
	l, ok := n.Left.(tsl.Node)
	if !ok {
		return nil, errors.BadRequest("invalid condition query structure")
	}

	// Extract the full field path
	leftStr, ok := l.Left.(string)
	if !ok {
		return nil, errors.BadRequest("expected string for left side of condition")
	}

	// After PreprocessConditionSubfields, the path is always 3 parts:
	// - status.conditions.Ready (status query)
	// - status.conditions.Ready__last_updated_time (subfield query, encoded with __)
	parts := strings.Split(leftStr, ".")
	if len(parts) != 3 || parts[0] != "status" || parts[1] != "conditions" {
		return nil, errors.BadRequest("invalid condition field path: %s", leftStr)
	}

	// Check if the 3rd part contains an encoded subfield (e.g., Ready__last_updated_time).
	// The __ encoding is produced by PreprocessConditionSubfields, but users can also
	// submit the encoded form directly — the same validation applies either way.
	subfieldParts := strings.SplitN(parts[2], "__", 2)
	conditionType := subfieldParts[0]

	// Validate condition type to prevent SQL injection
	if !conditionTypePattern.MatchString(conditionType) {
		return nil, errors.BadRequest(
			"condition type '%s' is invalid: must be PascalCase (e.g., Ready, Available)", conditionType,
		)
	}

	// Subfield query (e.g., Ready__last_updated_time -> conditionType=Ready, subfield=last_updated_time)
	if len(subfieldParts) == 2 {
		return conditionSubfieldConverter(n, conditionType, subfieldParts[1])
	}

	// Status query (e.g., status.conditions.Ready='True')
	return conditionStatusConverter(n, conditionType)
}

// conditionStatusConverter handles 3-part condition status queries:
// status.conditions.<ConditionType>='<Status>'
func conditionStatusConverter(n tsl.Node, conditionType string) (interface{}, *errors.ServiceError) {
	// Get the right side value (the expected status)
	r, ok := n.Right.(tsl.Node)
	if !ok {
		return nil, errors.BadRequest("invalid condition query structure: missing right side")
	}

	rightStr, ok := r.Left.(string)
	if !ok {
		return nil, errors.BadRequest("expected string for right side of condition")
	}

	// Validate condition status
	if !validConditionStatuses[rightStr] {
		return nil, errors.BadRequest(
			"condition status '%s' is invalid: must be True, False, or Unknown", rightStr,
		)
	}

	// Only support equality operator for condition status queries
	if n.Func != tsl.EqOp {
		return nil, errors.BadRequest("only equality operator (=) is supported for condition status queries")
	}

	// Build query using jsonb_path_query_first.
	// NOTE: Ideally the jsonpath literal would be inlined so PostgreSQL can match expression
	// indexes, but Squirrel treats the '?' in jsonpath filter syntax ($[*] ? (...)) as a bind
	// placeholder. HYPERFLEET-736 evaluates generated columns or table normalization as a
	// proper fix to enable index usage.
	jsonPath := fmt.Sprintf(`$[*] ? (@.type == "%s")`, conditionType)
	return sq.Expr("jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?", jsonPath, rightStr), nil
}

// conditionSubfieldConverter handles 4-part condition subfield queries:
// status.conditions.<ConditionType>.<Subfield> <op> '<Value>'
func conditionSubfieldConverter(n tsl.Node, conditionType, subfield string) (interface{}, *errors.ServiceError) {
	// Validate the operator is a supported comparison operator
	sqlOp, ok := comparisonOperators[n.Func]
	if !ok {
		return nil, errors.BadRequest(
			"operator '%s' is not supported for condition subfield queries; use =, !=, <, <=, >, or >=", n.Func,
		)
	}

	// Get the right side value
	r, rOk := n.Right.(tsl.Node)
	if !rOk {
		return nil, errors.BadRequest("invalid condition query structure: missing right side")
	}

	// NOTE: Ideally the jsonpath and subfield literals would be inlined so PostgreSQL can
	// match expression indexes, but Squirrel treats '?' in jsonpath filter syntax as a bind
	// placeholder. HYPERFLEET-736 evaluates generated columns or table normalization as a
	// proper fix to enable index usage.
	jsonPath := fmt.Sprintf(`$[*] ? (@.type == "%s")`, conditionType)

	// Handle time subfields
	if conditionTimeSubfields[subfield] {
		rightStr, strOk := r.Left.(string)
		if !strOk {
			return nil, errors.BadRequest(
				"expected string timestamp value for condition subfield '%s'", subfield,
			)
		}
		if _, parseErr := time.Parse(time.RFC3339, rightStr); parseErr != nil {
			return nil, errors.BadRequest(
				"invalid timestamp for condition subfield '%s': expected RFC3339 format (e.g., 2026-01-01T00:00:00Z)",
				subfield,
			)
		}
		query := fmt.Sprintf(
			"CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS TIMESTAMPTZ) %s ?::timestamptz",
			sqlOp,
		)
		return sq.Expr(query, jsonPath, subfield, rightStr), nil
	}

	// Handle integer subfields
	if conditionIntSubfields[subfield] {
		rightVal, numOk := r.Left.(float64)
		if !numOk {
			return nil, errors.BadRequest(
				"expected numeric value for condition subfield '%s'", subfield,
			)
		}
		if rightVal != math.Trunc(rightVal) {
			return nil, errors.BadRequest(
				"expected integer value for condition subfield '%s', got %v", subfield, rightVal,
			)
		}
		if rightVal < math.MinInt32 || rightVal > math.MaxInt32 {
			return nil, errors.BadRequest(
				"value %v is out of 32-bit integer range for condition subfield '%s'",
				rightVal, subfield,
			)
		}
		query := fmt.Sprintf(
			"CAST(jsonb_path_query_first(status_conditions, ?::jsonpath) ->> ? AS INTEGER) %s ?",
			sqlOp,
		)
		return sq.Expr(query, jsonPath, subfield, int(rightVal)), nil
	}

	return nil, errors.BadRequest(
		"condition subfield '%s' is not supported; use last_updated_time, last_transition_time, or observed_generation",
		subfield,
	)
}

// ConditionExpression represents an extracted condition query as a Squirrel expression
type ConditionExpression struct {
	Expr sq.Sqlizer
}

// ExtractConditionQueries walks the TSL tree and extracts condition queries,
// returning the modified tree (with condition nodes replaced) and the extracted conditions.
// This is necessary because the TSL library doesn't support JSONB containment operators.
func ExtractConditionQueries(n tsl.Node, tableName string) (tsl.Node, []sq.Sqlizer, *errors.ServiceError) {
	var conditions []sq.Sqlizer
	modifiedTree, err := extractConditionsWalk(n, &conditions)
	return modifiedTree, conditions, err
}

// Returns true if any node in the subtree rooted at n is a condition query
func subtreeHasCondition(n tsl.Node) bool {
	if hasCondition(n) {
		return true
	}

	l, ok := n.Left.(tsl.Node)
	if ok && subtreeHasCondition(l) {
		return true
	}

	r, ok := n.Right.(tsl.Node)
	if ok && subtreeHasCondition(r) {
		return true
	}

	rr, ok := n.Right.([]tsl.Node)
	if ok {
		for _, r := range rr {
			if subtreeHasCondition(r) {
				return true
			}
		}
	}
	return false
}

// extractConditionsWalk recursively walks the tree and extracts condition queries
func extractConditionsWalk(n tsl.Node, conditions *[]sq.Sqlizer) (tsl.Node, *errors.ServiceError) {
	// Reject NOT applied to condition queries — the condition is extracted before
	// NOT is applied, so the negation would be silently lost, producing wrong results.
	// Scan the entire NOT subtree, not just the direct child, to catch conditions
	// nested inside AND/OR under NOT (e.g., NOT (condA AND x)).
	if n.Func == tsl.NotOp {
		if child, ok := n.Left.(tsl.Node); ok && subtreeHasCondition(child) {
			return n, errors.BadRequest(
				"NOT operator is not supported with condition queries; use the inverse condition instead (e.g., Ready='False')",
			)
		}
	}

	// Check if this node is a condition query
	if hasCondition(n) {
		expr, err := conditionsNodeConverter(n)
		if err != nil {
			return n, err
		}
		*conditions = append(*conditions, expr.(sq.Sqlizer))

		// Replace with a placeholder that always evaluates to true
		// This allows the rest of the tree to be processed normally
		return tsl.Node{
			Func:  tsl.EqOp,
			Left:  tsl.Node{Func: tsl.NumberOp, Left: float64(1)},
			Right: tsl.Node{Func: tsl.NumberOp, Left: float64(1)},
		}, nil
	}

	// For non-condition nodes, recursively process children
	var newLeft, newRight interface{}
	var serviceErr *errors.ServiceError

	if n.Left != nil {
		switch v := n.Left.(type) {
		case tsl.Node:
			newLeftNode, err := extractConditionsWalk(v, conditions)
			if err != nil {
				return n, err
			}
			newLeft = newLeftNode
		default:
			newLeft = n.Left
		}
	}

	if n.Right != nil {
		switch v := n.Right.(type) {
		case tsl.Node:
			newRightNode, err := extractConditionsWalk(v, conditions)
			if err != nil {
				return n, err
			}
			newRight = newRightNode
		case []tsl.Node:
			var newRightNodes []tsl.Node
			for _, rightNode := range v {
				newRightNode, err := extractConditionsWalk(rightNode, conditions)
				if err != nil {
					return n, err
				}
				newRightNodes = append(newRightNodes, newRightNode)
			}
			newRight = newRightNodes
		default:
			newRight = n.Right
		}
	}

	if serviceErr != nil {
		return n, serviceErr
	}

	return tsl.Node{
		Func:  n.Func,
		Left:  newLeft,
		Right: newRight,
	}, nil
}

// FieldNameWalk walks on the filter tree and check/replace
// the search fields names:
// a. the the field name is valid.
// b. replace the field name with the SQL column name.
func FieldNameWalk(
	n tsl.Node,
	disallowedFields map[string]string) (newNode tsl.Node, err *errors.ServiceError) {

	var field string
	var l, r tsl.Node

	// Check for properties.<name> = <value> nodes, and convert them to
	// ( properties ->> <name> = <value> )
	// nodes.
	if hasProperty(n) {
		n = propertiesNodeConverter(n)
	}

	switch n.Func {
	case tsl.IdentOp:
		// If this is an Identifier, check field name is a string.
		userFieldName, ok := n.Left.(string)
		if !ok {
			err = errors.BadRequest("Identifier name must be a string")
			return
		}

		// Check field name in the disallowedFields field names.
		field, err = getField(userFieldName, disallowedFields)
		if err != nil {
			return
		}

		// Replace identifier name.
		newNode = tsl.Node{Func: tsl.IdentOp, Left: field}
	case tsl.StringOp, tsl.NumberOp:
		// This are leafs, just return.
		newNode = tsl.Node{Func: n.Func, Left: n.Left}
	default:
		// o/w continue walking the tree.
		if n.Left != nil {
			leftNode, ok := n.Left.(tsl.Node)
			if !ok {
				err = errors.BadRequest("invalid node structure")
				return
			}
			l, err = FieldNameWalk(leftNode, disallowedFields)
			if err != nil {
				return
			}
		}

		// Add right child(ren) if exist.
		if n.Right != nil {
			switch v := n.Right.(type) {
			case tsl.Node:
				// It's a regular node, just add it.
				r, err = FieldNameWalk(v, disallowedFields)
				if err != nil {
					return
				}

				newNode = tsl.Node{Func: n.Func, Left: l, Right: r}
			case []tsl.Node:
				// It's a list of nodes, some TSL operators have multiple RHS arguments
				// for example `IN` and `BETWEEN`. If this operator has a list of arguments,
				// loop over the list, and add all nodes.
				var rr []tsl.Node

				// Add all nodes in the right side array.
				for _, e := range v {
					r, err = FieldNameWalk(e, disallowedFields)
					if err != nil {
						return
					}

					rr = append(rr, r)
				}

				newNode = tsl.Node{Func: n.Func, Left: l, Right: rr}
			default:
				// We only support `Node` and `[]Node` types for the right hand side,
				// of TSL operators. If here than this is an unsupported right hand side
				// type.
				err = errors.BadRequest("unsupported right hand side type in search query")
			}
		} else {
			// If here than `n.Right` is nil. This is a legit type of node,
			// we just need to ignore the right hand side, and continue walking the
			// tree.
			newNode = tsl.Node{Func: n.Func, Left: l}
		}
	}

	return
}

// cleanOrderBy takes the orderBy arg and cleans it.
func cleanOrderBy(userArg string, disallowedFields map[string]string) (orderBy string, err *errors.ServiceError) {
	var orderField string

	// We want to accept user params with trailing and leading spaces
	trimedName := strings.Trim(userArg, " ")

	// Each OrderBy can be a "<field-name>" or a "<field-name> asc|desc"
	order := strings.Split(trimedName, " ")
	direction := "none valid"

	if len(order) == 1 {
		orderField, err = getField(order[0], disallowedFields)
		direction = "asc"
	} else if len(order) == 2 {
		orderField, err = getField(order[0], disallowedFields)
		direction = order[1]
	}
	if err != nil || (direction != "asc" && direction != "desc") {
		err = errors.BadRequest("bad order value '%s'", userArg)
		return
	}

	orderBy = fmt.Sprintf("%s %s", orderField, direction)
	return
}

// ArgsToOrderBy returns cleaned orderBy list.
func ArgsToOrderBy(
	orderByArgs []string,
	disallowedFields map[string]string) (orderBy []string, err *errors.ServiceError) {

	var order string
	if len(orderByArgs) != 0 {
		orderBy = []string{}
		for _, o := range orderByArgs {
			order, err = cleanOrderBy(o, disallowedFields)
			if err != nil {
				return
			}

			// If valid add the user entered order by, to the order by list
			orderBy = append(orderBy, order)
		}
	}
	return
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

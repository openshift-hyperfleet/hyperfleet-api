package db

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

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
		return errors.BadRequest("label key '%s' is invalid: must contain only lowercase letters, digits, and underscores", key)
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

// conditionsNodeConverter handles status.conditions.<ConditionType>='<Status>' queries
// Transforms: status.conditions.Ready='True' ->
//   jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Ready")') ->> 'status' = 'True'
// This uses the BTREE expression index on Ready conditions for efficient lookups.
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

	// Extract condition type from path (e.g., "status.conditions.Ready" -> "Ready")
	parts := strings.Split(leftStr, ".")
	if len(parts) != 3 || parts[0] != "status" || parts[1] != "conditions" {
		return nil, errors.BadRequest("invalid condition field path: %s", leftStr)
	}
	conditionType := parts[2]

	// Validate condition type to prevent SQL injection
	if !conditionTypePattern.MatchString(conditionType) {
		return nil, errors.BadRequest("condition type '%s' is invalid: must be PascalCase (e.g., Ready, Available)", conditionType)
	}

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
		return nil, errors.BadRequest("condition status '%s' is invalid: must be True, False, or Unknown", rightStr)
	}

	// Only support equality operator for condition queries
	if n.Func != tsl.EqOp {
		return nil, errors.BadRequest("only equality operator (=) is supported for condition queries")
	}

	// Build query using jsonb_path_query_first to leverage BTREE expression index
	// The index is: CREATE INDEX ... USING BTREE ((jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Ready")')))
	jsonPath := fmt.Sprintf(`$[*] ? (@.type == "%s")`, conditionType)

	return sq.Expr("jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?", jsonPath, rightStr), nil
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
	modifiedTree, err := extractConditionsWalk(n, tableName, &conditions)
	return modifiedTree, conditions, err
}

// extractConditionsWalk recursively walks the tree and extracts condition queries
func extractConditionsWalk(n tsl.Node, tableName string, conditions *[]sq.Sqlizer) (tsl.Node, *errors.ServiceError) {
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
			newLeftNode, err := extractConditionsWalk(v, tableName, conditions)
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
			newRightNode, err := extractConditionsWalk(v, tableName, conditions)
			if err != nil {
				return n, err
			}
			newRight = newRightNode
		case []tsl.Node:
			var newRightNodes []tsl.Node
			for _, rightNode := range v {
				newRightNode, err := extractConditionsWalk(rightNode, tableName, conditions)
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

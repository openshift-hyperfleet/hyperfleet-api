package db

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jinzhu/inflection"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/yaacov/tree-search-language/v6/pkg/tsl"
	"gorm.io/gorm"
)

// Label key validation pattern: only lowercase letters, digits, and underscores to prevent SQL injection
var labelKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// validateFieldKey validates a JSONB field key to prevent SQL injection through field
// name interpolation. Only allows lowercase letters, digits, and underscores.
// fieldType is used in error messages (e.g. "label key", "spec field segment").
func validateFieldKey(key, fieldType string) *errors.ServiceError {
	if key == "" {
		return errors.BadRequest("%s cannot be empty", fieldType)
	}

	if !labelKeyPattern.MatchString(key) {
		return errors.BadRequest(
			"%s '%s' is invalid: must contain only lowercase letters, digits, and underscores", fieldType, key,
		)
	}

	return nil
}

func validateLabelKey(key string) *errors.ServiceError {
	return validateFieldKey(key, "label key")
}

// Field mapping rules for user-friendly syntax to database columns
var statusFieldMappings = map[string]string{
	"status.conditions": "status_conditions",
}

// getField gets the sql field associated with a name.
func getField(name string, disallowedFields map[string]string) (field string, err *errors.ServiceError) {
	trimmedName := strings.Trim(name, " ")

	if strings.HasPrefix(trimmedName, "properties.") {
		if _, disallowed := disallowedFields["properties"]; disallowed {
			err = errors.BadRequest("%s is not a valid field name", name)
			return
		}
		key := strings.TrimPrefix(trimmedName, "properties.")
		if validationErr := validateFieldKey(key, "property key"); validationErr != nil {
			err = validationErr
			return
		}
		field = fmt.Sprintf("properties ->> '%s'", key)
		return
	}

	// Map user-friendly spec.xxx (and nested spec.xxx.yyy...) syntax to JSONB query.
	// v6 gives us the full dotted path directly:
	//   spec.region              → spec->>'region'
	//   spec.release.channel     → spec->'release'->>'channel'
	//   spec.a.b.c              → spec->'a'->'b'->>'c'
	if strings.HasPrefix(trimmedName, "spec.") {
		if _, disallowed := disallowedFields["spec"]; disallowed {
			err = errors.BadRequest("%s is not a valid field name", name)
			return
		}

		parts := strings.Split(strings.TrimPrefix(trimmedName, "spec."), ".")
		for _, part := range parts {
			if validationErr := validateFieldKey(part, "spec field segment"); validationErr != nil {
				err = validationErr
				return
			}
		}

		field = "spec"
		for i, part := range parts {
			if i == len(parts)-1 {
				field += fmt.Sprintf("->>'%s'", part)
			} else {
				field += fmt.Sprintf("->'%s'", part)
			}
		}
		return
	}

	// Map user-friendly labels.xxx syntax to JSONB query: labels->>'xxx'
	if strings.HasPrefix(trimmedName, "labels.") {
		key := strings.TrimPrefix(trimmedName, "labels.")

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

// Condition type validation pattern: PascalCase condition types (e.g., Reconciled, Available, Progressing)
var conditionTypePattern = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)

// Condition status validation: must be True, False, or Unknown
var validConditionStatuses = map[string]bool{
	"True":    true,
	"False":   true,
	"Unknown": true,
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
var comparisonOperators = map[tsl.Operator]string{
	tsl.OpEQ: "=",
	tsl.OpNE: "!=",
	tsl.OpLT: "<",
	tsl.OpLE: "<=",
	tsl.OpGT: ">",
	tsl.OpGE: ">=",
}

// startsWithConditions checks if a field starts with status.conditions.
func startsWithConditions(s string) bool {
	return strings.HasPrefix(s, "status.conditions.")
}

// hasCondition returns true if node has a status.conditions.<Type> identifier on left hand side.
func hasCondition(n *tsl.Node) bool {
	if n.Left == nil || n.Left.Kind != tsl.KindIdentifier {
		return false
	}
	leftStr, ok := n.Left.Value.(string)
	if !ok || !startsWithConditions(leftStr) {
		return false
	}
	return true
}

// conditionsNodeConverter handles condition queries in two forms:
//
// 3-part path (status query): status.conditions.<ConditionType>='<Status>'
// 4-part path (subfield query): status.conditions.<ConditionType>.<Subfield> <op> '<Value>'
func conditionsNodeConverter(n *tsl.Node) (interface{}, *errors.ServiceError) {
	if n.Left == nil || n.Left.Kind != tsl.KindIdentifier {
		return nil, errors.BadRequest("invalid condition query structure")
	}

	leftStr, ok := n.Left.Value.(string)
	if !ok {
		return nil, errors.BadRequest("expected string for left side of condition")
	}

	// v6 gives us the full dotted path: 3 or 4 parts
	parts := strings.Split(leftStr, ".")
	if len(parts) < 3 || len(parts) > 4 || parts[0] != "status" || parts[1] != "conditions" {
		return nil, errors.BadRequest("invalid condition field path: %s", leftStr)
	}

	conditionType := parts[2]

	if !conditionTypePattern.MatchString(conditionType) {
		return nil, errors.BadRequest(
			"condition type '%s' is invalid: must be PascalCase (e.g., Reconciled, Available)", conditionType,
		)
	}

	// 4-part path: subfield query (e.g., status.conditions.Reconciled.last_updated_time)
	if len(parts) == 4 {
		return conditionSubfieldConverter(n, conditionType, parts[3])
	}

	// 3-part path: status query (e.g., status.conditions.Reconciled='True')
	return conditionStatusConverter(n, conditionType)
}

// conditionStatusConverter handles 3-part condition status queries:
// status.conditions.<ConditionType>='<Status>'
func conditionStatusConverter(n *tsl.Node, conditionType string) (interface{}, *errors.ServiceError) {
	if n.Right == nil || n.Right.Kind != tsl.KindStringLiteral {
		return nil, errors.BadRequest("invalid condition query structure: missing right side")
	}

	rightStr, ok := n.Right.Value.(string)
	if !ok {
		return nil, errors.BadRequest("expected string for right side of condition")
	}

	if !validConditionStatuses[rightStr] {
		return nil, errors.BadRequest(
			"condition status '%s' is invalid: must be True, False, or Unknown", rightStr,
		)
	}

	if n.Operator != tsl.OpEQ {
		return nil, errors.BadRequest("only equality operator (=) is supported for condition status queries")
	}

	jsonPath := fmt.Sprintf(`$[*] ? (@.type == "%s")`, conditionType)
	return sq.Expr("jsonb_path_query_first(status_conditions, ?::jsonpath) ->> 'status' = ?", jsonPath, rightStr), nil
}

// conditionSubfieldConverter handles 4-part condition subfield queries:
// status.conditions.<ConditionType>.<Subfield> <op> '<Value>'
func conditionSubfieldConverter(n *tsl.Node, conditionType, subfield string) (interface{}, *errors.ServiceError) {
	sqlOp, ok := comparisonOperators[n.Operator]
	if !ok {
		return nil, errors.BadRequest(
			"operator '%s' is not supported for condition subfield queries; use =, !=, <, <=, >, or >=", n.Operator,
		)
	}

	if n.Right == nil {
		return nil, errors.BadRequest("invalid condition query structure: missing right side")
	}

	jsonPath := fmt.Sprintf(`$[*] ? (@.type == "%s")`, conditionType)

	if conditionTimeSubfields[subfield] {
		var rightStr string
		switch n.Right.Kind {
		case tsl.KindStringLiteral:
			s, ok := n.Right.Value.(string)
			if !ok {
				return nil, errors.BadRequest(
					"expected string timestamp value for condition subfield '%s'", subfield,
				)
			}
			rightStr = s
		case tsl.KindTimestampLiteral:
			// v6 parses RFC3339 strings as time.Time — convert back for our SQL binding
			t, ok := n.Right.Value.(time.Time)
			if !ok {
				return nil, errors.BadRequest(
					"expected timestamp value for condition subfield '%s'", subfield,
				)
			}
			rightStr = t.Format(time.RFC3339Nano)
		default:
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

	if conditionIntSubfields[subfield] {
		if n.Right.Kind != tsl.KindNumericLiteral {
			return nil, errors.BadRequest(
				"expected numeric value for condition subfield '%s'", subfield,
			)
		}
		rightVal, numOk := n.Right.Value.(float64)
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

// ExtractConditionQueries walks the TSL tree and extracts condition queries,
// returning the modified tree (with condition nodes replaced) and the extracted conditions.
func ExtractConditionQueries(n *tsl.Node) (*tsl.Node, []sq.Sqlizer, *errors.ServiceError) {
	var conditions []sq.Sqlizer
	modifiedTree, err := extractConditionsWalk(n, &conditions)
	return modifiedTree, conditions, err
}

// subtreeHasCondition returns true if any node in the subtree is a condition query
func subtreeHasCondition(n *tsl.Node) bool {
	if n == nil {
		return false
	}
	if hasCondition(n) {
		return true
	}
	if subtreeHasCondition(n.Left) {
		return true
	}
	if subtreeHasCondition(n.Right) {
		return true
	}
	return slices.ContainsFunc(n.Children, subtreeHasCondition)
}

// extractConditionsWalk recursively walks the tree and extracts condition queries
func extractConditionsWalk(n *tsl.Node, conditions *[]sq.Sqlizer) (*tsl.Node, *errors.ServiceError) {
	if n == nil {
		return nil, nil
	}

	// NOT is unary in v6: child is in Right, Left is nil
	if n.Kind == tsl.KindUnaryExpr && n.Operator == tsl.OpNot {
		if subtreeHasCondition(n.Right) {
			return n, errors.BadRequest(
				"NOT operator is not supported with condition queries; " +
					"use the inverse condition instead (e.g., Reconciled='False')",
			)
		}
	}

	// OR with condition queries is not supported: extracting conditions from
	// an OR branch and replacing them with 1=1 changes the query semantics
	// (e.g. "name='x' OR condition" becomes "(name='x' OR 1=1) AND condition",
	// which collapses the OR to always-true).
	if n.Kind == tsl.KindBinaryExpr && n.Operator == tsl.OpOr {
		if subtreeHasCondition(n.Left) || subtreeHasCondition(n.Right) {
			return n, errors.BadRequest(
				"OR operator is not supported with condition queries (status.conditions.*); " +
					"use separate requests or combine conditions with AND",
			)
		}
	}

	if hasCondition(n) {
		expr, err := conditionsNodeConverter(n)
		if err != nil {
			return n, err
		}
		sqlizer, ok := expr.(sq.Sqlizer)
		if !ok {
			return n, errors.GeneralError("unexpected type %T in condition expression", expr)
		}
		*conditions = append(*conditions, sqlizer)

		// Replace with a placeholder that always evaluates to true
		return &tsl.Node{
			Kind:     tsl.KindBinaryExpr,
			Operator: tsl.OpEQ,
			Left:     &tsl.Node{Kind: tsl.KindNumericLiteral, Value: float64(1)},
			Right:    &tsl.Node{Kind: tsl.KindNumericLiteral, Value: float64(1)},
		}, nil
	}

	// For non-condition nodes, recursively process children
	var newLeft, newRight *tsl.Node

	if n.Left != nil {
		var err *errors.ServiceError
		newLeft, err = extractConditionsWalk(n.Left, conditions)
		if err != nil {
			return n, err
		}
	}

	if n.Right != nil {
		var err *errors.ServiceError
		newRight, err = extractConditionsWalk(n.Right, conditions)
		if err != nil {
			return n, err
		}
	}

	var newChildren []*tsl.Node
	for _, child := range n.Children {
		newChild, childErr := extractConditionsWalk(child, conditions)
		if childErr != nil {
			return n, childErr
		}
		newChildren = append(newChildren, newChild)
	}

	return &tsl.Node{
		Kind:     n.Kind,
		Operator: n.Operator,
		Value:    n.Value,
		Left:     newLeft,
		Right:    newRight,
		Children: newChildren,
		Position: n.Position,
	}, nil
}

// FieldNameWalk walks the filter tree, maps user-facing field names to SQL columns
// via getField, then wraps spec JSONB numeric comparisons in CAST.
func FieldNameWalk(
	n *tsl.Node,
	disallowedFields map[string]string,
) (*tsl.Node, *errors.ServiceError) {
	mapped, err := IdentWalk(n, func(name string) (string, error) {
		field, svcErr := getField(name, disallowedFields)
		if svcErr != nil {
			return "", svcErr
		}
		return field, nil
	})
	if err != nil {
		if svcErr, ok := err.(*errors.ServiceError); ok {
			return n, svcErr
		}
		return n, errors.BadRequest("%s", err.Error())
	}
	return wrapSpecNumericCasts(mapped), nil
}

// wrapSpecNumericCasts wraps spec JSONB fields in CAST(... AS numeric) when
// compared against a number, so multi-digit values sort correctly.
func wrapSpecNumericCasts(n *tsl.Node) *tsl.Node {
	if n == nil {
		return nil
	}
	if n.Kind == tsl.KindBinaryExpr {
		l := wrapSpecNumericCasts(n.Left)
		r := wrapSpecNumericCasts(n.Right)
		if _, isCmp := comparisonOperators[n.Operator]; isCmp &&
			l != nil && r != nil &&
			l.Kind == tsl.KindIdentifier && r.Kind == tsl.KindNumericLiteral {
			if field, ok := l.Value.(string); ok && strings.HasPrefix(field, "spec->") {
				l = &tsl.Node{Kind: tsl.KindIdentifier, Value: fmt.Sprintf("CAST(%s AS numeric)", field)}
			}
		}
		if _, isCmp := comparisonOperators[n.Operator]; isCmp &&
			l != nil && r != nil &&
			l.Kind == tsl.KindNumericLiteral && r.Kind == tsl.KindIdentifier {
			if field, ok := r.Value.(string); ok && strings.HasPrefix(field, "spec->") {
				r = &tsl.Node{Kind: tsl.KindIdentifier, Value: fmt.Sprintf("CAST(%s AS numeric)", field)}
			}
		}
		return &tsl.Node{Kind: n.Kind, Operator: n.Operator, Left: l, Right: r, Position: n.Position}
	}
	if n.Kind == tsl.KindUnaryExpr {
		return &tsl.Node{Kind: n.Kind, Operator: n.Operator, Right: wrapSpecNumericCasts(n.Right), Position: n.Position}
	}
	return n
}

// IdentWalk walks the tree and replaces identifier values using the check function.
// Unlike v6's ident.Walk, this does NOT re-parse the return value — safe for JSONB expressions.
func IdentWalk(n *tsl.Node, check func(string) (string, error)) (*tsl.Node, error) {
	if n == nil {
		return nil, nil
	}

	switch n.Kind {
	case tsl.KindIdentifier:
		s, ok := n.Value.(string)
		if !ok {
			return nil, fmt.Errorf("identifier value is not a string")
		}
		v, err := check(s)
		if err != nil {
			return nil, err
		}
		return &tsl.Node{Kind: tsl.KindIdentifier, Value: v, Position: n.Position}, nil

	case tsl.KindStringLiteral, tsl.KindNumericLiteral, tsl.KindDateLiteral,
		tsl.KindTimestampLiteral, tsl.KindBooleanLiteral, tsl.KindNullLiteral:
		return n, nil

	case tsl.KindBinaryExpr:
		newLeft, err := IdentWalk(n.Left, check)
		if err != nil {
			return nil, err
		}
		newRight, err := IdentWalk(n.Right, check)
		if err != nil {
			return nil, err
		}
		return &tsl.Node{Kind: n.Kind, Operator: n.Operator, Left: newLeft, Right: newRight, Position: n.Position}, nil

	case tsl.KindUnaryExpr:
		newRight, err := IdentWalk(n.Right, check)
		if err != nil {
			return nil, err
		}
		return &tsl.Node{Kind: n.Kind, Operator: n.Operator, Right: newRight, Position: n.Position}, nil

	case tsl.KindArrayLiteral:
		// Array elements in IN [...] are values (literals), not column names.
		// Don't run them through the check function — it would try to resolve
		// them as field names and either mangle or reject valid values.
		return n, nil

	default:
		return n, nil
	}
}

// cleanOrderBy takes the orderBy arg and cleans it.
func cleanOrderBy(userArg string, disallowedFields map[string]string) (orderBy string, err *errors.ServiceError) {
	var orderField string

	trimedName := strings.Trim(userArg, " ")

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
	disallowedFields map[string]string,
) (orderBy []string, err *errors.ServiceError) {
	var order string
	if len(orderByArgs) != 0 {
		orderBy = []string{}
		for _, o := range orderByArgs {
			order, err = cleanOrderBy(o, disallowedFields)
			if err != nil {
				return
			}

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

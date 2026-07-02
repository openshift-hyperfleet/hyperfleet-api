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

// jsonbKeyPattern guards keys interpolated into JSONB paths (spec->>'%s', properties->>'%s').
var jsonbKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func validateJSONBKey(key, fieldType string) *errors.ServiceError {
	if key == "" {
		return errors.BadRequest("%s cannot be empty", fieldType)
	}

	if !jsonbKeyPattern.MatchString(key) {
		return errors.BadRequest(
			"%s '%s' is invalid: must contain only lowercase letters, digits, and underscores", fieldType, key,
		)
	}

	return nil
}

// TODO(HYPERFLEET-1159): remove once Cluster/NodePool are migrated to Resource-kind entities.
func validateLegacyLabelKey(key string) *errors.ServiceError {
	return validateJSONBKey(key, "label key")
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
		if validationErr := validateJSONBKey(key, "property key"); validationErr != nil {
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
			if validationErr := validateJSONBKey(part, "spec field segment"); validationErr != nil {
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

	// TODO(HYPERFLEET-1159): remove this legacy JSONB branch once Cluster/NodePool
	// are migrated to Resource-kind entities with the resource_labels table.
	if strings.HasPrefix(trimmedName, "labels.") {
		if _, disallowed := disallowedFields["labels"]; disallowed {
			err = errors.BadRequest("%s is not a valid field name", name)
			return
		}
		key := strings.TrimPrefix(trimmedName, "labels.")

		if validationErr := validateLegacyLabelKey(key); validationErr != nil {
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
	modifiedTree, err := extractMatchingQueries(
		n, hasCondition, conditionsNodeConverter,
		"NOT operator is not supported with condition queries; "+
			"use the inverse condition instead (e.g., Reconciled='False')",
		"OR operator is not supported with condition queries (status.conditions.*); "+
			"use separate requests or combine conditions with AND",
		&conditions,
	)
	return modifiedTree, conditions, err
}

// subtreeHasMatch returns true if any node in the subtree satisfies predicate.
func subtreeHasMatch(n *tsl.Node, predicate func(*tsl.Node) bool) bool {
	if n == nil {
		return false
	}
	if predicate(n) {
		return true
	}
	if subtreeHasMatch(n.Left, predicate) {
		return true
	}
	if subtreeHasMatch(n.Right, predicate) {
		return true
	}
	return slices.ContainsFunc(n.Children, func(c *tsl.Node) bool { return subtreeHasMatch(c, predicate) })
}

// extractMatchingQueries recursively walks the tree, replacing every node matching
// predicate with a `1=1` placeholder and collecting converter(node) into exprs.
// NOT and OR are rejected over a subtree containing a match: extracting a match from
// an OR/NOT branch and replacing it with 1=1 changes the query semantics (e.g.
// "name='x' OR condition" becomes "(name='x' OR 1=1) AND condition", which collapses
// the OR to always-true). notMsg/orMsg are the operator-specific error messages.
func extractMatchingQueries(
	n *tsl.Node,
	predicate func(*tsl.Node) bool,
	converter func(*tsl.Node) (any, *errors.ServiceError),
	notMsg, orMsg string,
	exprs *[]sq.Sqlizer,
) (*tsl.Node, *errors.ServiceError) {
	if n == nil {
		return nil, nil
	}

	// NOT is unary in v6: child is in Right, Left is nil
	if n.Kind == tsl.KindUnaryExpr && n.Operator == tsl.OpNot {
		if subtreeHasMatch(n.Right, predicate) {
			return n, errors.BadRequest("%s", notMsg)
		}
	}

	if n.Kind == tsl.KindBinaryExpr && n.Operator == tsl.OpOr {
		if subtreeHasMatch(n.Left, predicate) || subtreeHasMatch(n.Right, predicate) {
			return n, errors.BadRequest("%s", orMsg)
		}
	}

	if predicate(n) {
		expr, err := converter(n)
		if err != nil {
			return n, err
		}
		sqlizer, ok := expr.(sq.Sqlizer)
		if !ok {
			return n, errors.GeneralError("unexpected type %T in extracted expression", expr)
		}
		*exprs = append(*exprs, sqlizer)

		// Replace with a placeholder that always evaluates to true
		return &tsl.Node{
			Kind:     tsl.KindBinaryExpr,
			Operator: tsl.OpEQ,
			Left:     &tsl.Node{Kind: tsl.KindNumericLiteral, Value: float64(1)},
			Right:    &tsl.Node{Kind: tsl.KindNumericLiteral, Value: float64(1)},
		}, nil
	}

	// For non-matching nodes, recursively process children
	var newLeft, newRight *tsl.Node

	if n.Left != nil {
		var err *errors.ServiceError
		newLeft, err = extractMatchingQueries(n.Left, predicate, converter, notMsg, orMsg, exprs)
		if err != nil {
			return n, err
		}
	}

	if n.Right != nil {
		var err *errors.ServiceError
		newRight, err = extractMatchingQueries(n.Right, predicate, converter, notMsg, orMsg, exprs)
		if err != nil {
			return n, err
		}
	}

	var newChildren []*tsl.Node
	for _, child := range n.Children {
		newChild, childErr := extractMatchingQueries(child, predicate, converter, notMsg, orMsg, exprs)
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

// hasLabel returns true if node has a labels.<key> identifier on the left hand side.
func hasLabel(n *tsl.Node) bool {
	if n.Left == nil || n.Left.Kind != tsl.KindIdentifier {
		return false
	}
	leftStr, ok := n.Left.Value.(string)
	if !ok {
		return false
	}
	return strings.HasPrefix(leftStr, "labels.")
}

func labelsNodeConverter(n *tsl.Node, resourceTable string) (any, *errors.ServiceError) {
	if n.Left == nil || n.Left.Kind != tsl.KindIdentifier {
		return nil, errors.BadRequest("invalid label query structure")
	}

	leftStr, ok := n.Left.Value.(string)
	if !ok {
		return nil, errors.BadRequest("expected string for left side of label query")
	}

	parts := strings.Split(leftStr, ".")
	if len(parts) != 2 || parts[0] != "labels" {
		return nil, errors.BadRequest("invalid label field path: %s", leftStr)
	}

	key := parts[1]

	// Supported operators for label queries on the resource_labels table:
	// =, != use a simple value comparison; IN uses array binding.
	// Other operators (LIKE, range) would need different SQL patterns
	// and can be added when needed.
	switch n.Operator {
	case tsl.OpEQ, tsl.OpNE:
		sqlOp := comparisonOperators[n.Operator]

		if n.Right == nil || n.Right.Kind != tsl.KindStringLiteral {
			return nil, errors.BadRequest("expected string value for label '%s'", key)
		}
		rightStr, ok := n.Right.Value.(string)
		if !ok {
			return nil, errors.BadRequest("expected string value for label '%s'", key)
		}

		query := fmt.Sprintf(
			"EXISTS (SELECT 1 FROM resource_labels WHERE resource_labels.resource_id = %s.id "+
				"AND resource_labels.key = ? AND resource_labels.value %s ?)",
			resourceTable, sqlOp,
		)
		return sq.Expr(query, key, rightStr), nil

	case tsl.OpIn:
		if n.Right == nil || n.Right.Kind != tsl.KindArrayLiteral {
			return nil, errors.BadRequest("expected array value for label '%s' IN query", key)
		}
		values := make([]interface{}, 0, len(n.Right.Children))
		for _, child := range n.Right.Children {
			s, ok := child.Value.(string)
			if !ok {
				return nil, errors.BadRequest("expected string values in label '%s' IN list", key)
			}
			values = append(values, s)
		}
		if len(values) == 0 {
			return nil, errors.BadRequest("empty IN list for label '%s'", key)
		}

		query := fmt.Sprintf(
			"EXISTS (SELECT 1 FROM resource_labels WHERE resource_labels.resource_id = %s.id "+
				"AND resource_labels.key = ? AND resource_labels.value IN (%s))",
			resourceTable, sq.Placeholders(len(values)),
		)
		args := make([]interface{}, 0, 1+len(values))
		args = append(args, key)
		args = append(args, values...)
		return sq.Expr(query, args...), nil

	default:
		return nil, errors.BadRequest(
			"operator '%s' is not supported for label queries; use =, !=, or IN", n.Operator,
		)
	}
}

// isLabelIdentifier returns true for a bare labels.<key> identifier node, regardless
// of its position in the tree — unlike hasLabel, which only matches when the
// identifier is the direct left side of a comparison.
func isLabelIdentifier(n *tsl.Node) bool {
	if n == nil || n.Kind != tsl.KindIdentifier {
		return false
	}
	s, ok := n.Value.(string)
	return ok && strings.HasPrefix(s, "labels.")
}

func ExtractLabelQueries(n *tsl.Node, resourceTable string) (*tsl.Node, []sq.Sqlizer, *errors.ServiceError) {
	var labels []sq.Sqlizer
	converter := func(n *tsl.Node) (any, *errors.ServiceError) {
		return labelsNodeConverter(n, resourceTable)
	}
	modifiedTree, err := extractMatchingQueries(
		n, hasLabel, converter,
		"NOT operator is not supported with label queries",
		"OR operator is not supported with label queries (labels.*); "+
			"use separate requests or combine label filters with AND",
		&labels,
	)
	if err != nil {
		return n, nil, err
	}

	// "not labels.env='x'" parses as "(NOT labels.env) = 'x'" due to TSL precedence,
	// so hasLabel never matches it and it survives into modifiedTree. Without this
	// guard it falls through to getField's JSONB branch and hits a missing column.
	if subtreeHasMatch(modifiedTree, isLabelIdentifier) {
		return n, nil, errors.BadRequest(
			"labels.<key> must be used in a direct comparison, e.g. labels.env=\"prod\"; " +
				"wrap NOT in parentheses, e.g. not (labels.env=\"prod\")",
		)
	}

	return modifiedTree, labels, nil
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

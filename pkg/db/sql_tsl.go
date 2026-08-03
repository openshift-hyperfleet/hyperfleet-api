// sql_tsl.go implements a custom TSL-to-SQL walker instead of using the
// library's built-in SQL emitter. We need this because label and condition
// queries are resolved as scalar subqueries against separate tables
// (resource_labels, resource_conditions), JSONB spec fields require dynamic
// CAST wrapping for numeric comparisons, and certain operators (NOT on
// labels/conditions) must be rejected at walk time to prevent semantically
// broken SQL. The built-in walker has no hooks for any of this.
package db

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/yaacov/tree-search-language/v6/pkg/tsl"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

const (
	resourceLabelsTable     = "resource_labels"
	resourceConditionsTable = "resource_conditions"
	conditionStatusField    = "status"
)

// jsonbKeyPattern guards keys interpolated into JSONB paths (spec->>'%s', properties->>'%s').
// Must be numbers, lowercase letters, or underscores
var jsonbKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// conditionTypePattern validates condition type pattern: PascalCase condition types
// (e.g., Reconciled, Available, Progressing)
var conditionTypePattern = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)

// conditionAllowedStatuses condition status must be True, False, or Unknown
var conditionAllowedStatuses = map[string]bool{
	"True":    true,
	"False":   true,
	"Unknown": true,
}

// conditionAllowedSubfields lists the condition subfields that can be queried.
// created_time is intentionally excluded — it reflects when the condition was first created
// and is not useful for Sentinel polling or staleness queries.
var conditionAllowedSubfields = map[string]bool{
	"last_updated_time":    true,
	"last_transition_time": true,
	"observed_generation":  true,
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

func prefixStatusConditions(s string) bool {
	return strings.HasPrefix(s, "status.conditions.")
}

func prefixSpec(s string) bool {
	return strings.HasPrefix(s, "spec.")
}

func prefixLabels(s string) bool {
	return strings.HasPrefix(s, "labels.")
}

// searchTypedFields declares the expected TSL literal kind for top-level search
// columns whose underlying SQL type does not accept arbitrary text.
var searchTypedFields = map[string]tsl.Kind{
	"generation":   tsl.KindNumericLiteral,
	"created_time": tsl.KindTimestampLiteral,
	"updated_time": tsl.KindTimestampLiteral,
	"deleted_time": tsl.KindTimestampLiteral,
}

var typedKindHints = map[tsl.Kind]string{
	tsl.KindNumericLiteral:   "an integer",
	tsl.KindTimestampLiteral: "an RFC3339 timestamp (e.g. 2026-01-01T00:00:00Z)",
}

// WalkConfig provides table context and a hook for related-table resolution.
type WalkConfig struct {
	ResolveRelated func(name string) (string, error)
	TableName      string
}

type walkContext struct {
	cfg               WalkConfig
	conditionSubfield string
	inNot             bool
}

func isConditionNode(n *tsl.TSLNode) bool {
	if n == nil || n.Type() != tsl.KindIdentifier {
		return false
	}
	name, _ := n.AsString()
	return prefixStatusConditions(name)
}

// TSLToSQL walks the TSL tree (read-only) and emits a parameterized SQL
// WHERE fragment. Labels and conditions are resolved inline as scalar
// subqueries; JSONB mapping, CAST wrapping, and table-name prefixing all
// happen during emission.
func TSLToSQL(node *tsl.TSLNode, cfg WalkConfig) (string, []any, *errors.ServiceError) {
	ctx := &walkContext{cfg: cfg}
	return walkNode(node, ctx)
}

func walkNode(n *tsl.TSLNode, ctx *walkContext) (string, []any, *errors.ServiceError) {
	if n == nil {
		return "", nil, nil
	}

	switch n.Type() {
	case tsl.KindIdentifier:
		return resolveColumn(n, ctx)
	case tsl.KindStringLiteral:
		s, _ := n.AsString()
		return "?", []any{s}, nil
	case tsl.KindNumericLiteral:
		f, _ := n.AsFloat64()
		return "?", []any{f}, nil
	case tsl.KindBooleanLiteral:
		b, _ := n.AsBool()
		return "?", []any{b}, nil
	case tsl.KindTimestampLiteral:
		v := n.Value()
		if t, ok := v.(time.Time); ok {
			return "?", []any{t.Format(time.RFC3339Nano)}, nil
		}
		return "?", []any{v}, nil
	case tsl.KindDateLiteral:
		s, _ := n.AsString()
		return "?", []any{s}, nil
	case tsl.KindNullLiteral:
		return "NULL", nil, nil
	case tsl.KindBinaryExpr, tsl.KindUnaryExpr:
		return walkNaryExpr(n, ctx)
	case tsl.KindArrayLiteral:
		return walkArrayLiteral(n, ctx)
	default:
		return "", nil, errors.BadRequest("unsupported node type in search query")
	}
}

func resolveColumn(n *tsl.TSLNode, ctx *walkContext) (string, []any, *errors.ServiceError) {
	name, _ := n.AsString()

	// labels...
	if prefixLabels(name) {
		if ctx.inNot {
			return "", nil, errors.BadRequest(
				"NOT operator is not supported with label queries")
		}
		return resolveLabelColumn(name, ctx)
	}
	// status.conditions...
	if prefixStatusConditions(name) {
		if ctx.inNot {
			return "", nil, errors.BadRequest(
				"NOT operator is not supported with condition queries")
		}
		return resolveStatusConditionColumn(name, ctx)
	}
	// spec...
	if prefixSpec(name) {
		return resolveSpecColumn(name, ctx)
	}
	// any other field...
	// status.created_time , name, id etc..
	return resolveField(name, ctx)
}

func resolveLabelColumn(name string, ctx *walkContext) (string, []any, *errors.ServiceError) {
	key, _ := strings.CutPrefix(name, "labels.")
	if key == "" {
		return "", nil, errors.BadRequest("label key cannot be empty")
	}
	return fmt.Sprintf(
		"(SELECT value FROM %s WHERE %s.resource_id = %s.id AND %s.key = ?)",
		resourceLabelsTable, resourceLabelsTable, ctx.cfg.TableName, resourceLabelsTable,
	), []any{key}, nil
}

func resolveStatusConditionColumn(name string, ctx *walkContext) (string, []any, *errors.ServiceError) {
	typeFull, _ := strings.CutPrefix(name, "status.conditions.")
	if typeFull == "" {
		return "", nil, errors.BadRequest("condition type cannot be empty")
	}

	// e.g. "Reconciled" or "Reconciled.last_updated_time"
	typeParts := strings.Split(typeFull, ".")
	if len(typeParts) > 2 {
		return "", nil, errors.BadRequest(
			"invalid condition format: expected status.conditions.<Type> or status.conditions.<Type>.<subfield>")
	}

	typeName := typeParts[0]
	if !conditionTypePattern.MatchString(typeName) {
		return "", nil, errors.BadRequest(
			"condition type '%s' is invalid: must be PascalCase (e.g., Reconciled, Available)",
			typeName,
		)
	}

	subfield := conditionStatusField
	if len(typeParts) == 2 {
		subfield = typeParts[1]
		if !conditionAllowedSubfields[subfield] {
			return "", nil, errors.BadRequest(
				"condition subfield '%s' is not supported; "+
					"use last_updated_time, last_transition_time, or observed_generation",
				subfield,
			)
		}
	}

	ctx.conditionSubfield = subfield

	sql := fmt.Sprintf(
		"(SELECT rc.%s FROM %s rc WHERE rc.resource_id = %s.id AND rc.type = ?)",
		subfield, resourceConditionsTable, ctx.cfg.TableName,
	)
	return sql, []any{typeName}, nil
}

func resolveSpecColumn(name string, _ *walkContext) (string, []any, *errors.ServiceError) {
	specPath, _ := strings.CutPrefix(name, "spec.")
	parts := strings.Split(specPath, ".")
	for _, part := range parts {
		if validationErr := validateJSONBKey(part, "spec field segment"); validationErr != nil {
			return "", nil, validationErr
		}
	}

	var field strings.Builder
	fmt.Fprintf(&field, "spec")
	for i, part := range parts {
		if i == len(parts)-1 {
			fmt.Fprintf(&field, "->>'%s'", part)
		} else {
			fmt.Fprintf(&field, "->'%s'", part)
		}
	}

	return field.String(), nil, nil
}

func resolveField(name string, ctx *walkContext) (string, []any, *errors.ServiceError) {
	trimmedName := strings.TrimSpace(name)
	fieldParts := strings.Split(trimmedName, ".")

	if len(fieldParts) == 1 {
		if validationErr := validateJSONBKey(fieldParts[0], "field"); validationErr != nil {
			return "", nil, errors.BadRequest("%s is not a valid field name", name)
		}
		return fmt.Sprintf("%s.%s", ctx.cfg.TableName, trimmedName), nil, nil
	}

	for _, part := range fieldParts {
		if validationErr := validateJSONBKey(part, "field"); validationErr != nil {
			return "", nil, errors.BadRequest("%s is not a valid field name", name)
		}
	}

	if ctx.cfg.ResolveRelated != nil {
		resolved, relErr := ctx.cfg.ResolveRelated(name)
		if relErr != nil {
			return "", nil, errors.BadRequest("%s", relErr.Error())
		}
		return resolved, nil, nil
	}

	return "", nil, errors.BadRequest("%s is not a valid field name", name)
}

func walkNaryExpr(n *tsl.TSLNode, ctx *walkContext) (string, []any, *errors.ServiceError) {
	op, _ := n.AsExprOp()

	switch op.Operator {
	case tsl.OpAnd, tsl.OpOr:
		return walkLogical(op, ctx)
	case tsl.OpIn:
		return walkIn(op, ctx)
	case tsl.OpBetween:
		return walkBetween(op, ctx)
	case tsl.OpIs:
		return walkIsNull(op, ctx)
	case tsl.OpLike, tsl.OpILike:
		return walkStringMatch(op, ctx)
	case tsl.OpNot:
		childCtx := &walkContext{cfg: ctx.cfg, inNot: true}
		childSQL, childArgs, err := walkNode(op.Right, childCtx)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("NOT (%s)", childSQL), childArgs, nil
	case tsl.OpUMinus:
		childSQL, childArgs, err := walkNode(op.Right, ctx)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("(-%s)", childSQL), childArgs, nil

	default:
		return walkComparison(op, ctx)
	}
}

func walkLogical(op tsl.TSLExpressionOp, ctx *walkContext) (string, []any, *errors.ServiceError) {
	leftSQL, leftArgs, err := walkNode(op.Left, ctx)
	if err != nil {
		return "", nil, err
	}
	rightSQL, rightArgs, err := walkNode(op.Right, ctx)
	if err != nil {
		return "", nil, err
	}

	var keyword string
	switch op.Operator {
	case tsl.OpAnd:
		keyword = "AND"
	case tsl.OpOr:
		keyword = "OR"
	default:
		return "", nil, errors.BadRequest("unsupported logical operator: %s", op.Operator)
	}

	return fmt.Sprintf("(%s) %s (%s)", leftSQL, keyword, rightSQL),
		append(leftArgs, rightArgs...), nil
}

func walkComparison(op tsl.TSLExpressionOp, ctx *walkContext) (string, []any, *errors.ServiceError) { //nolint:cyclop
	if svcErr := validateTypedSide(op.Left, op.Right); svcErr != nil {
		return "", nil, svcErr
	}
	if svcErr := validateTypedSide(op.Right, op.Left); svcErr != nil {
		return "", nil, svcErr
	}

	leftSQL, leftArgs, err := walkNode(op.Left, ctx)
	if err != nil {
		return "", nil, err
	}
	rightSQL, rightArgs, err := walkNode(op.Right, ctx)
	if err != nil {
		return "", nil, err
	}

	sqlOp, ok := comparisonOperators[op.Operator]
	if !ok {
		return "", nil, errors.BadRequest("unsupported comparison operator: %s", op.Operator)
	}

	if strings.HasPrefix(leftSQL, "spec->") && len(rightArgs) > 0 {
		if _, isNum := rightArgs[0].(float64); isNum {
			leftSQL = fmt.Sprintf("CAST(%s AS numeric)", leftSQL)
		}
	}
	if strings.HasPrefix(rightSQL, "spec->") && len(leftArgs) > 0 {
		if _, isNum := leftArgs[0].(float64); isNum {
			rightSQL = fmt.Sprintf("CAST(%s AS numeric)", rightSQL)
		}
	}

	if ctx.conditionSubfield != "" {
		valueArgs := rightArgs
		if isConditionNode(op.Right) {
			valueArgs = leftArgs
		}
		if svcErr := validateConditionComparison(op, ctx.conditionSubfield, valueArgs); svcErr != nil {
			return "", nil, svcErr
		}
		ctx.conditionSubfield = ""
	}

	return fmt.Sprintf("%s %s %s", leftSQL, sqlOp, rightSQL),
		append(leftArgs, rightArgs...), nil
}

func walkIn(op tsl.TSLExpressionOp, ctx *walkContext) (string, []any, *errors.ServiceError) {
	if isConditionNode(op.Left) {
		return "", nil, errors.BadRequest(
			"IN is not supported for condition queries; use comparison operators (=, !=, <, <=, >, >=)")
	}
	if svcErr := validateTypedArray(op.Left, op.Right); svcErr != nil {
		return "", nil, svcErr
	}

	leftSQL, leftArgs, err := walkNode(op.Left, ctx)
	if err != nil {
		return "", nil, err
	}

	arr, ok := op.Right.AsArray()
	if !ok {
		return "", nil, errors.BadRequest("expected array on right side of IN")
	}

	placeholders := make([]string, len(arr.Values))
	var rightArgs []any
	for i, v := range arr.Values {
		s, a, walkErr := walkNode(v, ctx)
		if walkErr != nil {
			return "", nil, walkErr
		}
		placeholders[i] = s
		rightArgs = append(rightArgs, a...)
	}

	if strings.HasPrefix(leftSQL, "spec->") && len(rightArgs) > 0 {
		allNumeric := true
		for _, arg := range rightArgs {
			if _, isNum := arg.(float64); !isNum {
				allNumeric = false
				break
			}
		}
		if allNumeric {
			leftSQL = fmt.Sprintf("CAST(%s AS numeric)", leftSQL)
		}
	}

	return fmt.Sprintf("%s IN (%s)", leftSQL, strings.Join(placeholders, ", ")),
		append(leftArgs, rightArgs...), nil
}

func walkBetween(op tsl.TSLExpressionOp, ctx *walkContext) (string, []any, *errors.ServiceError) {
	if isConditionNode(op.Left) {
		return "", nil, errors.BadRequest(
			"BETWEEN is not supported for condition queries; use comparison operators (=, !=, <, <=, >, >=)")
	}
	leftSQL, leftArgs, err := walkNode(op.Left, ctx)
	if err != nil {
		return "", nil, err
	}

	arr, ok := op.Right.AsArray()
	if !ok || len(arr.Values) != 2 {
		return "", nil, errors.BadRequest("BETWEEN requires exactly 2 values")
	}

	lowSQL, lowArgs, err := walkNode(arr.Values[0], ctx)
	if err != nil {
		return "", nil, err
	}
	highSQL, highArgs, err := walkNode(arr.Values[1], ctx)
	if err != nil {
		return "", nil, err
	}

	if strings.HasPrefix(leftSQL, "spec->") && len(lowArgs) > 0 && len(highArgs) > 0 {
		_, lowIsNum := lowArgs[0].(float64)
		_, highIsNum := highArgs[0].(float64)
		if lowIsNum && highIsNum {
			leftSQL = fmt.Sprintf("CAST(%s AS numeric)", leftSQL)
		}
	}

	var args []any
	args = append(args, leftArgs...)
	args = append(args, lowArgs...)
	args = append(args, highArgs...)
	return fmt.Sprintf("%s BETWEEN %s AND %s", leftSQL, lowSQL, highSQL), args, nil
}

func walkIsNull(op tsl.TSLExpressionOp, ctx *walkContext) (string, []any, *errors.ServiceError) {
	leftSQL, leftArgs, err := walkNode(op.Left, ctx)
	if err != nil {
		return "", nil, err
	}
	ctx.conditionSubfield = ""
	return fmt.Sprintf("%s IS NULL", leftSQL), leftArgs, nil
}

func walkStringMatch(op tsl.TSLExpressionOp, ctx *walkContext) (string, []any, *errors.ServiceError) {
	if isConditionNode(op.Left) {
		return "", nil, errors.BadRequest(
			"LIKE/ILIKE is not supported for condition queries; use comparison operators (=, !=, <, <=, >, >=)")
	}
	leftSQL, leftArgs, err := walkNode(op.Left, ctx)
	if err != nil {
		return "", nil, err
	}
	rightSQL, rightArgs, err := walkNode(op.Right, ctx)
	if err != nil {
		return "", nil, err
	}

	keyword := "LIKE"
	if op.Operator == tsl.OpILike {
		keyword = "ILIKE"
	}

	return fmt.Sprintf("%s %s %s", leftSQL, keyword, rightSQL),
		append(leftArgs, rightArgs...), nil
}

func walkArrayLiteral(n *tsl.TSLNode, ctx *walkContext) (string, []any, *errors.ServiceError) {
	arr, ok := n.AsArray()
	if !ok {
		return "", nil, errors.BadRequest("expected array")
	}

	placeholders := make([]string, len(arr.Values))
	var args []any
	for i, v := range arr.Values {
		s, a, err := walkNode(v, ctx)
		if err != nil {
			return "", nil, err
		}
		placeholders[i] = s
		args = append(args, a...)
	}

	return fmt.Sprintf("(%s)", strings.Join(placeholders, ", ")), args, nil
}

// validateTypedSide checks that if ident is a typed field (generation, timestamps),
// the value node has the correct TSL literal kind.
func validateTypedSide(ident, value *tsl.TSLNode) *errors.ServiceError {
	if ident == nil || ident.Type() != tsl.KindIdentifier {
		return nil
	}
	field, ok := ident.AsString()
	if !ok {
		return nil
	}
	wantKind, tracked := searchTypedFields[field]
	if !tracked {
		return nil
	}
	return validateTypedNode(field, wantKind, value)
}

func validateTypedNode(field string, wantKind tsl.Kind, value *tsl.TSLNode) *errors.ServiceError {
	if value == nil {
		return errors.BadRequest("missing value for field '%s'", field)
	}
	if value.Type() == tsl.KindNullLiteral {
		return nil
	}
	kind := value.Type()
	if kind == wantKind || (wantKind == tsl.KindTimestampLiteral && kind == tsl.KindDateLiteral) {
		return nil
	}
	hint, ok := typedKindHints[wantKind]
	if !ok {
		hint = fmt.Sprintf("a value of kind %s", wantKind)
	}
	return errors.BadRequest("field '%s' expects %s", field, hint)
}

// validateTypedArray validates all elements in an array node against a typed field.
func validateTypedArray(ident, arr *tsl.TSLNode) *errors.ServiceError {
	if ident == nil || ident.Type() != tsl.KindIdentifier {
		return nil
	}
	field, ok := ident.AsString()
	if !ok {
		return nil
	}
	wantKind, tracked := searchTypedFields[field]
	if !tracked {
		return nil
	}
	arrayLit, ok := arr.AsArray()
	if !ok {
		return nil
	}
	for _, child := range arrayLit.Values {
		if svcErr := validateTypedNode(field, wantKind, child); svcErr != nil {
			return svcErr
		}
	}
	return nil
}

func validateConditionComparison(
	op tsl.TSLExpressionOp, subfield string, rightArgs []any,
) *errors.ServiceError {
	if len(rightArgs) == 0 {
		return nil
	}

	switch subfield {
	case conditionStatusField:
		if op.Operator != tsl.OpEQ {
			return errors.BadRequest(
				"only equality operator (=) is supported for condition status queries")
		}
		if s, ok := rightArgs[0].(string); ok && !conditionAllowedStatuses[s] {
			return errors.BadRequest(
				"condition status '%s' is invalid: must be True, False, or Unknown", s)
		}

	case "last_updated_time", "last_transition_time":
		if _, ok := comparisonOperators[op.Operator]; !ok {
			return errors.BadRequest(
				"operator '%s' is not supported for condition subfield queries; use =, !=, <, <=, >, or >=",
				op.Operator,
			)
		}
		if s, ok := rightArgs[0].(string); ok {
			if _, parseErr := time.Parse(time.RFC3339, s); parseErr != nil {
				return errors.BadRequest(
					"invalid timestamp for condition subfield: " +
						"expected RFC3339 format (e.g., 2026-01-01T00:00:00Z)")
			}
		}

	case "observed_generation":
		if _, ok := comparisonOperators[op.Operator]; !ok {
			return errors.BadRequest(
				"operator '%s' is not supported for condition subfield queries; use =, !=, <, <=, >, or >=",
				op.Operator,
			)
		}
		if f, ok := rightArgs[0].(float64); ok {
			if f != math.Trunc(f) {
				return errors.BadRequest(
					"expected integer value for condition subfield 'observed_generation', got %v", f)
			}
			if f < math.MinInt32 || f > math.MaxInt32 {
				return errors.BadRequest(
					"value %v is out of 32-bit integer range for condition subfield 'observed_generation'", f)
			}
		}

	default:
		return errors.BadRequest(
			"condition subfield '%s' has no comparison validation configured", subfield)
	}

	return nil
}

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

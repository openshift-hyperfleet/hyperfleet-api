package handlers

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

const defaultOrder = "created_time desc"

type listParams struct {
	RefType     string `validate:"required_with=RefTargetID"`
	RefTargetID string `validate:"required_with=RefType"`
	Size        int64  `validate:"min=1,max=100"`
	Page        int    `validate:"min=1,max=10000000"`
}

var listParamsValidator = validator.New()

func parseListParams(query url.Values) (*services.ListArguments, *errors.ServiceError) {
	p, formatErr := bindListParams(query)
	if formatErr != nil {
		return nil, formatErr
	}

	if err := validateListParams(p); err != nil {
		return nil, err
	}

	args := &services.ListArguments{
		Page:        p.Page,
		Size:        p.Size,
		Search:      strings.TrimSpace(query.Get("search")),
		RefType:     p.RefType,
		RefTargetID: p.RefTargetID,
		Fields:      ensureIDField(normalizeList(query["fields"])),
		Order:       normalizeList(query["order"]),
	}

	if len(args.Order) == 0 {
		args.Order = []string{defaultOrder}
	}

	return args, nil
}

func bindListParams(query url.Values) (*listParams, *errors.ServiceError) {
	defaults := services.NewListArguments()
	p := &listParams{
		Page:        defaults.Page,
		Size:        defaults.Size,
		RefType:     strings.TrimSpace(query.Get("ref_type")),
		RefTargetID: strings.TrimSpace(query.Get("ref_target_id")),
	}

	var formatErrors []errors.ValidationDetail

	if v := strings.TrimSpace(query.Get("page")); v != "" {
		page, err := strconv.Atoi(v)
		if err != nil {
			formatErrors = append(formatErrors, errors.ValidationDetail{
				Field:      "page",
				Value:      v,
				Constraint: "format",
				Message:    "must be a valid integer",
			})
		} else {
			p.Page = page
		}
	}

	if v := strings.TrimSpace(query.Get("size")); v != "" {
		size, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			formatErrors = append(formatErrors, errors.ValidationDetail{
				Field:      "size",
				Value:      v,
				Constraint: "format",
				Message:    "must be a valid integer",
			})
		} else {
			p.Size = size
		}
	}

	if len(formatErrors) > 0 {
		return nil, errors.ValidationWithDetails("Invalid query parameters", formatErrors)
	}

	return p, nil
}

func validateListParams(p *listParams) *errors.ServiceError {
	err := listParamsValidator.Struct(p)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return errors.GeneralError("unexpected validation error: %s", err)
	}

	var details []errors.ValidationDetail
	for _, fe := range validationErrors {
		field := strings.ToLower(fe.Field())
		details = append(details, errors.ValidationDetail{
			Field:      field,
			Value:      fe.Value(),
			Constraint: mapConstraint(fe.Tag()),
			Message:    validationMessage(field, fe),
		})
	}

	return errors.ValidationWithDetails("Invalid query parameters", details)
}

func validationMessage(field string, fe validator.FieldError) string {
	switch fe.Tag() {
	case "min":
		return field + " must be at least " + fe.Param()
	case "max":
		return field + " must be at most " + fe.Param()
	case "required_with":
		return "ref_type and ref_target_id must be provided together"
	default:
		return fe.Error()
	}
}

func mapConstraint(tag string) string {
	switch tag {
	case "required_with":
		return "required"
	default:
		return tag
	}
}

func ensureIDField(fields []string) []string {
	if len(fields) == 0 {
		return nil
	}
	if slices.Contains(fields, "id") {
		return fields
	}
	return append(fields, "id")
}

// normalizeList splits comma-separated values, trims whitespace, and drops empties.
// Supports both ?key=a,b and ?key=a&key=b.
func normalizeList(values []string) []string {
	var result []string

	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				result = append(result, item)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

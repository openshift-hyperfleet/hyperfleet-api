package presenters

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

type ProjectionList struct {
	Kind  string                   `json:"kind"`
	Page  int32                    `json:"page"`
	Size  int32                    `json:"size"`
	Total int32                    `json:"total"`
	Items []map[string]interface{} `json:"items"`
}

// SliceFilter returns a projected list containing requested fields from each item
func SliceFilter(fields2Store []string, model interface{}) (*ProjectionList, *errors.ServiceError) {
	if model == nil {
		return nil, errors.Validation("Empty model")
	}

	// Prepare list of required field
	var in = map[string]bool{}
	for i := 0; i < len(fields2Store); i++ {
		in[fields2Store[i]] = true
	}

	reflectValue := reflect.ValueOf(model)
	reflectValue = reflect.Indirect(reflectValue)

	// Initialize result structure
	result := &ProjectionList{
		Kind:  reflectValue.FieldByName("Kind").String(),
		Page:  int32(reflectValue.FieldByName("Page").Int()),
		Size:  int32(reflectValue.FieldByName("Size").Int()),
		Total: int32(reflectValue.FieldByName("Total").Int()),
		Items: nil,
	}

	field := reflectValue.FieldByName("Items").Interface()
	items := reflect.ValueOf(field)
	if items.Len() == 0 {
		return result, nil
	}

	// Validate model
	validateIn := make(map[string]bool)
	for key, value := range in {
		validateIn[key] = value
	}
	if err := validate(items.Index(0).Interface(), validateIn, ""); err != nil {
		return nil, err
	}

	// Convert items
	for i := 0; i < items.Len(); i++ {
		result.Items = append(result.Items, structToMap(items.Index(i).Interface(), in, ""))
	}
	return result, nil
}

func validate(model interface{}, in map[string]bool, prefix string) *errors.ServiceError {
	if model == nil {
		return errors.Validation("Empty model")
	}

	v := reflect.TypeOf(model)
	reflectValue := reflect.ValueOf(model)
	reflectValue = reflect.Indirect(reflectValue)

	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	for i := 0; i < v.NumField(); i++ {
		t := v.Field(i)
		tag := t.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		ttype := reflectValue.Field(i)
		kind := ttype.Kind()
		if kind == reflect.Pointer {
			kind = ttype.Elem().Kind()
		}
		field := reflectValue.Field(i).Interface()
		name := strings.Split(tag, ",")[0]
		switch kind { //nolint:exhaustive // Only Struct and Slice need special handling, rest handled by default
		case reflect.Struct:
			prefixedName := name
			if prefix != "" {
				prefixedName = fmt.Sprintf("%s.%s", prefix, name)
			}
			if t.Type == reflect.TypeOf(&time.Time{}) || t.Type == reflect.TypeOf(time.Time{}) {
				delete(in, prefixedName)
			} else {
				star := prefixedName + ".*"
				if _, ok := in[star]; ok {
					in = removeStar(in, prefixedName)
				} else {
					if err := validate(field, in, prefixedName); err != nil {
						return err
					}
				}
			}
		case reflect.Slice:
			prefixedName := name
			if prefix != "" {
				prefixedName = fmt.Sprintf("%s.%s", prefix, name)
			}
			delete(in, prefixedName)
			if _, ok := in[prefixedName+".*"]; ok {
				in = removeStar(in, prefixedName)
				continue
			}
			sliceType := ttype.Type()
			if sliceType.Kind() == reflect.Pointer {
				sliceType = sliceType.Elem()
			}
			elemType := sliceType.Elem()
			if elemType.Kind() == reflect.Pointer {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType != reflect.TypeOf(time.Time{}) {
				sliceFieldPrefix := prefixedName + "."
				subIn := make(map[string]bool)
				for k := range in {
					if strings.HasPrefix(k, sliceFieldPrefix) {
						subIn[k] = true
					}
				}
				if len(subIn) > 0 {
					subKeys := make([]string, 0, len(subIn))
					for k := range subIn {
						subKeys = append(subKeys, k)
					}
					elemValue := reflect.New(elemType).Elem().Interface()
					if err := validate(elemValue, subIn, prefixedName); err != nil {
						return err
					}
					for _, k := range subKeys {
						delete(in, k)
					}
				}
			} else {
				in = removeStar(in, prefixedName)
			}
		default:
			prefixedName := name
			if prefix != "" {
				prefixedName = fmt.Sprintf("%s.%s", prefix, name)
			}
			delete(in, prefixedName)
		}
	}

	// All fields present in data struct
	if len(in) == 0 {
		return nil
	}

	var fields []string
	for k := range in {
		fields = append(fields, k)
	}
	message := fmt.Sprintf("The following field(s) doesn't exist in `%s`: %s",
		reflect.TypeOf(model).Name(), strings.Join(fields, ", "))
	return errors.Validation("%s", message)
}

func removeStar(in map[string]bool, name string) map[string]bool {
	prefix := name + "."
	for k := range in {
		if strings.HasPrefix(k, prefix) {
			delete(in, k)
		}
	}
	return in
}

func structToMap(item interface{}, in map[string]bool, prefix string) map[string]interface{} {
	res := map[string]interface{}{}

	if item == nil {
		return res
	}
	v := reflect.TypeOf(item)
	reflectValue := reflect.ValueOf(item)
	reflectValue = reflect.Indirect(reflectValue)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	for i := 0; i < v.NumField(); i++ {
		t := v.Field(i)
		tag := t.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		ttype := reflectValue.Field(i)
		kind := ttype.Kind()
		if kind == reflect.Pointer {
			kind = ttype.Elem().Kind()
		}
		field := reflectValue.Field(i).Interface()
		name := strings.Split(tag, ",")[0]
		switch kind { //nolint:exhaustive // Only Struct and Slice need special handling, rest handled by default
		case reflect.Struct:
			if t.Type == reflect.TypeOf(&time.Time{}) || t.Type == reflect.TypeOf(time.Time{}) {
				prefixedName := name
				if prefix != "" {
					prefixedName = fmt.Sprintf("%s.%s", prefix, name)
				}
				prefixedStar := fmt.Sprintf("%s.*", prefix)
				if _, ok := in[prefixedName]; ok || in[prefixedStar] {
					if timePtr, ok := field.(*time.Time); ok && timePtr != nil {
						res[name] = timePtr.Format(time.RFC3339)
					} else if timeVal, ok := field.(time.Time); ok && !timeVal.IsZero() {
						res[name] = timeVal.Format(time.RFC3339)
					}
				}
			} else {
				nextPrefix := name
				if prefix != "" {
					nextPrefix = prefix + "." + name
				}
				subStruct := structToMap(field, in, nextPrefix)
				if len(subStruct) > 0 {
					res[name] = subStruct
				}
			}
		case reflect.Slice:
			s := reflect.ValueOf(field)
			nextPrefix := name
			if prefix != "" {
				nextPrefix = prefix + "." + name
			}
			starSelectorKey := nextPrefix + ".*"
			requested := in[nextPrefix] || in[starSelectorKey]
			if !requested {
				subPrefix := nextPrefix + "."
				for k := range in {
					if strings.HasPrefix(k, subPrefix) {
						requested = true
						break
					}
				}
			}
			if s.Len() == 0 {
				if requested {
					res[name] = []interface{}{}
				}
			} else {
				// Bare slice request acts as a star selector, returning all element fields.
				elemIn := in
				if _, ok := in[nextPrefix]; ok && !in[starSelectorKey] {
					elemIn = make(map[string]bool, len(in)+1)
					for k, v := range in {
						elemIn[k] = v
					}
					elemIn[starSelectorKey] = true
				}
				result := make([]interface{}, 0, s.Len())
				for i := 0; i < s.Len(); i++ {
					elem := s.Index(i)
					elemKind := elem.Kind()
					if elemKind == reflect.Pointer {
						if elem.IsNil() {
							continue
						}
						elemKind = elem.Elem().Kind()
					}
					if elemKind != reflect.Struct {
						if elemIn[starSelectorKey] {
							result = append(result, elem.Interface())
						}
						continue
					}
					slice := structToMap(elem.Interface(), elemIn, nextPrefix)
					if len(slice) == 0 {
						continue
					}
					result = append(result, slice)
				}
				if len(result) > 0 {
					res[name] = result
				}
			}
		default:
			prefixedName := name
			if prefix != "" {
				prefixedName = fmt.Sprintf("%s.%s", prefix, name)
			}
			if _, ok := in[prefixedName]; ok {
				res[name] = field
			} else {
				prefixedStar := fmt.Sprintf("%s.*", prefix)
				if _, ok := in[prefixedStar]; ok {
					res[name] = field
				}
			}
		}
	}

	return res
}

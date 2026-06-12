package presenters

import (
	"fmt"
)

type PathMappingFunc func(interface{}) string

var pathRegistry = make(map[string]PathMappingFunc)

func RegisterPath(objType interface{}, pathValue string) {
	typeName := fmt.Sprintf("%T", objType)
	pathRegistry[typeName] = func(interface{}) string {
		return pathValue
	}
}

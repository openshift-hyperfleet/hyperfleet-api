package presenters

import (
	"fmt"
)

type KindMappingFunc func(interface{}) string

var kindRegistry = make(map[string]KindMappingFunc)

func RegisterKind(objType interface{}, kindValue string) {
	typeName := fmt.Sprintf("%T", objType)
	kindRegistry[typeName] = func(interface{}) string {
		return kindValue
	}
}

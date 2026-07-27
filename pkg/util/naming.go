package util

import (
	"strings"
	"unicode"
)

// adapterConditionSuffix is the suffix appended to adapter names when generating condition types (QUAL-01)
const adapterConditionSuffix = "Successful"

// MapAdapterToConditionType converts an adapter name to a semantic condition type (PascalCase + "Successful" suffix).
// Used to derive the type name for per-adapter conditions mirrored into resource status
// (e.g. "adapter1" → "Adapter1Successful", "my-adapter" → "MyAdapterSuccessful").
func MapAdapterToConditionType(adapterName string) string {
	parts := strings.Split(adapterName, "-")
	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}

	result.WriteString(adapterConditionSuffix)
	return result.String()
}

//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	specschemas "github.com/openshift-hyperfleet/hyperfleet-api-spec/schemas"
)

func main() {
	output := flag.String("output", "openapi/openapi.yaml", "Output path for extracted schema")
	variant := flag.String("variant", "core", "Schema variant (core, gcp)")
	flag.Parse()

	path := fmt.Sprintf("%s/openapi.yaml", *variant)
	data, err := specschemas.FS.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*output, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *output, err)
		os.Exit(1)
	}

	fmt.Printf("Extracted %s to %s (%d bytes)\n", path, *output, len(data))
}

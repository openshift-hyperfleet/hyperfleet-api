//go:build fips

//go:debug fips140=on

package main

import "crypto/fips140"

// This file is built only into the container image (see the Dockerfile).
// The go:debug line turns FIPS mode on. init stops the service if FIPS mode
// was turned off at runtime, for example with GODEBUG=fips140=off.
func init() {
	if !fips140.Enabled() {
		panic("FIPS 140-3 mode is not enabled")
	}
}

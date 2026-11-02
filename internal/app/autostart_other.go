//go:build !windows

package app

// The Windows Run key only exists on Windows; tests that pretend to be Windows
// on other systems see it as unavailable.
func runValueExists(string) (bool, error) { return false, errAutostartUnsupported }
func setRunValue(string, string) error    { return errAutostartUnsupported }
func deleteRunValue(string) error         { return errAutostartUnsupported }

//go:build !darwin || !cgo

package apps

// DisplayName falls back to the bundle file name without localisation.
func DisplayName(path string) string { return stem(path) }

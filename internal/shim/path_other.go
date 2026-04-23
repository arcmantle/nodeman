//go:build !windows

package shim

func shortPath(path string) string {
	return path
}

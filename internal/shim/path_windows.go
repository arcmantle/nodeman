//go:build windows

package shim

import "syscall"

// shortPath returns the Windows 8.3 short path form, which is always ASCII.
// This avoids encoding issues when non-ASCII characters (e.g. øæå) appear in
// the user's home directory path. cmd.exe may misinterpret such bytes under
// the active OEM code page, causing .cmd shim wrappers to fail.
// Falls back to the original path if short path conversion fails (e.g. when
// 8.3 name generation is disabled on the volume).
func shortPath(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buf := make([]uint16, 260)
	n, err := syscall.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		return path
	}
	if n > uint32(len(buf)) {
		buf = make([]uint16, n)
		n, err = syscall.GetShortPathName(p, &buf[0], uint32(len(buf)))
		if err != nil {
			return path
		}
	}
	return syscall.UTF16ToString(buf[:n])
}

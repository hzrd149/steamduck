// Package hostlib finds shared libraries installed on the host system.
package hostlib

import (
	"os"
	"path/filepath"
)

// Runtime libraries kwakore's window program and the store window load.
const (
	WebKit = "libwebkit2gtk-4.1.so.0"
	SDL2   = "libSDL2-2.0.so.0"
)

// dirs are where SteamOS (Arch layout) and other distributions keep shared
// libraries.
var dirs = []string{"/usr/lib", "/usr/lib64", "/usr/lib/x86_64-linux-gnu", "/usr/lib/aarch64-linux-gnu", "/lib/x86_64-linux-gnu"}

// Find returns the path of a shared library by soname, or "".
func Find(soname string) string {
	for _, d := range dirs {
		p := filepath.Join(d, soname)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

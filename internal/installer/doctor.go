// Package installer puts kwakore and steamduck on a Steam Deck. Everything
// lives in the user's home (~/.local and ~/.config), which SteamOS keeps
// across system updates; the read-only system partition is never touched.
package installer

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hzrd149/steamduck/internal/paths"
)

// Check is one doctor finding.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// OSRelease returns /etc/os-release as a map.
func OSRelease() map[string]string {
	out := map[string]string{}
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			out[k] = strings.Trim(v, `"'`)
		}
	}
	return out
}

// IsSteamOS reports whether this is SteamOS.
func IsSteamOS() bool { return OSRelease()["ID"] == "steamos" }

// libDirs are where SteamOS (Arch layout) and other distributions keep
// shared libraries.
var libDirs = []string{"/usr/lib", "/usr/lib64", "/usr/lib/x86_64-linux-gnu", "/usr/lib/aarch64-linux-gnu", "/lib/x86_64-linux-gnu"}

// FindLib returns the path of a shared library by soname, or "".
func FindLib(soname string) string {
	for _, d := range libDirs {
		p := filepath.Join(d, soname)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Runtime libraries kwakore's window program loads.
const (
	LibWebKit = "libwebkit2gtk-4.1.so.0"
	LibSDL2   = "libSDL2-2.0.so.0"
)

// Doctor checks everything steamduck needs, without changing anything.
func Doctor(ctx context.Context) []Check {
	var checks []Check
	add := func(c Check) { checks = append(checks, c) }

	osr := OSRelease()
	add(Check{Name: "os", OK: true, Detail: strings.TrimSpace(osr["PRETTY_NAME"] + " " + osr["VERSION_ID"] + " " + runtime.GOARCH)})

	if out, err := exec.CommandContext(ctx, "systemctl", "--user", "is-system-running").Output(); err != nil && len(out) == 0 {
		add(Check{Name: "systemd user manager", Detail: "systemctl --user is not usable", Fix: "log in to a graphical session"})
	} else {
		add(Check{Name: "systemd user manager", OK: true, Detail: strings.TrimSpace(string(out))})
	}

	webkit := FindLib(LibWebKit)
	c := Check{Name: "WebKitGTK 4.1", OK: webkit != "", Detail: webkit}
	if webkit == "" {
		c.Detail = LibWebKit + " not found"
		c.Fix = "SteamOS does not ship it in this release; see docs/steamos-runtime.md"
	}
	add(c)

	sdl := FindLib(LibSDL2)
	c = Check{Name: "SDL2 (game controllers)", OK: sdl != "", Detail: sdl}
	if sdl == "" {
		c.Detail = LibSDL2 + " not found; napplets will see no controllers"
	}
	add(c)

	cur := paths.KwakoreCurrent()
	if _, err := os.Stat(filepath.Join(cur, "kwakore")); err != nil {
		add(Check{Name: "kwakore", Detail: "not installed", Fix: "steamduck install"})
	} else {
		add(Check{Name: "kwakore", OK: true, Detail: cur})
	}

	out, _ := exec.CommandContext(ctx, "systemctl", "--user", "is-active", "kwakore.socket").Output()
	state := strings.TrimSpace(string(out))
	add(Check{Name: "kwakore.socket", OK: state == "active", Detail: state, Fix: "systemctl --user enable --now kwakore.socket"})

	if _, err := os.Stat(DropInPath()); err != nil {
		add(Check{Name: "steamduck service drop-in", Detail: "missing", Fix: "steamduck install"})
	} else {
		add(Check{Name: "steamduck service drop-in", OK: true, Detail: DropInPath()})
	}

	if disp := os.Getenv("DISPLAY"); disp == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		add(Check{Name: "display", Detail: "this shell has no DISPLAY; checked from SSH?"})
	} else {
		add(Check{Name: "display", OK: true, Detail: "DISPLAY=" + disp + " WAYLAND_DISPLAY=" + os.Getenv("WAYLAND_DISPLAY")})
	}
	return checks
}

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

	"github.com/hzrd149/steamduck/internal/flatpak"
	"github.com/hzrd149/steamduck/internal/hostlib"
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

// FindLib returns the path of a shared library by soname, or "".
func FindLib(soname string) string { return hostlib.Find(soname) }

// Runtime libraries kwakore's window program loads.
const (
	LibWebKit = hostlib.WebKit
	LibSDL2   = hostlib.SDL2
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

	// WebKit windows (napplets and the store) run on the host's WebKitGTK
	// 4.1, or in a Flatpak runtime that ships it.
	cfg, cfgErr := paths.LoadConfig()
	webkit := FindLib(LibWebKit)
	var c Check
	switch {
	case cfgErr != nil:
		c = Check{Name: "window runtime", Detail: "config: " + cfgErr.Error()}
	case !flatpak.Use(cfg):
		c = Check{Name: "window runtime", OK: webkit != "", Detail: "host WebKitGTK 4.1 " + webkit}
		if webkit == "" {
			c.Detail = "window_runtime is host, but " + LibWebKit + " was not found"
			c.Fix = `set "window_runtime" to "auto" in ~/.config/steamduck/config.json`
		}
	case !flatpak.Available():
		c = Check{Name: "window runtime", Detail: "no WebKitGTK 4.1 on the host and flatpak is not installed"}
	case !flatpak.Installed(ctx, cfg.FlatpakRuntime):
		c = Check{Name: "window runtime", Detail: "Flatpak runtime " + cfg.FlatpakRuntime + " is not installed",
			Fix: "steamduck runtime install"}
	default:
		c = Check{Name: "window runtime", OK: true, Detail: "Flatpak " + cfg.FlatpakRuntime}
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

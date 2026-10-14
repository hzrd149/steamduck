// Package flatpak runs steamduck's WebKit windows (napplet windows and the
// store) inside a Flatpak runtime, for systems such as SteamOS whose
// read-only root has no WebKitGTK 4.1. The runtime installs per user
// without root and survives system updates.
//
// Only the window programs run in the sandbox. The kwakore service stays
// on the host; a napplet window reaches it over the connection an attached
// launch hands over, which the sandbox inherits as stdin and stdout, so the
// sandbox needs no network.
package flatpak

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hzrd149/steamduck/internal/hostlib"
	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/paths"
)

// FlathubURL is where the runtime comes from.
const FlathubURL = "https://dl.flathub.org/repo/flathub.flatpakrepo"

// Use reports whether WebKit windows should run in the Flatpak runtime.
func Use(cfg paths.Config) bool {
	switch cfg.WindowRuntime {
	case paths.WindowRuntimeFlatpak:
		return true
	case paths.WindowRuntimeHost:
		return false
	}
	return hostlib.Find(hostlib.WebKit) == ""
}

// InSandbox reports whether this process runs inside a Flatpak sandbox.
func InSandbox() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}

// Available reports whether the flatpak command exists.
func Available() bool {
	_, err := exec.LookPath("flatpak")
	return err == nil
}

// Installed reports whether runtime is installed, per user or system wide.
func Installed(ctx context.Context, runtime string) bool {
	return exec.CommandContext(ctx, "flatpak", "info", runtime).Run() == nil
}

// Install adds the Flathub remote for this user, if missing, and installs
// runtime from it. Progress goes to out.
func Install(ctx context.Context, runtime string, out io.Writer) error {
	if !Available() {
		return errors.New("flatpak is not installed")
	}
	if Installed(ctx, runtime) {
		return nil
	}
	steps := [][]string{
		{"remote-add", "--user", "--if-not-exists", "flathub", FlathubURL},
		{"install", "--user", "--noninteractive", "-y", "flathub", runtime},
	}
	for _, args := range steps {
		cmd := exec.CommandContext(ctx, "flatpak", args...)
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("flatpak %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

// ReleaseDir is the real directory of the live kwakore release. The
// sandbox gets this directory, not the "current" symlink to it.
func ReleaseDir() (string, error) {
	dir, err := filepath.EvalSymlinks(paths.KwakoreCurrent())
	if err != nil {
		return "", fmt.Errorf("kwakore release: %w", err)
	}
	return dir, nil
}

// sandboxArgs are shared by every WebKit window: X11 (gamescope composites
// X11 clients), GPU, gamepads and sound, and no network.
//
// WebKit reads gamepads through libmanette from /dev/input, which the
// sandbox only has with --device=input (or all, before Flatpak 1.15.6).
// Without it navigator.getGamepads() stays empty and the Deck's controls
// do nothing.
//
// WebKit's DMA-BUF renderer leaves the window blank inside the sandbox
// under X11, so it is turned off.
func sandboxArgs() []string {
	return []string{
		"--socket=x11",
		"--share=ipc",
		"--device=dri",
		"--device=" + inputDevice(),
		"--socket=pulseaudio",
		"--unshare=network",
		"--env=GDK_BACKEND=x11",
		"--env=WEBKIT_DISABLE_DMABUF_RENDERER=1",
	}
}

// inputDevice is the --device value that exposes /dev/input: "input" where
// Flatpak knows it (1.15.6 and later), "all" otherwise.
func inputDevice() string {
	out, err := exec.Command("flatpak", "--version").Output()
	if err == nil && versionAtLeast(strings.TrimPrefix(strings.TrimSpace(string(out)), "Flatpak "), 1, 15, 6) {
		return "input"
	}
	return "all"
}

// versionAtLeast reports whether the dotted version v is at least
// major.minor.patch. Unparsable parts count as zero.
func versionAtLeast(v string, want ...int) bool {
	parts := strings.Split(v, ".")
	for i, w := range want {
		n := 0
		if i < len(parts) {
			n, _ = strconv.Atoi(parts[i])
		}
		if n != w {
			return n > w
		}
	}
	return true
}

// WindowArgs are the `flatpak run` arguments that start kwakore's window
// program for w from releaseDir, read-only, inside runtime.
func WindowArgs(runtime, releaseDir string, w kwakrpc.AttachedWindow) []string {
	w.Name = titleText(w.Name)
	args := []string{
		"run",
		"--command=" + filepath.Join(releaseDir, "kwaklet"),
		"--filesystem=" + releaseDir + ":ro",
	}
	args = append(args, sandboxArgs()...)
	for _, kv := range append(w.Env(), "WEBVIEW_PATH="+releaseDir) {
		args = append(args, "--env="+kv)
	}
	return append(args, runtime)
}

// StoreArgs are the `flatpak run` arguments that run the steamduck binary
// exe as the store inside runtime. The store reaches kwakore through its
// socket, starts napplet windows inside its own sandbox (so they stay in
// its Steam session), and runs host commands such as `steamduck sync`
// through flatpak-spawn --host.
func StoreArgs(runtime, exe, releaseDir, socket string, storeArgs []string) []string {
	args := []string{
		"run",
		"--command=" + exe,
		"--filesystem=" + filepath.Dir(exe) + ":ro",
		"--filesystem=" + releaseDir + ":ro",
		"--filesystem=" + socketDir(socket),
		"--filesystem=" + paths.DataDir() + ":create",
		"--talk-name=org.freedesktop.Flatpak",
	}
	args = append(args, sandboxArgs()...)
	args = append(args,
		"--env=STEAMDUCK_IN_FLATPAK=1",
		"--env=STEAMDUCK_HOST_EXE="+exe,
		"--env=STEAMDUCK_KWAKORE_DIR="+releaseDir,
		"--env=STEAMDUCK_KWAKORE_SOCKET="+socket,
		"--env=XDG_DATA_HOME="+filepath.Dir(paths.DataDir()),
		runtime,
		"store",
	)
	return append(args, storeArgs...)
}

// socketDir exposes the directory of the kwakore socket. One below
// $XDG_RUNTIME_DIR uses Flatpak's xdg-run form, which maps it to the same
// path inside the sandbox.
func socketDir(socket string) string {
	dir := filepath.Dir(socket)
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		if rel, err := filepath.Rel(rt, dir); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return "xdg-run/" + rel
		}
	}
	return dir
}

// titleText reduces an author-provided name to one bounded line, as kwakore
// does before it puts a name in the window program's environment.
func titleText(name string) string {
	name = strings.ToValidUTF8(name, "�")
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > 256 {
		name = strings.TrimSpace(string([]rune(name)[:256]))
	}
	return name
}

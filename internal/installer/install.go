package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hzrd149/steamduck/internal/flatpak"
	"github.com/hzrd149/steamduck/internal/paths"
)

// KwakoreRepo is where kwakore releases and its install helper come from.
const KwakoreRepo = "hzrd149/kwakore"

// Options for Install.
type Options struct {
	// KwakoreVersion is a release tag such as v0.2.0; "" installs the latest.
	KwakoreVersion string
	// KwakoreArchive installs a local kwakore-linux-ARCH.tar.gz (from
	// `just bundle`) instead of downloading a release.
	KwakoreArchive string
	// KwakoreInstaller is a local copy of kwakore's scripts/install.sh;
	// "" downloads it from the same ref as the release.
	KwakoreInstaller string
	// SkipKwakore leaves an existing kwakore installation alone.
	SkipKwakore bool
	Log         *slog.Logger
}

// Install sets up kwakore and steamduck for the current user:
//
//  1. kwakore, through its own install helper, into ~/.local/lib/kwakore
//     with the kwakore.socket and kwakore.service user units;
//  2. a kwakore.service drop-in with the environment napplet windows need
//     under gamescope;
//  3. the steamduck binary into ~/.local/lib/steamduck, linked from
//     ~/.local/bin;
//  4. the steamduck-sync user service that keeps Steam shortcuts current;
//  5. a desktop entry for the store app in desktop mode.
func Install(ctx context.Context, opts Options) error {
	log := opts.Log
	if os.Geteuid() == 0 {
		return errors.New("run steamduck install as the deck user, not root")
	}
	cfg, err := paths.LoadConfig()
	if err != nil {
		return err
	}

	if !opts.SkipKwakore {
		if err := installKwakore(ctx, opts); err != nil {
			return fmt.Errorf("install kwakore: %w", err)
		}
	}
	if err := writeDropIn(); err != nil {
		return err
	}
	exe, err := installSelf()
	if err != nil {
		return fmt.Errorf("install steamduck: %w", err)
	}
	log.Info("installed steamduck", "path", exe)
	if err := writeUnits(exe); err != nil {
		return err
	}
	if err := writeDesktopEntry(exe); err != nil {
		return err
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	// pick up the drop-in; the socket stays, so nothing is lost
	_ = systemctl(ctx, "try-restart", "kwakore.service")
	if err := systemctl(ctx, "enable", "--now", "steamduck-sync.service"); err != nil {
		return err
	}

	// Without WebKitGTK 4.1 on the host (SteamOS), napplet windows and the
	// store run in a Flatpak runtime. It is a large download, so it comes
	// last; a failure leaves everything else installed and says how to
	// retry.
	if flatpak.Use(cfg) {
		log.Info("no WebKitGTK 4.1 on this system; installing the Flatpak runtime that napplet windows and the store run in (about 1 GB on disk)",
			"runtime", cfg.FlatpakRuntime)
		if err := flatpak.Install(ctx, cfg.FlatpakRuntime, os.Stderr); err != nil {
			log.Error("could not install the Flatpak runtime; napplets will not open until it is installed. Retry with: steamduck runtime install",
				"err", err)
		}
	}
	return nil
}

func installKwakore(ctx context.Context, opts Options) error {
	script := opts.KwakoreInstaller
	if script == "" {
		ref := opts.KwakoreVersion
		if ref == "" {
			ref = "master"
		}
		tmp, err := os.MkdirTemp("", "steamduck-install-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		script = filepath.Join(tmp, "install.sh")
		url := "https://raw.githubusercontent.com/" + KwakoreRepo + "/" + ref + "/scripts/install.sh"
		opts.Log.Info("downloading kwakore install helper", "url", url)
		if err := download(ctx, url, script); err != nil {
			return err
		}
	}
	args := []string{script}
	if opts.KwakoreArchive != "" {
		args = append(args, "--archive", opts.KwakoreArchive)
	} else if opts.KwakoreVersion != "" {
		args = append(args, "--version", opts.KwakoreVersion)
	}
	cmd := exec.CommandContext(ctx, "bash", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func download(ctx context.Context, url, dst string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 4<<20)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// DropInPath is steamduck's drop-in for kwakore.service.
func DropInPath() string {
	return filepath.Join(paths.SystemdUserDir(), "kwakore.service.d", "steamduck.conf")
}

// dropIn is the environment kwakore's napplet windows inherit.
//
// GDK_BACKEND=x11: in game mode gamescope only composites X11 clients and
// picks the visible one by the STEAM_GAME property, which steamduck sets on
// X11 windows. Desktop mode on SteamOS is an X11 Plasma session, so this
// changes nothing there.
const dropIn = `# Written by steamduck install; removed by steamduck uninstall.
[Service]
Environment=GDK_BACKEND=x11
`

func writeDropIn() error {
	return writeFile(DropInPath(), []byte(dropIn), 0o644)
}

// installSelf copies the running binary to ~/.local/lib/steamduck and links
// it from ~/.local/bin. Steam shortcuts point at the lib path.
func installSelf() (string, error) {
	src, err := os.Executable()
	if err != nil {
		return "", err
	}
	src, _ = filepath.EvalSymlinks(src)
	dst := filepath.Join(paths.LibDir(), "steamduck")
	if src != dst {
		data, err := os.ReadFile(src)
		if err != nil {
			return "", err
		}
		if err := writeFile(dst, data, 0o755); err != nil {
			return "", err
		}
	}
	link := filepath.Join(paths.BinDir(), "steamduck")
	if err := os.MkdirAll(paths.BinDir(), 0o755); err != nil {
		return "", err
	}
	if cur, err := os.Readlink(link); err != nil || cur != dst {
		tmp := link + ".tmp"
		_ = os.Remove(tmp)
		if err := os.Symlink(dst, tmp); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, link); err != nil {
			return "", err
		}
	}
	return dst, nil
}

const syncUnit = `# Keeps Steam's non-Steam shortcuts in step with installed napplets.
[Unit]
Description=steamduck Steam shortcut sync
Documentation=https://github.com/hzrd149/steamduck
After=kwakore.socket
Wants=kwakore.socket

[Service]
Type=simple
ExecStart=@EXE@ sync --watch
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=default.target
`

func writeUnits(exe string) error {
	unit := strings.ReplaceAll(syncUnit, "@EXE@", exe)
	return writeFile(filepath.Join(paths.SystemdUserDir(), "steamduck-sync.service"), []byte(unit), 0o644)
}

func desktopEntryPath() string {
	return filepath.Join(paths.Home(), ".local", "share", "applications", "steamduck-store.desktop")
}

func writeDesktopEntry(exe string) error {
	entry := "[Desktop Entry]\nType=Application\nName=Steamduck Store\nComment=Install and launch napplets\n" +
		"Exec=\"" + exe + "\" store\nIcon=" + filepath.Join(paths.IconsDir(), "store.png") +
		"\nCategories=Game;Network;\nTerminal=false\n"
	return writeFile(desktopEntryPath(), []byte(entry), 0o644)
}

// UninstallOptions for Uninstall.
type UninstallOptions struct {
	// Kwakore also removes kwakore through its own uninstall helper. Its
	// configuration and installed napplets are kept.
	Kwakore bool
	Log     *slog.Logger
}

// StopSync stops the shortcut sync service, so it cannot write shortcuts
// back while they are being removed.
func StopSync(ctx context.Context) error {
	return systemctl(ctx, "disable", "--now", "steamduck-sync.service")
}

// Uninstall removes everything Install and install.sh added for steamduck:
// the sync service, the kwakore.service drop-in, the desktop entry, the
// binary and its link, steamduck's data and configuration, and the PATH
// lines install.sh added to shell startup files. Steam shortcuts are the
// caller's to remove first (library.RemoveAll), while the binary they run
// is still in place.
func Uninstall(ctx context.Context, opts UninstallOptions) error {
	log := opts.Log
	if os.Geteuid() == 0 {
		return errors.New("run steamduck uninstall as the deck user, not root")
	}
	_ = StopSync(ctx)
	for _, p := range []string{
		filepath.Join(paths.SystemdUserDir(), "steamduck-sync.service"),
		DropInPath(),
		desktopEntryPath(),
	} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("remove", "path", p, "err", err)
		}
	}
	_ = os.Remove(filepath.Dir(DropInPath())) // only when empty
	// only the link install made; a steamduck the user put there stays
	link := filepath.Join(paths.BinDir(), "steamduck")
	if cur, err := os.Readlink(link); err == nil && cur == filepath.Join(paths.LibDir(), "steamduck") {
		if err := os.Remove(link); err != nil {
			log.Warn("remove", "path", link, "err", err)
		}
	}
	for _, dir := range []string{paths.LibDir(), paths.DataDir(), paths.ConfigDir()} {
		if err := os.RemoveAll(dir); err != nil {
			log.Warn("remove", "path", dir, "err", err)
		}
	}
	removePathLines(log)
	_ = systemctl(ctx, "daemon-reload")
	_ = systemctl(ctx, "reset-failed", "steamduck-sync.service")

	if opts.Kwakore {
		if err := uninstallKwakore(ctx, log); err != nil {
			return fmt.Errorf("uninstall kwakore: %w", err)
		}
	} else {
		// drop the drop-in's environment
		_ = systemctl(ctx, "try-restart", "kwakore.service")
	}
	return nil
}

// pathMarker heads the lines install.sh adds to put ~/.local/bin on PATH.
const pathMarker = "# Added by the steamduck installer"

// removePathLines undoes install.sh's PATH change: the fish conf.d file,
// and in ~/.bashrc and ~/.zshrc the marker line, the line after it and the
// blank line before it.
func removePathLines(log *slog.Logger) {
	fish := filepath.Join(xdgConfigHome(), "fish", "conf.d", "steamduck.fish")
	if data, err := os.ReadFile(fish); err == nil && strings.HasPrefix(string(data), pathMarker) {
		if err := os.Remove(fish); err != nil {
			log.Warn("remove", "path", fish, "err", err)
		}
	}
	for _, rc := range []string{".bashrc", ".zshrc"} {
		rc = filepath.Join(paths.Home(), rc)
		data, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		var kept []string
		changed := false
		for i := 0; i < len(lines); i++ {
			if lines[i] != pathMarker {
				kept = append(kept, lines[i])
				continue
			}
			if n := len(kept); n > 0 && kept[n-1] == "" {
				kept = kept[:n-1]
			}
			i++ // the PATH line
			changed = true
		}
		if !changed {
			continue
		}
		// in place, so a dotfiles symlink stays a symlink
		if err := os.WriteFile(rc, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
			log.Warn("remove PATH lines", "path", rc, "err", err)
		} else {
			log.Info("removed ~/.local/bin from PATH", "path", rc)
		}
	}
}

func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(paths.Home(), ".config")
}

// uninstallKwakore runs kwakore's own uninstall helper, which removes its
// service and binaries and keeps its configuration and napplets.
func uninstallKwakore(ctx context.Context, log *slog.Logger) error {
	tmp, err := os.MkdirTemp("", "steamduck-uninstall-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	script := filepath.Join(tmp, "uninstall.sh")
	url := "https://raw.githubusercontent.com/" + KwakoreRepo + "/master/scripts/uninstall.sh"
	log.Info("downloading kwakore uninstall helper", "url", url)
	if err := download(ctx, url, script); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func systemctl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl --user %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Package paths holds steamduck's per-user file locations and settings.
package paths

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func xdg(env, fallback string) string {
	if v := os.Getenv(env); v != "" && filepath.IsAbs(v) {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

// Home returns the user's home directory.
func Home() string {
	h, _ := os.UserHomeDir()
	return h
}

// LibDir is where steamduck installs program files:
// ~/.local/lib/steamduck, beside kwakore's ~/.local/lib/kwakore.
func LibDir() string { return filepath.Join(Home(), ".local", "lib", "steamduck") }

// KwakoreLibDir is the kwakore install helper's program directory.
func KwakoreLibDir() string { return filepath.Join(Home(), ".local", "lib", "kwakore") }

// KwakoreCurrent is the directory of the live kwakore release.
// $STEAMDUCK_KWAKORE_DIR overrides it, for testing a kwakore build.
func KwakoreCurrent() string {
	if d := os.Getenv("STEAMDUCK_KWAKORE_DIR"); d != "" {
		return d
	}
	return filepath.Join(KwakoreLibDir(), "current")
}

// BinDir is the user's ~/.local/bin.
func BinDir() string { return filepath.Join(Home(), ".local", "bin") }

// ConfigDir is $XDG_CONFIG_HOME/steamduck.
func ConfigDir() string { return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "steamduck") }

// SystemdUserDir is $XDG_CONFIG_HOME/systemd/user.
func SystemdUserDir() string {
	return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "systemd", "user")
}

// DataDir is $XDG_DATA_HOME/steamduck.
func DataDir() string { return filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), "steamduck") }

// IconsDir holds napplet icons referenced by Steam shortcuts.
func IconsDir() string { return filepath.Join(DataDir(), "icons") }

// Shortcut selections for Config.Shortcuts.
const (
	ShortcutsGames = "games" // installed napplets with the "game" archetype
	ShortcutsAll   = "all"   // every installed napplet
	ShortcutsNone  = "none"  // only the store shortcut
)

// Config is ~/.config/steamduck/config.json.
type Config struct {
	// Shortcuts picks which installed napplets get a Steam shortcut.
	Shortcuts string `json:"shortcuts"`
	// StoreShortcut adds the store app itself to Steam.
	StoreShortcut bool `json:"store_shortcut"`
	// WindowRuntime picks where WebKit windows (napplets and the store)
	// run: "host" uses the system's WebKitGTK 4.1, "flatpak" runs them in
	// FlatpakRuntime, and "auto" (the default) uses the host when it has
	// WebKitGTK 4.1 and Flatpak otherwise. $STEAMDUCK_WINDOW_RUNTIME
	// overrides it.
	WindowRuntime string `json:"window_runtime"`
	// FlatpakRuntime is the Flatpak runtime ref windows run in.
	FlatpakRuntime string `json:"flatpak_runtime"`
}

// Window runtimes for Config.WindowRuntime.
const (
	WindowRuntimeAuto    = "auto"
	WindowRuntimeHost    = "host"
	WindowRuntimeFlatpak = "flatpak"
)

// DefaultFlatpakRuntime ships WebKitGTK 4.1 and its helper processes.
const DefaultFlatpakRuntime = "org.gnome.Platform//50"

// DefaultConfig is used when no config file exists.
func DefaultConfig() Config {
	return Config{
		Shortcuts:      ShortcutsAll,
		StoreShortcut:  true,
		WindowRuntime:  WindowRuntimeAuto,
		FlatpakRuntime: DefaultFlatpakRuntime,
	}
}

// LoadConfig reads the config file, filling unset fields with defaults.
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(ConfigDir(), "config.json"))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return cfg, err
	default:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}
	if v := os.Getenv("STEAMDUCK_WINDOW_RUNTIME"); v != "" {
		cfg.WindowRuntime = v
	}
	switch cfg.WindowRuntime {
	case WindowRuntimeAuto, WindowRuntimeHost, WindowRuntimeFlatpak:
	case "":
		cfg.WindowRuntime = WindowRuntimeAuto
	default:
		return cfg, errors.New(`config: "window_runtime" must be "auto", "host" or "flatpak"`)
	}
	if cfg.FlatpakRuntime == "" {
		cfg.FlatpakRuntime = DefaultFlatpakRuntime
	}
	switch cfg.Shortcuts {
	case ShortcutsGames, ShortcutsAll, ShortcutsNone:
	case "":
		cfg.Shortcuts = ShortcutsAll
	default:
		return cfg, errors.New(`config: "shortcuts" must be "games", "all" or "none"`)
	}
	return cfg, nil
}

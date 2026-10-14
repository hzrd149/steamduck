// Package steam manages the non-Steam shortcuts steamduck owns: it finds the
// Steam installation and its user accounts, reconciles shortcuts.vdf with
// the installed napplets and writes their grid artwork.
package steam

import (
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Root returns the Steam installation directory. On SteamOS and most
// distributions ~/.steam/steam links to ~/.local/share/Steam.
func Root() (string, error) {
	if v := os.Getenv("STEAMDUCK_STEAM_ROOT"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, p := range []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
	} {
		if fi, err := os.Stat(filepath.Join(p, "userdata")); err == nil && fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("no Steam installation with a userdata directory found")
}

// User is one Steam account that has logged in on this machine.
type User struct {
	ID  string // 32-bit account id, the userdata directory name
	Dir string // <root>/userdata/<id>
}

// ShortcutsPath is the account's shortcuts.vdf.
func (u User) ShortcutsPath() string { return filepath.Join(u.Dir, "config", "shortcuts.vdf") }

// GridDir is where the account's custom artwork lives.
func (u User) GridDir() string { return filepath.Join(u.Dir, "config", "grid") }

// Users lists the accounts under root/userdata. The "0" directory Steam
// keeps for anonymous data is skipped.
func Users(root string) ([]User, error) {
	entries, err := os.ReadDir(filepath.Join(root, "userdata"))
	if err != nil {
		return nil, err
	}
	var users []User
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "0" {
			continue
		}
		if _, err := strconv.ParseUint(e.Name(), 10, 32); err != nil {
			continue
		}
		users = append(users, User{ID: e.Name(), Dir: filepath.Join(root, "userdata", e.Name())})
	}
	return users, nil
}

// ShortcutAppID is the 32-bit id Steam derives for a non-Steam shortcut
// from its target and name. Grid artwork is named by it, and gamescope
// matches windows to the running shortcut by it.
func ShortcutAppID(exe, name string) uint32 {
	return crc32.ChecksumIEEE([]byte(exe+name)) | 0x80000000
}

// GameID is the 64-bit id used in steam://rungameid/ URLs.
func GameID(appID uint32) uint64 { return uint64(appID)<<32 | 0x02000000 }

// Running reports whether a Steam client process is running for this user.
// Steam keeps shortcuts in memory and writes them back on exit, so edits to
// shortcuts.vdf made while it runs are lost and not shown.
func Running() bool {
	entries, _ := os.ReadDir("/proc")
	uid := os.Getuid()
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		fi, err := os.Stat(dir)
		if err != nil || ownerUID(fi) != uid {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(dir, "comm"))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "steam" {
			return true
		}
	}
	return false
}

func quote(s string) string { return fmt.Sprintf("%q", s) }

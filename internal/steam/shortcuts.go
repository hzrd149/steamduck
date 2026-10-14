package steam

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hzrd149/steamduck/internal/steam/vdf"
)

// ManagedTag marks the shortcuts steamduck owns. It doubles as a Steam
// collection, so napplets are grouped together in the library.
const ManagedTag = "Napplets"

// Launch options identify managed shortcuts; the tag alone could be added
// to a user's own shortcut by hand.
const (
	nappletPrefix = "launch --napplet "
	storeOptions  = "store"
	// StoreKey is the Key of the store app's own shortcut.
	StoreKey = "store"
)

// Shortcut is the state steamduck wants one managed shortcut to have.
type Shortcut struct {
	Key  string // canonical napplet address, or StoreKey
	Name string
	Exe  string // absolute path of the steamduck binary
	Icon string // absolute path of a PNG, or ""
}

// NappletShortcut returns the shortcut that launches a napplet.
func NappletShortcut(exe, address, name string) Shortcut {
	return Shortcut{Key: address, Name: name, Exe: exe}
}

// LaunchOptions is what Steam passes to Exe.
func (s Shortcut) LaunchOptions() string {
	if s.Key == StoreKey {
		return storeOptions
	}
	return nappletPrefix + s.Key
}

// quotedExe is the form Steam itself stores Exe in.
func (s Shortcut) quotedExe() string { return quote(s.Exe) }

// AppID is the stable id of the shortcut. It is derived from the key, not
// the display name, so renaming a napplet keeps its playtime and art.
func (s Shortcut) AppID() uint32 { return ShortcutAppID(s.quotedExe(), s.Key) }

// KeyOf returns the key of a managed shortcut entry.
func KeyOf(entry *vdf.Map) (string, bool) {
	opts := strings.TrimSpace(entry.String("LaunchOptions"))
	if opts == storeOptions && hasTag(entry, ManagedTag) {
		return StoreKey, true
	}
	if !strings.HasPrefix(opts, nappletPrefix) {
		return "", false
	}
	addr := strings.TrimSpace(strings.TrimPrefix(opts, nappletPrefix))
	return addr, addr != ""
}

func hasTag(entry *vdf.Map, tag string) bool {
	tags := entry.Child("tags")
	if tags == nil {
		return false
	}
	for _, f := range tags.Fields {
		if v, _ := f.Value.(string); v == tag {
			return true
		}
	}
	return false
}

// Change summarizes what a reconcile did to one file.
type Change struct {
	Added   []string `json:"added,omitempty"` // keys
	Updated []string `json:"updated,omitempty"`
	Removed []string `json:"removed,omitempty"`
	// AppIDs maps every wanted key to the appid its entry has, which for an
	// existing entry may not be Shortcut.AppID.
	AppIDs map[string]uint32 `json:"app_ids"`
	// RemovedAppIDs are the appids of the removed entries.
	RemovedAppIDs []uint32 `json:"-"`
}

// Empty reports whether nothing changed.
func (c Change) Empty() bool { return len(c.Added)+len(c.Updated)+len(c.Removed) == 0 }

// LoadShortcuts reads a shortcuts.vdf; a missing file is an empty list.
func LoadShortcuts(path string) (*vdf.Map, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &vdf.Map{Fields: []vdf.Field{{Key: "shortcuts", Value: &vdf.Map{}}}}, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := vdf.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if root.Child("shortcuts") == nil {
		root.Set("shortcuts", &vdf.Map{})
	}
	return root, nil
}

// Reconcile makes the managed entries in root match want exactly, leaving
// every other shortcut untouched. Entries are renumbered 0..n-1 as Steam
// expects.
func Reconcile(root *vdf.Map, want []Shortcut) Change {
	list := root.Child("shortcuts")
	byAddr := make(map[string]Shortcut, len(want))
	for _, s := range want {
		byAddr[s.Key] = s
	}

	change := Change{AppIDs: map[string]uint32{}}
	seen := map[string]bool{}
	var kept []*vdf.Map
	for _, f := range list.Fields {
		entry, ok := f.Value.(*vdf.Map)
		if !ok {
			continue
		}
		addr, managed := KeyOf(entry)
		if !managed {
			kept = append(kept, entry)
			continue
		}
		s, wanted := byAddr[addr]
		if !wanted || seen[addr] {
			change.Removed = append(change.Removed, addr)
			change.RemovedAppIDs = append(change.RemovedAppIDs, uint32(entry.Int("appid")))
			continue
		}
		seen[addr] = true
		if apply(entry, s) {
			change.Updated = append(change.Updated, addr)
		}
		change.AppIDs[addr] = uint32(entry.Int("appid"))
		kept = append(kept, entry)
	}

	addrs := make([]string, 0, len(byAddr))
	for a := range byAddr {
		if !seen[a] {
			addrs = append(addrs, a)
		}
	}
	sort.Strings(addrs)
	for _, a := range addrs {
		entry := newEntry(byAddr[a])
		kept = append(kept, entry)
		change.Added = append(change.Added, a)
		change.AppIDs[a] = byAddr[a].AppID()
	}

	list.Fields = list.Fields[:0]
	for i, e := range kept {
		list.Fields = append(list.Fields, vdf.Field{Key: strconv.Itoa(i), Value: e})
	}
	return change
}

func newEntry(s Shortcut) *vdf.Map {
	e := &vdf.Map{}
	e.Set("appid", int32(s.AppID()))
	e.Set("AppName", s.Name)
	e.Set("Exe", s.quotedExe())
	e.Set("StartDir", quote(filepath.Dir(s.Exe)))
	e.Set("icon", s.Icon)
	e.Set("ShortcutPath", "")
	e.Set("LaunchOptions", s.LaunchOptions())
	e.Set("IsHidden", int32(0))
	e.Set("AllowDesktopConfig", int32(1))
	e.Set("AllowOverlay", int32(1))
	e.Set("OpenVR", int32(0))
	e.Set("Devkit", int32(0))
	e.Set("DevkitGameID", "")
	e.Set("DevkitOverrideAppID", int32(0))
	e.Set("LastPlayTime", int32(0))
	e.Set("FlatpakAppID", "")
	e.Set("tags", &vdf.Map{Fields: []vdf.Field{{Key: "0", Value: ManagedTag}}})
	return e
}

// apply updates the fields steamduck owns and reports whether any changed.
// Fields the user or Steam set (playtime, hidden, tags they added) stay.
func apply(e *vdf.Map, s Shortcut) bool {
	changed := false
	set := func(k string, v any) {
		if old, ok := e.Get(k); !ok || old != v {
			e.Set(k, v)
			changed = true
		}
	}
	// Keep an existing appid: Steam assigns its own to shortcuts added
	// through its client API (the Decky plugin), and art and playtime
	// follow it.
	if _, ok := e.Get("appid"); !ok {
		set("appid", int32(s.AppID()))
	}
	set("AppName", s.Name)
	set("Exe", s.quotedExe())
	set("StartDir", quote(filepath.Dir(s.Exe)))
	set("icon", s.Icon)
	if !hasTag(e, ManagedTag) {
		tags := e.Child("tags")
		if tags == nil {
			tags = &vdf.Map{}
			e.Set("tags", tags)
		}
		tags.Set(strconv.Itoa(len(tags.Fields)), ManagedTag)
		changed = true
	}
	return changed
}

// SaveShortcuts writes root to path atomically, keeping one backup of the
// previous file beside it.
func SaveShortcuts(path string, root *vdf.Map) error {
	data, err := vdf.Marshal(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil {
		if bytes.Equal(old, data) {
			return nil
		}
		if err := os.WriteFile(path+".steamduck.bak", old, 0o644); err != nil {
			return err
		}
	}
	tmp := path + ".steamduck.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

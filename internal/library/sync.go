// Package library keeps Steam's non-Steam shortcuts in step with the
// napplets installed in kwakore.
package library

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hzrd149/steamduck/internal/kwakrpc"
	"github.com/hzrd149/steamduck/internal/paths"
	"github.com/hzrd149/steamduck/internal/steam"
)

// StoreName is the store app's title in the Steam library.
const StoreName = "Napplet Store"

// Syncer reconciles shortcuts for every Steam account on the machine.
type Syncer struct {
	Client    *kwakrpc.Client
	Config    paths.Config
	Exe       string // absolute path Steam should run (the steamduck binary)
	SteamRoot string
	Log       *slog.Logger
	// Force writes shortcuts.vdf even while Steam runs. Steam does not
	// reread it until restarted and may overwrite it before then.
	Force bool

	retry chan struct{}
}

// Result reports one sync pass.
type Result struct {
	Changes      map[string]steam.Change `json:"changes"` // by Steam account id
	SteamRunning bool                    `json:"steam_running"`
	// Deferred is set when Steam was running and nothing was written.
	Deferred      bool `json:"deferred"`
	ShortcutCount int  `json:"shortcut_count"`
}

// Changed reports whether any account's shortcuts changed.
func (r Result) Changed() bool {
	for _, c := range r.Changes {
		if !c.Empty() {
			return true
		}
	}
	return false
}

// Sync runs one reconcile pass.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	res := Result{Changes: map[string]steam.Change{}, SteamRunning: steam.Running()}
	if res.SteamRunning && !s.Force {
		res.Deferred = true
		return res, nil
	}
	users, err := steam.Users(s.SteamRoot)
	if err != nil {
		return res, err
	}
	if len(users) == 0 {
		return res, errors.New("no Steam accounts found; log in to Steam once first")
	}
	want, icons, err := s.desired(ctx)
	if err != nil {
		return res, err
	}
	res.ShortcutCount = len(want)

	art, err := steam.LoadArtState(filepath.Join(paths.DataDir(), "art.json"))
	if err != nil {
		return res, err
	}
	defer func() {
		if err := art.Save(); err != nil {
			s.Log.Warn("save art state", "err", err)
		}
	}()

	for _, u := range users {
		root, err := steam.LoadShortcuts(u.ShortcutsPath())
		if err != nil {
			return res, err
		}
		ch := steam.Reconcile(root, want)
		res.Changes[u.ID] = ch
		if !ch.Empty() {
			if err := steam.SaveShortcuts(u.ShortcutsPath(), root); err != nil {
				return res, err
			}
		}
		for _, id := range ch.RemovedAppIDs {
			art.RemoveOwned(u.GridDir(), id)
		}
		for _, sc := range want {
			png, ok := icons[sc.Key]
			if !ok {
				continue
			}
			if err := art.WriteArt(u.GridDir(), ch.AppIDs[sc.Key], png); err != nil {
				s.Log.Warn("write artwork", "napplet", sc.Name, "err", err)
			}
		}
	}
	return res, nil
}

// desired lists the shortcuts that should exist and the icon PNG of each
// that has one.
func (s *Syncer) desired(ctx context.Context) ([]steam.Shortcut, map[string][]byte, error) {
	var want []steam.Shortcut
	icons := map[string][]byte{}
	if s.Config.StoreShortcut {
		store := steam.Shortcut{Key: steam.StoreKey, Name: StoreName, Exe: s.Exe}
		if p, data, err := storeIcon(); err == nil {
			store.Icon, icons[steam.StoreKey] = p, data
		}
		want = append(want, store)
	}
	if s.Config.Shortcuts == paths.ShortcutsNone {
		return want, icons, nil
	}
	installed, err := s.Client.Installed(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list installed napplets: %w", err)
	}
	for _, it := range installed {
		if s.Config.Shortcuts == paths.ShortcutsGames && !it.IsGame() {
			continue
		}
		if it.Format != "" && it.Format != "napplet" {
			continue // legacy napps have no window runtime on Linux
		}
		name := it.Name
		if name == "" {
			name = it.Address
		}
		sc := steam.NappletShortcut(s.Exe, it.Address, name)
		if it.Icon != nil {
			if p, data, err := s.fetchIcon(ctx, it.Address); err == nil {
				sc.Icon, icons[it.Address] = p, data
			} else {
				s.Log.Debug("no icon", "napplet", name, "err", err)
			}
		}
		want = append(want, sc)
	}
	return want, icons, nil
}

// fetchIcon saves a napplet's verified icon under IconsDir by hash.
func (s *Syncer) fetchIcon(ctx context.Context, address string) (string, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	icon, err := s.Client.Icon(ctx, address)
	if err != nil {
		return "", nil, err
	}
	data, err := base64.StdEncoding.DecodeString(icon.Data)
	if err != nil {
		return "", nil, err
	}
	h := sha256.Sum256(data)
	name := hex.EncodeToString(h[:]) + ".png"
	if err := os.MkdirAll(paths.IconsDir(), 0o755); err != nil {
		return "", nil, err
	}
	p := filepath.Join(paths.IconsDir(), name)
	if _, err := os.Stat(p); err != nil {
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return "", nil, err
		}
	}
	return p, data, nil
}

// storeIcon writes the embedded store icon to IconsDir.
func storeIcon() (string, []byte, error) {
	if err := os.MkdirAll(paths.IconsDir(), 0o755); err != nil {
		return "", nil, err
	}
	p := filepath.Join(paths.IconsDir(), "store.png")
	if err := os.WriteFile(p, StoreIconPNG, 0o644); err != nil {
		return "", nil, err
	}
	return p, StoreIconPNG, nil
}

// Watch syncs once, then again after every install, update or uninstall
// kwakore reports, until ctx ends. A dropped stream is reopened.
func (s *Syncer) Watch(ctx context.Context, onSync func(Result)) error {
	s.retry = make(chan struct{}, 1)
	pending := false
	run := func() {
		res, err := s.Sync(ctx)
		if err != nil {
			s.Log.Error("sync", "err", err)
			pending = true
			return
		}
		pending = res.Deferred
		if onSync != nil {
			onSync(res)
		}
	}
	run()
	// A deferred sync is retried once Steam has exited (switching between
	// game and desktop mode restarts Steam).
	go func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				select {
				case s.retry <- struct{}{}:
				default:
				}
			}
		}
	}()
	for ctx.Err() == nil {
		sub, err := s.Client.Subscribe(ctx, "napplet.installed", "napplet.updated", "napplet.uninstalled", "events.overflow")
		if err != nil {
			s.Log.Warn("subscribe to kwakore events", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
			continue
		}
		run() // catch anything that changed while not subscribed
		s.follow(ctx, sub, run, &pending)
		sub.Close()
	}
	return ctx.Err()
}

// follow debounces bursts of events (an installed-list sync can install
// many napplets at once) into one sync.
func (s *Syncer) follow(ctx context.Context, sub *kwakrpc.Subscription, run func(), pending *bool) {
	var timer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C:
			if !ok {
				if err := sub.Err(); err != nil {
					s.Log.Warn("kwakore event stream ended", "err", err)
				}
				return
			}
			s.Log.Debug("kwakore event", "type", ev.Type, "address", ev.Address)
			timer = time.After(2 * time.Second)
		case <-timer:
			timer = nil
			run()
		case <-s.retry:
			if *pending && !steam.Running() {
				run()
			}
		}
	}
}

// Wanted is one shortcut as `steamduck shortcuts` prints it: everything a
// client that adds shortcuts through Steam's own API needs.
type Wanted struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Exe           string `json:"exe"`
	StartDir      string `json:"start_dir"`
	LaunchOptions string `json:"launch_options"`
	Icon          string `json:"icon,omitempty"` // absolute PNG path
	// AppID is the id `steamduck sync` gives the shortcut; Steam assigns a
	// different one to shortcuts added through its API.
	AppID uint32 `json:"app_id"`
}

// Wanted lists the shortcuts that should exist.
func (s *Syncer) Wanted(ctx context.Context) ([]Wanted, error) {
	want, _, err := s.desired(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Wanted, 0, len(want))
	for _, sc := range want {
		out = append(out, Wanted{
			Key: sc.Key, Name: sc.Name, Exe: sc.Exe, StartDir: filepath.Dir(sc.Exe),
			LaunchOptions: sc.LaunchOptions(), Icon: sc.Icon, AppID: sc.AppID(),
		})
	}
	return out, nil
}

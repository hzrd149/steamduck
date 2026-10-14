# steamduck

steamduck integrates the [kwakore](https://github.com/hzrd149/kwakore) napplet runtime into SteamOS on the Steam Deck:

1. **Install**: `steamduck install` sets up kwakore and steamduck as per-user services under `~/.local` and `~/.config`. It needs no root, and SteamOS updates leave it in place.
2. **Store**: `steamduck store` is a controller-friendly webview app for discovering, installing, updating, launching and removing napplets. It talks to kwakore over its control socket (JSON-RPC, protocol v1).
3. **Steam library**: installed napplets become non-Steam shortcuts with generated artwork, in a "Napplets" collection. They launch from the Steam UI in both game mode and desktop mode.

## Install on a Steam Deck

Switch to Desktop Mode, open Konsole and run:

```sh
curl -fsSL https://raw.githubusercontent.com/hzrd149/steamduck/master/install.sh | bash
```

This installs the latest release, or the rolling `nightly` build of `master` while no release exists. To pick a build, add `-s -- --nightly` or `-s -- --version v0.1.0`. The installer checks every download against the release's `SHA256SUMS`. It needs no root and asks before restarting Steam to show the new shortcuts. Run it again to update.

Releases are built by `.github/workflows/release.yml`. They contain the static `steamduck` binary and kwakore built from a pinned commit (kwakore has no releases of its own yet), along with kwakore's install helper. Pushes to `master` refresh the `nightly` prerelease and `v*` tags publish a release.

## How it fits together

```
Steam ──runs──▶ steamduck launch --napplet ADDR ──exec──▶ kwak run ADDR ──exec──▶ kwaklet window
                                                            │  napplet.launch {attach:true}
                                                            └──── socket becomes the window's wire ────▶ kwakore.service

shortcuts.vdf ◀── steamduck-sync.service ◀── napplet.installed / uninstalled events
```

- **Launching.** Kwakore's attached launch, `kwak run`, lets the process Steam started become the napplet window. `steamduck launch` execs `kwak run ADDRESS`, which asks the daemon to prepare the window and then execs `kwaklet` on the same connection. The PID never changes, so the window runs on Steam's display with Steam's overlay, gamescope shows it as the game, Steam sees it exit, and "Exit game" ends it directly. Window diagnostics go to `~/.local/share/steamduck/steamduck.log`. "Play" in the store starts `kwak run` as its own child, so in game mode the window joins the store's session.
- **Fallback for older kwakore.** A kwakore without `kwak run`, or `launch --try`, uses the legacy path. `napplet.launch` makes the daemon spawn the window. steamduck then moves the daemon to the caller's display, finds the window's processes by `KWAKORE_INSTANCE_ID` and sets the `STEAM_GAME` X11 property gamescope uses. It waits for `window.closed` and turns SIGTERM into `napplet.stop`. `STEAMDUCK_LEGACY_LAUNCH=1` forces this path.
- **Shortcuts.** `steamduck-sync.service` follows kwakore's install and uninstall events and rewrites `userdata/*/config/shortcuts.vdf`. Only steamduck's own entries change: they are identified by their `launch --napplet` options or the store's `store` option. Steam's own file is kept as `shortcuts.vdf.steamduck.bak`. Grid art is never written over art you picked yourself.

## Commands

```
steamduck install [--kwakore-version TAG | --kwakore-archive FILE] [--kwakore-installer FILE] [--skip-kwakore]
steamduck doctor            # check libraries, services, display, Steam
steamduck store             # the store window (Steam runs: steamduck store)
steamduck store --serve 127.0.0.1:8080   # the same UI in a browser, for development
steamduck launch --napplet ADDRESS [--appid ID] [--try]
steamduck sync [--watch] [--force] [--json]
steamduck shortcuts         # wanted shortcuts as JSON (for a Decky plugin)
steamduck uninstall
```

Configuration lives in `~/.config/steamduck/config.json`:

```json
{ "shortcuts": "all", "store_shortcut": true }
```

`shortcuts` is `all` (the default), `games` (only napplets with the `game` archetype) or `none`.

## Known limitations

- **Steam must restart to see new shortcuts.** Steam keeps shortcuts in memory and writes them back itself, so steamduck defers writing while Steam runs. It writes as soon as Steam exits, for example when you switch between game and desktop mode. `--force` writes anyway, but Steam will not show the change and may overwrite it. A Decky Loader plugin can add shortcuts live through Steam's client API; see `docs/design.md`.
- **WebKitGTK 4.1** must be available to the kwakore window program. See [docs/steamos-runtime.md](docs/steamos-runtime.md).
- Tested here on an Ubuntu X11 desktop against a real kwakore daemon: sync, store and launch. It has not yet been run on a Steam Deck, under gamescope.

## Development

```sh
go test ./...
go build -o steamduck ./cmd/steamduck
./steamduck store --serve 127.0.0.1:8080   # open the printed URL
```

The store window uses the `libwebview.so` from the kwakore release, through purego, so building needs no cgo.

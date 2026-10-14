# steamduck

steamduck integrates the [kwakore](https://github.com/hzrd149/kwakore) napplet runtime into SteamOS on the Steam Deck:

1. **Install**: `steamduck install` sets up kwakore and steamduck as per-user services under `~/.local` and `~/.config`. It needs no root, and SteamOS updates leave it in place.
2. **Store**: `steamduck store` is a controller-friendly webview app for discovering, installing, updating, launching and removing napplets. Its Account tab signs napplets in with a Nostr signer: a phone signer app paired by QR code (NIP-46 nostrconnect), a `bunker://` link, a private key or the system signer. Its Settings tab manages discovery relays, Blossom servers, theme, desktop integration, installed-list sync and social refresh timing, with controls to save changes or restore configured values. Each napplet has a full page, like a Steam game page, with tabs for votes and emoji reactions, comments and replies, mentions, zaps (a Lightning invoice QR code for a phone wallet) and, once installed, the permission answers it has saved, which can be forgotten. It talks to kwakore over its control socket (JSON-RPC, protocol v1).
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
steamduck store --debug     # enable the trusted napplet's webview inspector
steamduck launch --napplet ADDRESS [--appid ID] [--try]
steamduck sync [--watch] [--force] [--json]
steamduck shortcuts         # wanted shortcuts as JSON (for a Decky plugin)
steamduck uninstall [--kwakore] [--keep-shortcuts] [--restart-steam]
```

`uninstall` removes the Steam shortcuts and the art steamduck made for them, the sync service, the kwakore.service drop-in, the desktop entry, `~/.local/lib/steamduck`, steamduck's data and config, and the PATH lines the installer added. Steam must be closed, because it would write the shortcuts back; `--restart-steam` closes it and starts it again. `--kwakore` also runs kwakore's uninstall helper, which keeps kwakore's settings and installed napplets.

Configuration lives in `~/.config/steamduck/config.json`:

```json
{ "shortcuts": "all", "store_shortcut": true }
```

`shortcuts` is `all` (the default), `games` (only napplets with the `game` archetype) or `none`.

## Known limitations

- **Steam must restart to see new shortcuts.** Steam keeps shortcuts in memory and writes them back itself, so steamduck defers writing while Steam runs. It writes as soon as Steam exits, for example when you switch between game and desktop mode. `--force` writes anyway, but Steam will not show the change and may overwrite it. A Decky Loader plugin can add shortcuts live through Steam's client API; see `docs/design.md`.
- **WebKitGTK 4.1:** SteamOS lacks it, so napplet windows and the store run in the Flatpak GNOME runtime, which `steamduck install` sets up per user (about 1 GB, no root). Gamescope and performance under it are still to be confirmed on a Deck. See [docs/steamos-runtime.md](docs/steamos-runtime.md).
- Tested here on an Ubuntu X11 desktop against a real kwakore daemon: sync, store and launch. It has not yet been run on a Steam Deck, under gamescope.

## Development

### Nix development shell

With [Nix](https://nixos.org/download/) installed, enter the pinned development
environment on x86_64/aarch64 Linux or Apple Silicon macOS:

```sh
nix develop
npm --prefix internal/store/web ci
just build
just store              # rebuild and open the native store
just test
```

The shell provides Go 1.26, Node.js 24 (including npm), `just` and Git.
`flake.lock` pins the tools for all contributors. If your Nix installation has
not enabled flakes, use
`nix --extra-experimental-features 'nix-command flakes' develop` instead.
The first shell entry and `npm ci` need network access to fetch dependencies.
You can also run commands directly, for example `nix develop -c just deck`
to cross-build the Linux amd64 binary for a Steam Deck.

The shell disables cgo, matching the release build. `just store` rebuilds the
frontend and Go binary, then opens the trusted store napplet. Use
`just store --debug` to enable the webview inspector, or `just build-store` to
only compile frontend artifacts. Native launch requires a separately installed,
compatible Kwakore daemon and runtime (including `kwaklet` and `libwebview.so`).
It does not silently fall back to compatibility mode when the daemon is too old.
There is no standalone/HTTP store fallback. Browser tests use injected mock NAP
domains and additionally need Playwright's Chromium and its
host libraries; see [the store build guide](docs/store-napplet.md#verification).

To intentionally update the pinned tools, run `nix flake update` and commit
the resulting `flake.lock`.

### Without Nix

Install Go 1.26 or newer and Node.js 22.12 or newer (CI uses Node.js 24), then:

```sh
npm --prefix internal/store/web ci
npm --prefix internal/store/web run build  # rebuild the embedded store artifacts
go test ./...
go build -o steamduck ./cmd/steamduck
./steamduck store                         # open the trusted store napplet
```

The store uses Kwakore's `kwaklet` window program and `libwebview.so`; the Go
broker no longer owns a standalone webview, so building needs no cgo.

The store frontend uses Svelte 5, Vite and `@napplet/vite-plugin` to build a
single HTML napplet. The default store window uses the trusted built-in runtime
path with NAP-STEAMDUCK, optional NAP-GAMEPAD and NAP-THEME. Frontend source,
configuration, dependencies and tests live in `internal/store/web`; Vite writes
its single-file output to `internal/store/web/dist`.
See [the store build guide](docs/store-napplet.md) for artifacts, live
development and sandbox tests.

# Design notes

## Phases

1. **Packaging and install** (`internal/installer`). Kwakore's own `scripts/install.sh` installs the release into `~/.local/lib/kwakore/releases/<sha256>` behind a `current` symlink and enables `kwakore.socket`. Reusing it keeps steamduck and kwakore's install helper interchangeable. steamduck adds three things:
   - `kwakore.service.d/steamduck.conf`, which sets `GDK_BACKEND=x11` because gamescope composites X11 clients;
   - `steamduck-sync.service`;
   - a desktop entry for the store, used in desktop mode.
2. **Store** (`internal/store`). This is plain HTML, CSS and JS, embedded and inlined into one document. The webview loads it with `SetHtml`, so it has no URL and no listening port. JS calls Go through one bound function, `steamduckCall(method, params)`. Errors come back as values, so the UI keeps the daemon's error codes. `--serve` exposes the same method table over loopback HTTP, guarded by a random token, for browser development. Controller input comes through the Gamepad API (Steam Input presents the Deck's controls as a standard pad) and arrow keys, with spatial focus navigation.
3. **Steam integration** (`internal/steam`, `internal/library`, `internal/launcher`). See the README for the launch flow: `kwak run` (kwakore's attached launch, see `kwakore-attached-launch.md`), with the legacy daemon-spawned path and window tagging kept as a fallback. Each shortcut's app ID is `crc32("\"exe\"" + key) | 0x80000000`, where the key is the napplet address. Using the address rather than the display name means renames keep playtime and artwork. An existing shortcut's app ID is never changed.

## Decky Loader

[Decky Loader](https://decky.xyz/) runs plugins inside Steam's own UI, where the `SteamClient` API is available. It solves the one thing a file-based sync cannot do: changing shortcuts while Steam runs, which is always the case in game mode. The API calls involved are `SteamClient.Apps.AddShortcut`, `RemoveShortcut`, `SetShortcutName`, `SetShortcutLaunchOptions`, `SetShortcutIcon` and `SetCustomArtworkForApp`.

The plan is a small optional plugin:

- Its Python backend runs `steamduck shortcuts` and gets the wanted list: key, name, exe, start dir, launch options and icon.
- Its frontend reconciles that list against the shortcuts it created before, keeping a key → app ID map in plugin settings. It also listens for changes, for example by having the backend follow `steamduck sync --watch --json` output.
- It could add a Quick Access panel listing running napplet windows, with a close button that calls `napplet.stop`.

Shortcuts added this way get app IDs assigned by Steam. That works with the rest of steamduck: the vdf sync keeps existing app IDs, and `steamduck launch` reads the app ID from Steam's `SteamGameId` environment variable.

Decky is a third-party install that Valve's updates can break, so steamduck keeps working without it. Shortcuts then appear after Steam restarts.

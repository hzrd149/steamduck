# Kwakore's runtime libraries on SteamOS

Kwakore's window program (`kwaklet`) and the store window load `libwebview.so`, which links against **WebKitGTK 4.1** (`libwebkit2gtk-4.1.so.0`). Game controllers need **SDL2** (`libSDL2-2.0.so.0`). `steamduck doctor` reports whether each is present.

SteamOS's system partition is read-only and replaced on every update, so anything installed into it with pacman is lost. Whether a given SteamOS release ships WebKitGTK 4.1 is not verified yet. Run `steamduck doctor` on the Deck to find out. SDL2 is expected, because the Steam client uses it.

## If WebKitGTK 4.1 is missing

These are the options, roughly in order of preference. None is implemented yet. The right choice depends on what `doctor` reports on real hardware.

1. **systemd-sysext image.** Build an extension image holding `webkit2gtk-4.1` and its dependencies from the matching SteamOS/Arch package versions. Install it to `/var/lib/extensions`, which survives updates. This needs root once and must be rebuilt when the SteamOS version changes, because the image's `extension-release` must match `os-release`.
2. **Flatpak GNOME runtime.** Run `kwakore` inside `org.gnome.Platform`, which ships WebKitGTK 4.1 and SDL2. It needs no root and survives updates. The catch is that kwakore expects systemd socket activation and checks `LISTEN_PID`, so the units would run the daemon in foreground mode inside `flatpak run`. The socket directory would be shared with `--filesystem=xdg-run/kwakore:create`, and windows would need `--socket=x11 --device=all`.
3. **Distrobox/podman container.** SteamOS 3.5+ includes podman and distrobox. Run the daemon in an Arch container that shares `$HOME` and `/run/user/$UID`. This has the same foreground-mode caveat as Flatpak, and it is a large download.
4. **Pacman into the read-only root.** Run `steamos-readonly disable`, then `pacman -S webkit2gtk-4.1`. This works until the next update, and `steamduck doctor` would catch the regression. Use it only for testing.

A private copy of the libraries with `LD_LIBRARY_PATH` alone does not work. WebKitGTK starts its `WebKitWebProcess` and `WebKitNetworkProcess` helpers from a path compiled into the library, and release builds ignore `WEBKIT_EXEC_PATH`.

# WebKitGTK on SteamOS

Napplet windows (kwakore's `kwaklet`) and the store window load `libwebview.so`, which links against **WebKitGTK 4.1** (`libwebkit2gtk-4.1.so.0`). SteamOS's read-only system partition does not include it, and anything installed there with pacman is lost on the next update.

## The Flatpak runtime

When the host has no WebKitGTK 4.1, steamduck runs its WebKit windows inside the **Flatpak GNOME runtime** (`org.gnome.Platform//50`), which ships WebKitGTK 4.1 and its helper processes. `steamduck install` installs it per user from Flathub (`flatpak install --user`). This needs no root and survives SteamOS updates. It takes about 1 GB on disk. `steamduck runtime install` retries a failed install, and `steamduck runtime status` shows what is in use.

Only the window programs run in the sandbox; the kwakore service stays on the host:

- **Napplet windows.** `steamduck launch` makes a kwakore *attached launch* (`napplet.launch` with `attach:true`), puts the returned connection on its stdin and stdout, and execs `flatpak run --command=…/kwaklet org.gnome.Platform//50`. The window speaks to the daemon over that inherited connection, so the sandbox has **no network** (`--unshare=network`). It gets the kwakore release directory read-only, X11, GPU and sound. The process Steam started stays the window's.
- **The store.** `steamduck store` keeps its native administrative broker on the host and runs only the trusted built-in's `kwaklet` window inside the runtime. The child gets the inherited window protocol, with no socket or network access. Play opens an ordinary napplet window; Steam shortcut sync stays on the host. The older whole-store sandbox path remains available through `--legacy-window`.

WebKit's DMA-BUF renderer leaves windows blank inside the sandbox under X11, so it is turned off (`WEBKIT_DISABLE_DMABUF_RENDERER=1`).

## Choosing

`window_runtime` in `~/.config/steamduck/config.json` is `auto` (the default: the host's WebKitGTK when present, Flatpak otherwise), `host` or `flatpak`. `flatpak_runtime` changes the runtime ref. `STEAMDUCK_WINDOW_RUNTIME=flatpak` forces it for one command, which is how it was tested on a desktop that has WebKitGTK.

## Why not ship WebKitGTK ourselves

Release builds of WebKitGTK find their helper processes (`WebKitWebProcess`, `WebKitNetworkProcess`) only at a directory compiled in, such as `/usr/lib/webkit2gtk-4.1`. `WEBKIT_EXEC_PATH` works only in developer builds (`Source/WebKit/Shared/glib/ProcessExecutablePathGLib.cpp`). Shipping a copy under `~/.local` would mean building WebKitGTK ourselves for a home-directory prefix against SteamOS's libraries, then shipping its security updates. The Flatpak runtime is maintained by GNOME.

## Still to verify on a Steam Deck

- Game mode: gamescope shows the sandboxed windows (they have no `_NET_WM_PID`, because of the sandbox's process namespace).
- WebGL performance with DMA-BUF off.
- Steam overlay, audio and controller input (input comes through the host daemon).

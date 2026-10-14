# Feature request for kwakore: attached napplet launch

> **Status:** implemented in kwakore `3fe5936` ("add attached napplet launches and kwak run"). steamduck uses it as its default launch path. This document is kept as the rationale.

**From:** steamduck (Steam Deck integration for kwakore)
**Goal:** a caller such as a Steam shortcut can run a napplet window as *its own* process, instead of the daemon spawning it.

## Problem

Every launch path ends in `napplet.launch`, and the daemon then starts `kwaklet` itself (`backend/linuxhost/host_linux.go`, `OpenWindowContext`: `exec.Command(h.Program)` with `Setpgid`). The window is a child of `kwakore.service`, which has three consequences:

- **It uses the daemon's display, not the caller's.** Game mode on SteamOS (gamescope) and desktop mode use different displays, so steamduck has to rewrite the user manager's environment and restart the daemon.
- **Steam does not know the window belongs to the game it launched.** Gamescope only shows windows that carry the running game's `STEAM_GAME` X11 property, which Steam's overlay sets on the game's own processes. steamduck finds the window by scanning `/proc` for `KWAKORE_INSTANCE_ID` and sets the property itself.
- **Steam cannot see the game end, or end it.** steamduck keeps a process alive until a `window.closed` event arrives, and turns Steam's SIGTERM into `napplet.stop`.

If the process Steam starts *is* the window, all three go away.

## Requested feature

### 1. `napplet.launch` with `attach: true`

Add an optional named param `attach: boolean` (default `false`) to `napplet.launch`, and the same to `napplet.try` if that is simple. With `attach:true` the daemon does everything it does today before spawning: resolve the napplet, apply permissions, set up storage and signer, allocate the instance. It then **does not spawn** `kwaklet`. Instead it:

- Replies with the values it currently passes to the child as environment variables:
  ```json
  {"address":"…","window_id":"<instance>","outcome":"attached",
   "window":{"napp_id":"…","name":"…","instance_id":"…","width":1024,"height":700,
             "format":"napplet","theme":"…","theme_vars":"…"}}
  ```
- Turns that connection into the window's transport after the reply frame, the same way `events.subscribe` takes over a connection. From then on the connection carries exactly the newline-framed wire messages that `childTransport` exchanges with `kwaklet` over stdin and stdout.
- Implements the transport as a new `socketTransport` that satisfies `backend.Transport`:
  - `Send` writes a line to the connection.
  - `Focus` does nothing.
  - `Close` sends `{"t":"close"}` and then closes the connection after a grace period. It cannot kill a process the daemon did not start.
  - Lines read from the connection go to `backend.HandleMessage(instance, msg)`, with the same `nap.start` readiness handling as `readChild`.
  - End of stream, or a read or write error, calls the same path as a child exiting, so `window.closed` fires and the instance is cleaned up.
- Registers the transport with `backend.AttachServiceWindowTransport(instance, transport)` so the window appears in `napplet.windows`, emits `window.opened` and `window.closed`, and `napplet.stop` works on it.

Nothing else may arrive on the connection after the reply until the window has sent its first line. Otherwise a client reading the reply through a buffered reader could swallow window messages. Alternatively, document that clients must read the reply frame unbuffered.

### 2. `kwak run ADDRESS`

Add a CLI command that becomes the napplet window:

1. Connect to the standard socket with the usual server UID check, and send `napplet.launch` with `attach:true`. Accept the same address input as `kwak launch`: canonical address or `naddr`.
2. Read exactly one reply frame. On an error, print it as `kwak launch` does and exit non-zero.
3. Locate `kwaklet` and `libwebview.so` beside `kwak`, and run the same checks `checkProgram` runs today. Those checks move into the CLI, because the daemon no longer starts the program.
4. Put the connection on fd 0 and fd 1 (`dup2`), keep stderr, set the `KWAKORE_*` variables from the reply plus `WEBVIEW_PATH=<dir of kwaklet>`, keep the caller's environment (`DISPLAY`, `WAYLAND_DISPLAY`, Steam's variables), and `exec` `kwaklet`.

`kwak run` thereby turns into the window process with no relay in between. The window then:

- inherits the caller's display, and in Steam also the overlay and the game identity gamescope uses;
- exits when the user closes it, so Steam sees the game end;
- is killed directly by Steam's "Exit game";
- exits on its own when the daemon closes the connection. `kwaklet`'s reader already calls `w.Terminate()` when stdin reaches end of file (`desktop/child/main.go`), so `Close` and daemon shutdown work without a kill.

### 3. Documentation

- `docs/control-protocol.md`: the `attach` param, the `attached` outcome, the `window` object, and the connection takeover rule next to the events rule.
- `docs/service.md`: `kwak run`, and that native desktop entries may keep `launch-token` as they are.

## Compatibility

This only adds an optional param, an outcome value and a CLI command, which is compatible within protocol version 1. An older daemon rejects the unknown param with `Invalid params` (-32602); steamduck uses that to fall back to its current launcher. Advertising the feature in `service.status`, for example `features:["attach"]`, would let clients check without a failed call. That addition is optional.

## Trust note (maintainer decision)

Today the daemon chooses and verifies the window program. In attached mode, a client the daemon has already accepted chooses it. That client is the same user (checked with `SO_PEERCRED`), who can already run arbitrary programs. The window also receives no secrets beyond what the wire protocol already sends to a daemon-spawned child, so this looks like the same trust boundary. Please confirm, and keep the per-window protections (the `nap.start` token binding and the permission prompts) in the daemon, where they are now.

## Acceptance checks

- `kwak run ADDRESS` opens the napplet. `kwak windows` lists it, `kwak events` shows `window.opened`, and closing the window makes `kwak run` exit 0 and emits `window.closed`.
- `kwak run` started with `DISPLAY` different from the daemon's opens on the caller's display.
- `napplet.stop` on an attached window closes it and `kwak run` exits; killing the `kwak run` process fires `window.closed`.
- Daemon shutdown or restart closes attached windows cleanly.
- `napplet.launch` without `attach` is unchanged: existing tests pass, and desktop entries and the TUI still work.
- A regression test in `backend/daemon` drives an attached launch over a socket pair with a fake child speaking the wire protocol: readiness, a round trip, and close in both directions.

## What steamduck does once this exists

Shortcuts run `kwak run ADDRESS` through `steamduck launch`, which keeps the old path as a fallback. steamduck can then delete its display rewriting, the `/proc` scan, `STEAM_GAME` tagging and the wait-for-close loop.

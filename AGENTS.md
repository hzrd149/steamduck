# Repository guidance

Steamduck integrates the Kwakore napplet runtime with SteamOS. It provides a
Go CLI, Steam library synchronization, and a controller-friendly store napplet.

## Project layout

- `cmd/steamduck`: CLI entrypoint and commands.
- `internal/store`: Go store backend, runtime integration, and embedded assets.
- `internal/store/web`: TypeScript/JavaScript store UI and CSS; `main.ts` is the entrypoint.
- `internal/store/nap-steamduck`: trusted runtime binding and protocol types.
- `internal/kwakrpc`: Kwakore control socket client and protocol operations.
- `internal/launcher`, `internal/steam`, `internal/library`: napplet launch and Steam integration.
- `internal/installer`, `internal/flatpak`: installation and SteamOS runtime setup.
- `tests/store`: Playwright tests for the store UI and runtime binding.
- `docs`: runtime contracts, architecture, and build instructions.

## Development and validation

Use `nix develop` for the pinned toolchain, then `npm ci` to install frontend
dependencies. Without Nix, use Go 1.26 or newer and Node.js 22.12 or newer.

- `just build`: rebuild store artifacts and the local Go binary with cgo disabled.
- `just test`: rebuild store artifacts, run `go vet ./...`, and run `go test ./...`.
- `npm run check` (or `just test-store`): typecheck, build and verify store artifacts, and run Playwright tests.
- `just deck`: rebuild store artifacts and cross-build for Linux amd64.
- `just dev-store`: serve the compatibility UI against a local Kwakore daemon.

Run checks appropriate to the change before committing. Browser tests require
Playwright Chromium and its host libraries; see `docs/store-napplet.md`.
Report any checks that could not run and why.

## Store implementation rules

Edit frontend source, then run `npm run build` to regenerate the checked-in
`internal/store/built` artifacts. Commit generated artifacts with the source
changes; direct Go builds embed those files. Do not edit generated bundles by hand.

Keep the production napplet self-contained and use host-injected NAP APIs.
Keep native bindings, loopback HTTP transport, and browser gamepad polling in
the separate compatibility build. Preserve the private NAP-STEAMDUCK runtime
boundary and detect optional capabilities before using them.

Account for WebKit and opaque/custom-origin frames when using browser APIs.
Cover runtime compatibility fixes with focused regression tests where practical.
Consult `docs/store-napplet.md`, `docs/NAP-STEAMDUCK.md`, and
`docs/nap-steamduck-binding.md` when changing store or binding behavior.

## Git workflow

After making significant changes or adding features, run the relevant checks
and create a commit without waiting for a separate request, unless the user
explicitly asks otherwise. Include related tests, documentation, and regenerated
artifacts in the commit. Use a concise commit message describing the change.

Review the diff before committing. Stage only files belonging to the task;
preserve unrelated work. Do not amend existing commits, push, or rewrite history
unless requested. Finish by reporting the commit hash and validation results.

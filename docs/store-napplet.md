# Building the Steamduck store napplet

The store frontend lives in `internal/store/web`. Svelte 5 owns the application
shell, startup state, theme subscription, notifications, and keyed Library and
Discover cards. `src/main.ts` only mounts `src/App.svelte`. Reusable UI components
live in `src/components/`, views in `src/views/`, and shared presentation state
in `src/ui.ts`. Package/lock files, Vite, TypeScript, and Playwright configuration,
build scripts, and browser tests all live in this app directory.

Migration is incremental: account, settings, and napplet details currently remain
imperative feature controllers with markup compiled by `src/views/FeatureScreens.svelte`.
`index.html` is only the Vite entry document; it has no screen template or DOM
transplanting. `src/store.js` still coordinates feature operations and controller focus.
These are not yet a complete Svelte rewrite. New UI should use Svelte components,
component-local styles/state, and the existing host API boundary rather than
adding more DOM rendering helpers.

Vite compiles its module
entrypoint, styles and logo with the official `@napplet/vite-plugin`. Its
single-file mode produces an HTML artifact with every runtime asset inline and
a `.nip5a-manifest.json` sidecar describing the final artifact's SHA-256.
See the [official plugin documentation](https://napplet.run/docs/packages/vite-plugin.html).

## Build and artifacts

Use Node.js 22.12 or newer (CI uses Node 24):

```sh
cd internal/store/web
npm ci
npm run build
```

| File | Purpose |
| --- | --- |
| `internal/store/web/dist/index.html` | Production single-file napplet. |
| `internal/store/web/dist/.nip5a-manifest.json` | Unsigned publishing metadata and artifact hash. |
| `internal/store/built/napplet.html` | Checked-in production artifact, embedded by Go and exposed through `store.NappletPage()`. |
| `internal/store/built/steamduck.js` | Runtime-injected NAP-STEAMDUCK prelude, returned by `store.SteamduckPrelude()`. |

`internal/store/web/scripts/verify-store-build.mjs` verifies the artifact hashes, declared domains,
absence of external runtime assets and absence of compatibility transport in
the production napplet before refreshing the Go-embedded files. Commit those
files together with frontend source changes. Direct Go builds use the checked-in
artifacts; `just build`, `just deck` and CI rebuild them first.

## Native development launch

```sh
nix develop
npm --prefix internal/store/web ci
just store                 # rebuild frontend + Go, then open the actual napplet
just store --debug         # also enable the native webview inspector
just build-store           # frontend artifacts only; does not open a window
```

Run from a graphical desktop session with a compatible running Kwakore daemon
and installed window runtime. `just store` uses the same trusted built-in path
as the installed application, including runtime-owned gamepad snapshots. Closing
the window ends the store broker. Missing native support is an error, not an
automatic fallback to a different controller/transport implementation.

If launch reports `Method not found (-32601)` for built-in registration, the
running daemon is too old: update Kwakore to a version with trusted built-in
support and restart its daemon. `STEAMDUCK_KWAKORE_DIR` can point to a compatible
local runtime bundle (`kwakore`, `kwaklet`, `libwebview.so`), but changing the
window bundle alone does not update the running daemon. `steamduck doctor`
helps diagnose runtime, service and display dependencies.

The legacy build, `--legacy-window`, `--serve`, and `just dev-store` have been
removed. There is one store napplet, using host-injected APIs only. With a
controller attached, verify directional
focus movement, A activation, B back/modal dismissal, L1/R1 tab cycling, and
that focus loss pauses input and requires held buttons to be released on return.

The unsigned sidecar is build metadata. This bundled administrative store does
not gain trust by publishing or signing a manifest. Do not deploy it as an
ordinary remote napplet expecting administrative access.

## Runtime contract

The production napplet requires `window.napplet.steamduck` before its scripts run.
It calls `info()` to check bridge version 1, then uses the convenience operations
from [NAP-STEAMDUCK](NAP-STEAMDUCK.md). It displays an unavailable view when the
private domain is absent or incompatible. It does not install a runtime shim,
create a privileged namespace, use direct HTTP, or reach native Go bindings.

NAP-RESOURCE is required for image mediation. Optional domains are detected by
their actual injected presence:

- `gamepad`: controller snapshots from the runtime's native reader. Navigation
  stops while unavailable or unfocused; buttons held across focus regain must
  be released before they can act. Keyboard and pointer navigation remain usable.
- `theme`: the published `@napplet/nap/theme/sdk` helpers apply initial colors
  and subsequent theme changes. No fonts or assets are fetched from the network.

- `identity`: the shell-user profile, follow and relay lists, badges, received
  zaps, mutes, blocks and public categorized lists. The Account tab takes an
  initial public-key snapshot and responds to `identity.changed`; stale reads
  and image results are discarded when accounts change.
- `relay`: profile editing signs and publishes kind 0 through Kwakore. The editor
  reads the latest raw metadata, preserves unknown fields and event tags, and
  merges only changed fields into a fresh read before publishing. Signer denial
  leaves edits available for retry. Private keys never enter the editor.
- `resource`: profile and badge image bytes come through runtime mediation.
  Only bounded raster images are rendered, as document-owned blob URLs. Without this capability,
  the profile uses placeholder artwork and makes no direct network requests.
- `intent`: “Open profile app” and social account buttons dispatch
  `napplet:profile/open` with the selected public key.

The pinned SDK and Kwakore implementation have no `NAP-PROFILE` domain. Profile
reads are part of `NAP-IDENTITY`; updates use `NAP-RELAY`, and profile app routing
uses `NAP-INTENT` / the `profile` archetype. The app imports SDK-only entrypoints
and relies on the host's injected domains. Missing optional capabilities keep
existing sign-in and profile-reading behavior available. Received zap amounts
are converted from millisats to sats; these are public receipts, not a wallet
balance. Social lists are read-only, and private list contents are not decrypted.

The pinned published `@napplet/nap` 0.33.0 package does not yet export the draft
GAMEPAD SDK. The app therefore consumes `window.napplet.gamepad` directly behind
a small typed controller adapter. It can move to the official GAMEPAD SDK when
that entrypoint is published; the required runtime contract is unchanged.

Kwakore now supports native built-ins and custom domains. The separately built
[NAP-STEAMDUCK binding](nap-steamduck-binding.md) targets its lexical private
transport. Steamduck registers the approved store through its native broker;
catalog/install state stays in NAP-STEAMDUCK, while images use RESOURCE references.
Installed lists are paginated, and custom response envelopes stay within 256 KiB.
The original [Kwakore handoff](kwakore-custom-nap-handoff.md) is proposal history;
the new runtime's native-built-in documentation defines its implemented contract.

## Frontend development

Use `just store` to rebuild and exercise the real napplet and its controller
integration. `npm run dev` from `internal/store/web` starts Vite, but a plain
browser has no injected host capabilities and intentionally displays “Store
unavailable.” It is not a live Kwakore backend or controller fallback. Browser
tests provide isolated mock NAP domains rather than another application build.

## Kwakore settings page

The Settings tab edits `relays`, `blossom_servers`, `discover_on_user_relays`,
`theme`, `desktop_entries`, `sync_id` and `social_refresh`. GNOME search is hidden;
Steamduck installation configures it off when no explicit configuration exists,
while preserving existing configuration and daemon overrides. Signer credentials and account
management remain on the Account tab. The page works through NAP-STEAMDUCK in
the native runtime.

Settings save automatically: toggles and list changes save immediately, and
text fields save after a 500 ms typing pause. Requests run sequentially and
retain edits made during a save. Failed saves keep the edited value and offer
Retry saving. Drafts survive tab changes. The page reads effective values after
mutations and reports an acknowledged write separately from a failed read.

Relay and Blossom lists have single-address Add forms and per-address Remove
buttons. They preserve priority order, normalize host casing, schemes and ports,
and reject duplicate entries. Bare relay hosts
use `wss://`; bare Blossom hosts use `https://`. Relays require `wss://`, while
Blossom supports `https://` and `http://`. Credentials, query strings and
fragments are rejected. Empty arrays, `false`, an empty sync name and a zero
social-refresh interval are explicit settings, rather than resets.

`settings.inspect` shows each field’s source. Reset removes an override through
`settings.clear`, revealing the configured or built-in value; it does not write
hard-coded defaults. Reload configuration uses `settings.reload`. Older daemons
without inspection fall back to `settings.get`, showing only returned fields
and hiding unsupported reset/reload controls. Settings are local daemon
preferences; editing discovery relays does not publish a Nostr relay-list event.

## Verification

```sh
cd internal/store/web
npx playwright install chromium
npm run check
cd ../../..
go vet ./...
go test ./...
```

Browser tests load the production artifact inside an opaque `allow-scripts`
iframe with restricted CSP and injected test domains. They cover store/account
calls, theme integration, missing/incompatible APIs, public errors, controller
focus isolation and the single production artifact. Account tests cover
identity-driven refresh, late results after switching accounts, mediated images,
public social data, profile intents, metadata preservation and denied signing. They also reject
unexpected network requests and native gamepad polling from the napplet.

These tests use a mock NAP-STEAMDUCK host, not an implemented Kwakore extension.
Real Kwakore/WebKit and physical Steam Deck verification follow the native
integration. The build emits the custom `steamduck` required-domain tag with an
expected plugin warning because that private domain is outside its stock registry.

## Native store testing

The native path requires Kwakore `c9b4016c2884b04bbedb9a542bfcaf55bf6a8ebf`
or a later version containing built-in RESOURCE support, cached original Blossom
icons and 180-second custom route deadlines. The release workflow pins this
revision. The trusted built-in requests gamepad, theme, storage and RESOURCE;
other optional account domains are used only when the runtime supplies them.

An assembled development runtime is available locally in `dist/testing` with
`steamduck`, `kwakore`, `kwak`, `kwaklet` and `libwebview.so`. This opt-in smoke
starts a separate daemon under temporary XDG directories and runs the actual
store, injected extension and stock domains in WebKit without touching Steam:

```sh
NO_AT_BRIDGE=1 STEAMDUCK_STORE_SMOKE=1 \
  STEAMDUCK_KWAKORE_DIR="$PWD/dist/testing" \
  xvfb-run -a go test ./internal/store -run '^TestRealBuiltinStore$' -count=1 -v
```

For a live store, first run/install the matching Kwakore daemon, then launch:

```sh
STEAMDUCK_KWAKORE_DIR="$PWD/dist/testing" ./dist/testing/steamduck store --debug
```

On a physical Deck, verify controller navigation, held-button release after
focus changes, Play opening an ordinary napplet, and return to the store after
closing the game. Check that image fetch denial leaves usable placeholders and
that Sync Steam shortcuts still runs on the host. Xvfb checks cannot substitute
for physical controller, gamescope, Steam overlay or Flatpak verification.

# Building the Steamduck store napplet

The store frontend lives in `internal/store/web`. Vite compiles its module
entrypoint, styles and logo with the official `@napplet/vite-plugin`. Its
single-file mode produces an HTML artifact with every runtime asset inline and
a `.nip5a-manifest.json` sidecar describing the final artifact's SHA-256.
See the [official plugin documentation](https://napplet.run/docs/packages/vite-plugin.html).

## Build and artifacts

Use Node.js 22.12 or newer (CI uses Node 24):

```sh
npm ci
npm run build
```

| File | Purpose |
| --- | --- |
| `dist/store/index.html` | Production single-file napplet. |
| `dist/store/.nip5a-manifest.json` | Unsigned publishing metadata and artifact hash. |
| `dist/store-legacy/index.html` | Compatibility build for the existing standalone window and loopback development server. |
| `internal/store/built/napplet.html` | Checked-in production artifact, embedded by Go and exposed through `store.NappletPage()`. |
| `internal/store/built/legacy.html` | Checked-in compatibility artifact, returned by the current `store.Page()`. |

`scripts/verify-store-build.mjs` verifies the artifact hashes, declared domains,
absence of external runtime assets and absence of compatibility transport in
the production napplet before refreshing the Go-embedded files. Commit those
files together with frontend source changes. Direct Go builds use the checked-in
artifacts; `just build`, `just deck` and CI rebuild them first.

The unsigned sidecar is build metadata. This bundled administrative store does
not gain trust by publishing or signing a manifest. Do not deploy it as an
ordinary remote napplet expecting administrative access.

## Runtime contract

The production napplet requires `window.napplet.steamduck` before its scripts run.
It calls `info()` to check bridge version 1, then uses the convenience operations
from [NAP-STEAMDUCK](NAP-STEAMDUCK.md). It displays an unavailable view when the
private domain is absent or incompatible. It does not install a runtime shim,
create a privileged namespace, use direct HTTP, or reach native Go bindings.

Optional domains are detected by their actual injected presence:

- `gamepad`: controller snapshots from the runtime's native reader. Navigation
  stops while unavailable or unfocused; buttons held across focus regain must
  be released before they can act. Keyboard and pointer navigation remain usable.
- `theme`: the published `@napplet/nap/theme/sdk` helpers apply initial colors
  and subsequent theme changes. No fonts or assets are fetched from the network.

The pinned published `@napplet/nap` 0.33.0 package does not yet export the draft
GAMEPAD SDK. The app therefore consumes `window.napplet.gamepad` directly behind
a small typed controller adapter. It can move to the official GAMEPAD SDK when
that entrypoint is published; the required runtime contract is unchanged.

Kwakore integration is still pending. The native host must launch this HTML as
its trusted built-in store, inject the custom API, and provide the standard
domains through the shared session. The implementation plan is in the
[Kwakore handoff](kwakore-custom-nap-handoff.md). No Kwakore runtime changes or
production launch-path switch are part of this frontend migration.

## Compatibility and live development

The current `steamduck store` command continues to use the compatibility build.
Its adapter maps the existing Go binding or token-protected loopback HTTP calls
to the same frontend interface. It is compiled out of the production napplet.
Browser-native gamepad polling is retained only in this compatibility build;
it does not resolve the standalone WebKit controller issue.

For the existing browser development path:

```sh
go run ./cmd/steamduck store --serve 127.0.0.1:8080
```

Open the URL it prints, including its random token. For Vite live reload, leave
that backend running and start this in a second terminal:

```sh
npm run dev
```

Open Vite's loopback URL with the same `#token=…` fragment from the backend URL.
Vite proxies `/api` to `127.0.0.1:8080`; requests retain the backend's token check.
The compatibility adapter is available only in the legacy build/development mode.
It never replaces a real injected NAP-STEAMDUCK domain.

## Verification

```sh
npx playwright install chromium
npm run check
go vet ./...
go test ./...
```

Browser tests load the production artifact inside an opaque `allow-scripts`
iframe with restricted CSP and injected test domains. They cover store/account
calls, theme integration, missing/incompatible APIs, public errors, controller
focus isolation and the separate compatibility artifact. They also reject
unexpected network requests and native gamepad polling from the napplet.

These tests use a mock NAP-STEAMDUCK host, not an implemented Kwakore extension.
Real Kwakore/WebKit and physical Steam Deck verification follow the native
integration. The build emits the custom `steamduck` required-domain tag with an
expected plugin warning because that private domain is outside its stock registry.

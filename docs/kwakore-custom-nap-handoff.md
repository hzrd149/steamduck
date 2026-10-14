# Handoff: custom NAP domains in Kwakore

Status: implementation proposal, not an implemented API. Prepared 2026-10-13.

Update: Kwakore implemented native built-ins/custom domains in commit `2c999cf`.
This document retains the original proposal; some proposed details differ from
the shipped private-port transport and limits. The
[binding integration guide](nap-steamduck-binding.md) records the actual contract
and remaining Steamduck adapter work.

## Objective

Allow a native application using Kwakore to supply custom NAP bindings and
handlers for a trusted built-in napplet. The same window must retain Kwakore's
standard NAP domains, session lifecycle, permissions and controller handling.

Steamduck is the first consumer. Its bundled store needs
`window.napplet.steamduck` as defined in [NAP-STEAMDUCK](NAP-STEAMDUCK.md), plus
NAP-GAMEPAD for controller navigation. Ordinary napplets must never receive
Steamduck's administrative domain, even if they copy the store's manifest,
address, display name or HTML.

Implement the extension machinery generically in Kwakore. Steamduck owns its
custom JavaScript binding and Go business logic; Kwakore must not import
Steamduck, embed the store UI, or hard-code its convenience method table.

## Why this needs runtime support

The current Steamduck store is a separate WebKit window. Its page uses the
`steamduckCall` Go binding and polls the browser's `navigator.getGamepads()`.
Kwakore's napplets instead receive controller snapshots read by SDL2 in the
host daemon through NAP-GAMEPAD. Successful controller input in a napplet does
not demonstrate that the browser API in the standalone store works.

Kwakore already provides the desired runtime behavior. Reuse its existing
sandboxed napplet window instead of recreating the standard NAP handlers in
Steamduck. Installing a JavaScript namespace alone is insufficient: the daemon
must recognize its messages, scope them to the right session and route results
back to the same document generation.

## Current integration points

These paths are relative to the Kwakore repository. They were inspected at
checkout HEAD `3fe5936b3f38ab91fccc217939e65c5093f11e07`; verify their current
contents before editing.

| File | Existing responsibility | Extension work |
| --- | --- | --- |
| `backend/nap.go` | `napDomains`, `napSession`, enqueue/dispatch, async replies, start/reset/close, boot/srcdoc generation | Add per-instance domain and route resolution; bind custom work to the existing session context and generation. |
| `backend/nap_route.go` | Explicit gates, failure shapes, size bounds, correlators, dispatch limits and built-in registration | Define validated extension route metadata without weakening the built-in table. |
| `backend/window_instances.go` | Instance identity, document loading, storage identity, trial state, attached transport selection | Create an explicit built-in instance kind; attach native extension ownership without treating it as a temporary remote napplet. |
| `backend/webview/napplet.go` | CSP, pristine shim activation, catalog/gamepad preludes, document marker | Insert approved custom binding code in the trusted preamble alongside the stock domains. |
| `backend/webview/napplet-host.js` | Frame source validation, session lifecycle, envelope bounds, failure shapes and gamepad focus reporting | Carry custom envelopes and their failure metadata through the same authenticated path. |
| `backend/wire.go` | Backend/window wire messages and transport bounds | Introduce separate native extension frames if using the attached companion arrangement below. |
| `backend/daemon/attached_linux.go` | Unix connection takeover and window transport | Extend attachment to a built-in window and native extension broker without giving iframe code native broker authority. |
| `desktop/child/napplet.go` | Hardened WebKit window, top-frame native token, host page and sandboxed frame | Keep this window implementation; do not add another privileged direct binding to the napplet frame. |
| `backend/nap_conformance_test.go` and `backend/webview/*_test.go` | Built-in domain/shim conformance and host behavior checks | Preserve existing fixtures; test private domains separately and test combined per-window routing. |

Important current details:

- The domain list and route lookup are global today. Merely appending a custom
  domain to `napDomains` would advertise it to unrelated napplets.
- Session establishment is a trusted host action (`nap.start`), not an iframe
  handshake. Preserve that boundary.
- `napPushGen` guards replies against stale document generations. Extension
  results and pushes need the same guard.
- The stock prelude's document marker prevents a replacement frame document
  from continuing the old session. Preserve marker ordering and reload checks.
- Go and the host page maintain matching failure shapes. A private request with
  a structured numeric error needs support on both sides; a generic string
  failure would leave NAP-STEAMDUCK's error contract inconsistent.
- Preview HTML exists, but preview instances have trial behavior. A trusted
  built-in store is a distinct lifecycle, not an ordinary `try` invocation.

## Proposed architecture

### 1. Native registration of a built-in definition

Provide a native registration/launch contract containing:

| Item | Purpose |
| --- | --- |
| Stable built-in key | Host-owned identity for lifecycle and persistent storage; distinct from remote Nostr napplet identities. |
| Bundled HTML and its digest | The installed native application's assets; verified before boot and rebuilt from the approved bytes on reload. |
| Display metadata | Window title and size; descriptive, never authorization. |
| Requested standard domains | Domains the built-in wants; intersected with what this runtime actually supports and its policy allows. |
| Custom domain definitions | Trusted binding code plus validated routes and native handler ownership. |
| Native handler channel | The owning application's endpoint for custom requests, replies and cancellation. |

The registration contract is native administrative functionality. Never expose
registration through `nap.msg`, manifest capability tags, a normal permission
prompt, or an iframe-callable RPC. Never accept `trusted: true` from napplet code.

For an embedded host, registration can be a typed Go API. For Steamduck's existing
separate-process deployment, support an authenticated local Unix control/attach
path. The names below are proposals, not existing methods:

```text
runtime.builtin.open(definition) -> attached window + extension broker
```

Only the native host supplies the definition. If definitions are persisted,
updates must come from the same native administrative authority, never relay
discovery. A textual built-in key does not grant authority to another endpoint.

For Steamduck, load HTML and binding code embedded in the installed Steamduck
binary. The native application vouches for those bytes over the authenticated
administrative channel. The release packaging and native launch path establish
trust; Kwakore does not infer it from a claimed store address.

Kwakore's current Unix control socket authorizes same-user native clients. This
design preserves that threat boundary: it protects against untrusted napplet
code, not against arbitrary native code already executing as that user. Do not
claim that peer UID verification authenticates a specific application binary.

### 2. Per-instance extension registry

Keep built-in routes immutable and retain them for ordinary napplets. A built-in
instance holds an additional immutable registry whose lifetime is owned by its
native host. Resolve a request against that instance's authorized registry,
never a global registry updated by a launch request.

Each custom domain definition should declare:

- A bare domain name and binding version.
- Trusted JavaScript prelude bytes, separate from untrusted napplet HTML.
- Exact inbound request types and permitted outbound response/push types.
- Correlator requirements, response envelope and public error schema.
- Raw message size, response size, concurrency/rate limits and request deadlines.
- Native handler ownership and teardown behavior.

Reject malformed names, duplicate domain definitions, collisions with stock
domains and collisions with lifecycle/internal namespaces. Custom code must not
replace `nap.start`, `nap.reset`, `nap.boot`, the document marker, gamepad controls
or the privileged native RPC wrappers. Reject registrations exceeding existing
transport bounds rather than increasing the bounds globally.

For v1, support request/result custom routes. Subscription and unsolicited push
support can be added through explicit lifecycle declarations later. Do not
promise subscriptions while silently treating them as unary calls.

The extension API needs a failure adapter capable of returning structured
`{code, message, reason?, data?}` errors. Existing string-based standard domain
failures must remain unchanged. Bounds, validation, disconnection, timeout and
panic recovery must all settle a custom promise in its declared result shape.

### 3. Reuse standard domains

The same instance uses the stock NAP handlers for `gamepad`, `theme`, `storage`,
`identity`, `relay` and the other supported domains. Their permissions and
subscriptions stay in the daemon, independent of custom handlers.

Use the current runtime domain list as the supported inventory, not a copied
Steamduck list. At the inspected revision it contains `relay`, `identity`,
`storage`, `resource`, `common`, `theme`, `inc`, `intent`, `link`, `upload`,
`outbox`, `media`, `config`, `notify`, `catalog` and `gamepad`. Inject only domains
that are usable for this instance; a built-in must not claim unsupported APIs.

Audit standard handlers' assumptions about `Napp`, publication author, manifest
hash and installation paths. Define an internal built-in identity/storage
mapping where appropriate. Never fabricate a signed manifest or Nostr author
to satisfy those assumptions. Omit a domain with unresolved identity requirements
until its built-in behavior is defined.

Prioritize `gamepad`, `theme` and isolated `storage` for the first built-in
consumer. Custom administrative access does not implicitly bypass the consent
rules of a standard domain. The stored namespace must remain isolated from
remote napplets and stable across native application upgrades; instance-scoped
storage remains unique per launch.

### 4. Preamble installation

Extend srcdoc construction to take approved custom preludes alongside the stock
domain list. Run the pristine shim first, install approved custom domain objects
in the trusted preamble, and preserve the document marker before any eager
subscription or user script can send messages.

The custom binding should use the existing NAP `postMessage` transport and
accept results only from its parent host. It should add only its own
`window.napplet.<domain>` object; keep implementation globals private. The shim
must not manufacture a stock domain implementation for an unknown custom name.

Preserve the sandbox, CSP, source checks, native window secret and engine
hardening used by ordinary napplets. Do not relax network or frame permissions
to make the built-in work; its native handler performs privileged operations.
The extension prelude builder must prevent inline script termination by supplied
code and validate UTF-8. Run tests on the actual WebKit engine, not only Node.

### 5. Out-of-process handler transport

Steamduck already talks to the daemon over its Unix control socket and launches
attached window processes. Keep its business logic in that native process and
broker custom requests through a session-bound native channel.

A practical first implementation uses a native proxy between the daemon's
attached connection and the stock window program. The proxy forwards ordinary
window messages and intercepts extension request frames for its own handlers.
An independent companion connection is also viable, but must have equivalent
ownership, authentication and cancellation guarantees. Choose and document one
carrier before implementation; this handoff does not assume either exists.

Illustrative native frames for a proxy carrier:

```json
{"t":"extension.request","request_id":"native-1","instance":"window-1","generation":4,"domain":"steamduck","envelope":{"type":"steamduck.request","id":"ui-1","method":"sync"}}
{"t":"extension.response","request_id":"native-1","envelope":{"type":"steamduck.request.result","id":"ui-1","result":{"deferred":true,"shortcut_count":12}}}
{"t":"extension.cancel","request_id":"native-1"}
```

These are proposed native frames, not NAP messages and not existing `WireMsg`
variants. The daemon supplies identity, generation and opaque native request ID.
An extension handler cannot choose another target instance or generation.

- Reserve a request-ID namespace distinct from window RPC IDs and NAP IDs.
- Accept native replies only from the broker owning that built-in instance.
- The proxy must reject extension frames originating from the window child;
  they must never be forwarded into the trusted broker lane.
- The daemon validates replies against the pending request and permitted output
  type, then delivers through its existing generation-checked NAP push path.
- Never expose a generic `eval` writer to the custom handler. A reply contains
  validated data, not JavaScript to execute in the host or another window.
- Serialize carrier writes, bound pending work and queue bytes, and process
  handlers concurrently so a long pairing wait cannot block browsing or focus.
- End the session and cancel extension work on reset, close or native broker
  disconnect. Reject new extension calls immediately if their owner is gone.
- Late, duplicate and unrecognized native replies are dropped. A document reload
  cannot receive the prior document's result, even if it reuses a NAP ID.
- A queued mutation cancelled before execution must never run. Work already
  committed may remain committed; do not imply rollback or automatically retry.

The stock child may remain unchanged if the native proxy filters all extension
frames. Explicitly test that it never receives private broker frames or secrets.

## Steamduck adapter contract

Register the `steamduck` custom domain only for the bundled store instance. Its
v1 custom routes are:

| Request | Response | Handler |
| --- | --- | --- |
| `steamduck.info` | `steamduck.info.result` | Bridge metadata, even when the daemon is unavailable. |
| `steamduck.request` | `steamduck.request.result` | Steamduck's validated convenience method table. |
| `steamduck.kwakore.request` | `steamduck.kwakore.request.result` | Direct unary daemon control operations. |

The complete parameter/result/error contract is in NAP-STEAMDUCK; do not copy its
business logic into Kwakore. Direct daemon operations use a separate connection
so they cannot deadlock the attached NAP session. Reject stream takeover methods
(`events.subscribe`, attached launch) through the unary API.

Built-in registration and extension transport setup are native-only management
operations. They MUST NOT be forwarded by `steamduck.kwakore.call`, even though
the trusted store has broad access to the daemon's ordinary control API. This
preserves the rule that iframe messages cannot establish native extension trust.
If Kwakore adds such management methods to its control endpoint, update
NAP-STEAMDUCK's direct-control section with this explicit exclusion.

The current Steamduck implementation points are:

- `internal/store/api.go`: store method dispatch, launches and public errors.
- `internal/store/account.go`, `social.go`, `profile.go`: account and social logic.
- `internal/store/serve.go`: embedded store HTML assembly and browser development mode.
- `internal/store/window.go`: standalone window to replace after built-in support exists.
- `internal/kwakrpc/attached.go`: current attached launch client and environment.
- `internal/flatpak/`: window runtime and store sandbox launch paths.
- `internal/kwakrpc/client.go`: currently retries non-daemon errors once. Add a
  non-replaying request mode for the new privileged adapter, including forks;
  a lost response may follow a committed mutation.

Keep the browser development transport usable. The production store transport
becomes `window.napplet.steamduck.call`, and controller polling becomes
`window.napplet.gamepad.getGamepads()` or NAP-GAMEPAD change notifications. Do not
depend on browser `navigator.getGamepads()` being replaced: Kwakore's current
compatibility prelude only replaces it under particular browser policy conditions.

Signer credentials remain native write-only inputs. Pairing offers may contain
their one-time secret and must reach only the owning trusted store generation.
Catalog descriptions, profile data and comments displayed by the administrative
store remain untrusted data and must not become executable HTML.

## Implementation sequence

1. Define validated extension metadata, per-instance registration and a distinct
   built-in identity/document lifecycle. Establish the native registration
   boundary before adding any privileged route.
2. Add per-instance route/domain resolution and custom preamble composition.
   Preserve stock route and shim conformance for ordinary napplets.
3. Implement the native handler carrier, correlated responses, cancellation,
   structured failures and bounded concurrent work.
4. Add a fake custom domain to runtime tests and prove isolation, stock-domain
   coexistence and teardown before integrating Steamduck.
5. Add a built-in attached launch path using the stock hardened window program.
   Define launch ownership, close behavior and storage without trial semantics.
6. Integrate Steamduck's adapter and bundled page, preserving Steam session
   inheritance and Flatpak operation. Follow the NAP-STEAMDUCK deadlines.
7. Validate the combined window on WebKit and a physical Steam Deck, then remove
   the production store's standalone privileged webview transport.

## Required verification

| Scenario | Required outcome |
| --- | --- |
| Ordinary napplet requests or forges a custom type | No custom binding, handler execution or privileged data. |
| Remote napplet copies the store's metadata/document | Still an ordinary napplet; no extension grant. |
| Two built-ins register the same custom domain separately | Each routes only to its own native handler and data. |
| Registration collides with a standard/internal type | Rejected before the window boots. |
| Custom and stock calls run concurrently | Both work in one session; custom waits do not block controller delivery. |
| Envelope/response exceeds a limit | Declared bounded failure; no hanging promise or session escape. |
| Duplicate pending IDs or duplicate native replies | At most one dispatch and one terminal answer to the original request. |
| Frame reload, navigation, reset or close | Old work cancelled; late results cannot reach the new generation. |
| Native handler disconnects or panics | Pending custom requests settle safely; no raw errors or leaked resources. |
| Window child forges a broker response | Rejected at the native carrier boundary. |
| Custom handler attempts cross-window delivery or eval | Rejected; handler can reply only to its authorized session. |
| Built-in store uses theme and isolated storage | Stock contracts hold without fabricated remote identity or trial prompts. |
| Gamepad in store, then launched game, then store again | Only the focused session responds, using Kwakore's SDL reader. |
| Direct privileged mutation loses its response | No automatic replay; UI can re-read relevant state. |
| Store closes | Broker and store resources close; daemon and unrelated windows remain alive. |
| Flatpak store/child run without network | Native broker and standard domains still work through their inherited/control connections. |

Follow Kwakore's repository instructions for backend, desktop and actual-engine
checks. Run its standard conformance suite, `go vet` and `go test` in both
modules. Any host/prelude/child change also needs the documented WebKit smoke
tests under a display or Xvfb, with PASS rather than SKIP. Physical Steam Deck
focus and controller tests remain required acceptance work beyond CI.

## Completion criteria for the Kwakore change

A native application can launch its approved built-in HTML with one custom NAP
domain, use standard NAP-GAMEPAD/theme/storage alongside it, and serve custom
requests outside Kwakore without changing Kwakore for each domain. A normal
remote napplet cannot acquire or invoke that extension. The handoff includes the
actual exported registration types/control contract, carrier framing, failure
metadata, supported built-in standard domains and a minimal working native-host
example so Steamduck can integrate without inventing another runtime.

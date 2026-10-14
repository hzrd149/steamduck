# Runtime-injected NAP-STEAMDUCK binding

The injected JavaScript binding is implemented in
`internal/store/nap-steamduck/binding.ts`. Its shared application/runtime types
are in `types.ts`. It exposes the [NAP-STEAMDUCK](NAP-STEAMDUCK.md) v1 operations:

```js
const store = window.napplet.steamduck;
const info = await store.info();
if (info.version !== 1) throw new Error("Unsupported store bridge");
const installed = await store.call("installed");
const status = await store.kwakore.call("service.status");
```

Callers check the bridge version before mutations. The store application already
does this and verifies advertised convenience methods. The binding does not gate
`info()` or send an application identity/authentication handshake.

## Build and injection

```sh
npm --prefix internal/store/web run build:binding
```

This creates `internal/store/web/dist/binding/steamduck.js` and refreshes the identical,
checked-in `internal/store/built/steamduck.js`. The full `npm run build` includes
this step. The Go binary embeds it and exposes it as `store.SteamduckPrelude()`.

The artifact is a classic-script IIFE with no external assets or installer
globals. It is intentionally separate from the app's HTML. The app imports only
its shared types, never the runtime installer or prelude.

Kwakore commit `2c999cf` introduced native built-ins and the lexical helper
`napExtension.call(type, fields, deadlineMS?)`. This binding uses that helper;
it does not create a second `postMessage` transport. Kwakore owns the message
port, request IDs, response correlation, transport timeout and document-root
replacement checks. Neither the helper nor the private port is a window global.

The native host registers the artifact as the `prelude` of its `steamduck`
extension, only on the approved bundled store's built-in definition:

```text
extension.name = "steamduck"
extension.version = "1"
extension.prelude = store.SteamduckPrelude()
extension.routes = [
  steamduck.info,
  steamduck.request,
  steamduck.kwakore.request
]
```

Each route must also declare Kwakore's request/response size bounds, concurrency,
rate and deadline. The native definition includes `store.NappletPage()`, its
exact SHA-256, a stable storage key, and requested stock domains such as
`gamepad`, `theme`, `storage` and `resource`. Do not add `steamduck` to the stock shim's global
domain list; it is installed by this extension's own approved prelude.

The prelude runs inside Kwakore's trusted scope after the stock namespace and
document marker and before application code. Its entrypoint passes the lexical
helper to `installSteamduckBinding(window, napExtension)`. Loading the artifact
as an ordinary standalone script without that helper fails; it is not a browser
polyfill or a grant mechanism.

An embedded runtime can import the TypeScript installer and retain its returned
`dispose()` handle. Installing it again through the same installer returns the
existing handle. Another binding already owning the domain is never overwritten.
The generated prelude keeps the handle private and disposes local waits on
`pagehide`; the runtime's own private transport handles document replacement.

## Binding behavior

- Adds only a frozen `window.napplet.steamduck` object and its frozen `kwakore`
  member, preserving all standard domains already installed.
- Sends `steamduck.info`, `steamduck.request` or `steamduck.kwakore.request`
  through the private helper. The helper supplies IDs; the app never supplies
  an instance, generation, socket path or token.
- Resolves directly to validated JSON results, including explicit null success.
  Rejects with `SteamduckError` carrying `code`, `message`, optional `reason`
  and `data`. Public native errors retain their code and safe payload.
- Rejects invalid parameter types, cycles, non-finite numbers, functions,
  undefined values, symbols and non-JSON objects before dispatch. Error messages
  never echo parameter values. Accessor properties are not executed.
- Rejects raw `napplet.icon`, event subscriptions, attached launches and native-only
  `runtime.builtin.*` / `runtime.extension.*` management calls locally. The native
  dispatcher must enforce those exclusions independently.
- Bounds local pending calls at 64. Closing rejects current/future waits with
  code 1007. There is no retry, reconnect or claim of mutation rollback.
- Requests the method-specific draft host budget plus five seconds of transport
  overhead. The runtime helper caps local waits at 185 seconds.

Kwakore's reserved runtime failures are mapped to the store's public errors:
deadline → 1008 Timeout; rate/concurrency → 1003 Busy; document-ended → 1007
Closing; invalid-request → -32602 Invalid params; other internal, malformed or
size failures → -32603 Internal error. This preserves existing UI explanations
without returning raw internal errors.

## Native integration and resource budgets

`steamduck store` registers the embedded HTML and extension through the native
builtin broker. The host dispatches validated convenience requests and direct
control calls on separate authenticated connections, without mutation retries.
Only the approved built-in gets this extension. Games launch as ordinary napplets.
The store has only the trusted napplet window; no standalone compatibility
binding or HTTP transport is installed.

Custom route deadlines allow 180 seconds, and the private helper allows 185
seconds. Custom responses remain capped at 262,144 bytes. Icons and profile images
are NAP-RESOURCE references, not base64 payloads. Installed/catalog/social lists
are paginated; oversized convenience pages shorten with a correct continuation.
Direct results preserve daemon schemas and fail explicitly if oversized.

The built-in requests RESOURCE. Original cached icons resolve via
`blossom:<hash>.<ext>` without network access; a miss uses normal permission-controlled
Blossom fetching. Remote HTTPS images retain the
normal fetch permission, network policy, MIME checks and resource byte limits.

## Verification

```sh
npm --prefix internal/store/web run typecheck
npm --prefix internal/store/web run build
npm --prefix internal/store/web test
go vet ./...
go test ./...
```

Browser tests exercise the actual built prelude with a captured Kwakore
`napExtension` helper based on `2c999cf`, with the 185-second timeout update in an opaque sandbox. They check IDs,
out-of-order results, private-port isolation, structured failures, invalid inputs,
runtime deadlines, no replay, lifecycle cleanup, bounded concurrency and a full
store round trip. The fixture is test-only; Kwakore supplies the real helper at
runtime. Go tests cover native dispatch, socket brokerage and mutation replay prevention.
Physical Steam Deck controller testing is still required.

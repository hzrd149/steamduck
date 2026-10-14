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
npm run build:binding
```

This creates `dist/store-binding/steamduck.js` and refreshes the identical,
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
`gamepad`, `theme` and `storage`. Do not add `steamduck` to the stock shim's global
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
- Rejects event subscriptions, attached launches and native-only
  `runtime.builtin.*` / `runtime.extension.*` management calls locally. The native
  dispatcher must enforce those exclusions independently.
- Bounds local pending calls at 64. Closing rejects current/future waits with
  code 1007. There is no retry, reconnect or claim of mutation rollback.
- Requests the method-specific draft host budget plus five seconds of transport
  overhead. Kwakore's current helper may apply a lower cap, described below.

Kwakore's reserved runtime failures are mapped to the store's public errors:
deadline → 1008 Timeout; rate/concurrency → 1003 Busy; document-ended → 1007
Closing; invalid-request → -32602 Invalid params; other internal, malformed or
size failures → -32603 Internal error. This preserves existing UI explanations
without returning raw internal errors.

## Native integration still required

The binding and compiled store can now be registered with Kwakore's implemented
extension mechanism. Steamduck still needs the native registration/proxy launch
and extension handler that dispatches to its Go store API. Production
`steamduck store` still uses the compatibility window until that adapter is
implemented and verified. Authorization remains in Kwakore's per-instance,
per-generation route handling; the JavaScript binding cannot grant native access.

Two current Kwakore bounds need resolution for the complete draft contract:

1. Route deadlines are at most 120,000 ms and the helper caps local waits at
   125,000 ms. The draft requires up to 180 seconds for install/update and direct
   control calls, and 135 seconds for pairing waits. The binding requests those
   budgets but cannot override the runtime's limits. Supporting the full draft
   requires increasing both runtime bounds or explicitly redesigning long
   operations; it must not silently replay timed-out work.
2. Custom responses are at most 262,144 bytes. Existing complete installed lists,
   social results, icons and profile images can exceed that. Profile downloads
   alone currently allow up to 3 MiB per image before base64 encoding. The native
   adapter needs bounded result/image handling or a larger explicit runtime
   response allowance before all convenience operations can be claimed supported.

These limitations are recorded rather than changing the draft's operation
semantics or introducing undocumented streaming operations.

## Verification

```sh
npm run typecheck
npm run build
npm test
go vet ./...
go test ./...
```

Browser tests exercise the actual built prelude with a captured Kwakore
`napExtension` helper from `2c999cf` in an opaque sandbox. They check IDs,
out-of-order results, private-port isolation, structured failures, invalid inputs,
runtime deadlines, no replay, lifecycle cleanup, bounded concurrency and a full
store round trip. The fixture is test-only; Kwakore supplies the real helper at
runtime. Native broker, real WebKit and Steam Deck testing remain part of the
subsequent launch/handler integration.

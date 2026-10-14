NAP-STEAMDUCK
=============

Trusted Steamduck Store Control
------------------------------

`draft` · version `1` · JavaScript binding and trusted native adapter implemented; physical Deck verification pending

**NAP ID:** NAP-STEAMDUCK  
**Domain:** `steamduck`  
**Web binding:** `window.napplet.steamduck`; domain presence signals availability.

## Description

NAP-STEAMDUCK gives Steamduck's trusted, bundled store napplet privileged access
to Kwakore's control API and Steamduck's store operations. The native host owns
the daemon connection, Steam integration, signer credentials and process
lifecycle. The store consumes structured requests and results over the napplet
transport instead of receiving a socket, filesystem access or command runner.

This is a private administrative extension, not a capability offered to ordinary
napplets. Only the trusted built-in store endpoint may receive it. Access to the
Kwakore control API is intentionally broad; the security boundary is the trusted
store's identity, not the limited permissions of a normal napplet.

Controller input belongs to NAP-GAMEPAD. The store declares and uses that domain
independently. NAP-STEAMDUCK defines no controller reader, gamepad messages or
replacement for focus arbitration. Standard NAP domains remain independently
usable and do not wait for this domain's readiness.

Packaging, code injection, trusted identity representation and handler
registration are subsequent implementation work. This document specifies the
behavior they must preserve.

## Trust and availability

- The host MUST establish the bundled store's trusted identity before exposing
  this domain or servicing any of its messages.
- Trust MUST be assigned by the native host to the store assets shipped with the
  installed Steamduck release. A name, address, publisher key, manifest domain
  declaration, caller-supplied flag or readiness message MUST NOT confer trust.
- A remotely obtained napplet, including one claiming to be the store, MUST NOT
  receive this domain. Ordinary permission prompts MUST NOT grant it.
- The host MUST authenticate the sending endpoint and its current generation on
  every request. Merely withholding the JavaScript binding is insufficient.
- Other windows, nested frames and napplets launched by the store MUST NOT inherit
  the store's privilege. They use their own identities and grants.
- Navigation, reload, replacement or endpoint destruction MUST invalidate the
  old generation's access and pending response delivery. A new generation MUST
  be authorized independently before receiving access.
- An unauthorized endpoint MUST cause no side effect and receive no privileged
  data. If the transport returns a correlated denial, it uses `Unauthorized`.

Domain absence means the extension is unavailable. Its presence does not imply
that the daemon, signer, relays or Steam integration are currently healthy.

## API surface

| Operation | Parameters | Result | Wire |
| --- | --- | --- | --- |
| `info()` | none | `BridgeInfo` | `steamduck.info` / `steamduck.info.result` |
| `call(method, params?)` | store method name; named parameters | method-specific JSON value | `steamduck.request` / `steamduck.request.result` |
| `kwakore.call(method, params?)` | Kwakore control method name; named parameters | daemon result JSON value | `steamduck.kwakore.request` / `steamduck.kwakore.request.result` |

The web operations return promises. Successful calls resolve to the result
value directly. Failed calls reject with an error carrying `code`, `message`
and optional `reason` and `data`. The binding does not expose wire envelopes to
application code.

### BridgeInfo

| Field | Required | Type | Meaning |
| --- | --- | --- | --- |
| `version` | yes | integer | NAP-STEAMDUCK version; `1` for this draft. |
| `methods` | yes | list of text | Store convenience methods implemented by this host. |
| `kwakore_protocol_version` | yes | integer or null | Observed daemon protocol version, or null when unavailable. |

`info()` MUST remain usable when the daemon is unavailable. It is descriptive,
not an authorization handshake. Calling other operations does not require it.
Clients MUST reject unsupported bridge versions before issuing mutations.

### Direct Kwakore access

`kwakore.call` exposes the daemon's request/result control methods, including
administrative settings, signer selection, installation and window lifecycle
operations. It MUST NOT silently rewrite method names, parameters or successful
result schemas. The native host supplies JSON-RPC framing and daemon connection
details; the napplet supplies only the method and named parameters.

The daemon's protocol version, validation, permission checks, confirmations and
error semantics remain authoritative. New daemon methods do not require a new
NAP-STEAMDUCK version. The store MUST check `kwakore_protocol_version` before
depending on a particular daemon schema.

Native built-in registration and extension broker setup remain host-only
operations. If Kwakore exposes them through its administrative control socket,
the bridge MUST reject them with `Unsupported operation` rather than forwarding
them from iframe code. Broad daemon control access does not allow a napplet to
create a new trusted native endpoint. See the
[custom-domain implementation handoff](kwakore-custom-nap-handoff.md).

This v1 binding transports one terminal JSON result per request. Streaming
`events.subscribe` and attached launches (`attach: true`) need separate transport
contracts and MUST return `Unsupported operation` through this binding. Normal
window launches remain available; use the store `launch` and `try` operations
when Steam session inheritance is needed. Full control access does not imply
exposing raw file descriptors or executing caller-supplied commands.

The [local Kwakore reference snapshot](nap-steamduck-kwakore-reference.md)
records the current daemon schemas for reviewing this draft. It does not make
its socket framing or streaming mechanisms part of NAP-STEAMDUCK.

### Store convenience operations

These preserve the existing store backend's behavior. Their names intentionally
match the former `steamduckCall` method table (that binding has been removed). They are a separate namespace
from `kwakore.call`. Large catalog, installed and social lists use daemon page
schemas (`items`, `total`, `next_offset`); callers follow a non-null continuation.

| Method | Parameters | Successful result |
| --- | --- | --- |
| `status` | none | Daemon `service.status` result. |
| `discover` | optional `query`, `refresh`, `offset`, `limit`, `archetype`, `compatible` | Daemon `napplet.discover` result; default limit is 60. |
| `installed` | optional `offset`, `limit` | Page of daemon `InstalledItem` records; default limit 60. |
| `detail` | `address` | Daemon `napplet.detail` result. |
| `icon` | `address` | `ResourceReference` for a cached icon, or null when unavailable. |
| `install` | `address` | Daemon `napplet.install` result. |
| `update` | `address` | null after update completes. |
| `uninstall` | `address` | null after uninstall completes; this operation expresses confirmed uninstall intent. |
| `launch` | `address` | `LaunchResult`; opens an installed napplet in the store's Steam session where supported. |
| `try` | `address` | `LaunchResult`; opens a temporary napplet without installing it. |
| `sync` | none | `SyncResult`; reconciles Steam shortcuts. |
| `quit` | none | null; closes the store endpoint after acknowledgement. |
| `signer.status` | none | `SignerView`. |
| `signer.switch` | `mode`, optional `secret`, `socket` | `SignerView` after switching the host's signer. |
| `signer.pair.start` | none | `PairOffer`; host creates the pairing secret. |
| `signer.pair.wait` | none | `SignerView` after pairing completes. |
| `signer.pair.cancel` | none | null after the store's pending offer is cancelled. |
| `profile.get` | optional `refresh` boolean | `Profile` or null when signed out. |
| `permissions.get` | `address` | Daemon `napplet.permissions.get` result; empty saved rules are `[]`. |
| `permissions.clear` | `address`, `permission`, optional `subject`, `decision` | null; forgets the matching saved rule, without granting access. `decision` is accepted for existing UI compatibility and does not select the rule. |
| `settings.get` | none | Daemon non-secret `Settings`. |
| `settings.inspect` | none | `{settings:Settings,sources:{FIELD:"default"\|"config"\|"override"}}`. |
| `settings.reload` | none | `{settings:Settings}` after re-reading the declarative configuration. |
| `settings.clear` | `field` | null after removing that field’s local override. |
| `settings.set` | `field`, `value` | null after persisting the setting. |
| `napplet.social` | daemon parameters | Daemon result. |
| `napplet.reactions` | daemon parameters | Daemon result. |
| `napplet.comments` | daemon parameters | Daemon result. |
| `napplet.mentions` | daemon parameters | Daemon result. |
| `napplet.zaps` | daemon parameters | Daemon result. |
| `napplet.vote` | daemon parameters | Daemon result. |
| `napplet.react` | daemon parameters | Daemon result. |
| `napplet.comment` | daemon parameters | Daemon result. |
| `napplet.delete` | daemon parameters | Daemon result. |
| `napplet.share` | daemon parameters | Daemon result. |
| `napplet.zap` | daemon parameters | `ZapInvoice`; creates an invoice, not a payment. |

Daemon parameters and result schemas are recorded in the reference snapshot.
Addresses MUST use its canonical address format, not an `naddr1…` or `nostr:`
input. Social operations use the host's selected signer. Partial publication
results remain successful structured results; the host MUST NOT turn them into
an unqualified claim of successful delivery to every relay.

All parameter objects MUST be validated before dispatch. Unknown store methods
MUST fail with `Method not found`, never fall through to daemon dispatch. Unknown
fields, missing required fields and invalid types MUST fail with `Invalid params`.
The direct Kwakore namespace remains the explicit route for broader control.

### Store result schemas

| Type | Fields |
| --- | --- |
| `LaunchResult` | required `address` and `outcome` text; optional `window_id` text. `outcome: "opened"` acknowledges opening through the store, not that a game's initialization completed. |
| `SyncResult` | required `deferred` boolean and `shortcut_count` nonnegative integer; optional `steam_running` boolean and `changes` object mapping Steam account IDs to `ShortcutChanges`. A deferred result does not claim that Steam has loaded new shortcuts. |
| `ShortcutChanges` | optional `added`, `updated`, `removed` lists of text shortcut keys; required `app_ids` object mapping shortcut keys to unsigned 32-bit Steam app IDs. Omitted change lists mean no changes of that kind. |
| `SignerView` | required `mode` (`none`, `nsec`, `bunker`, `system`), `public_key` text (hex or empty), `connection_state` text, `npub` text (or empty). No credentials. |
| `PairOffer` | required `uri` text (`nostrconnect://`), `qr` text (PNG data URL), `relay` text. The URI and QR contain a one-time pairing secret. |
| `Profile` | required `public_key`, `npub` text and `found` boolean; optional `name`, `display_name`, `about`, `website`, `nip05`, `lud16`, `created_at` integer Unix seconds and `picture`, `banner` resource references. Image bytes are fetched through NAP-RESOURCE. `found: false` means no usable metadata was found, not that the account does not exist. |
| `ZapInvoice` | required `invoice` text (BOLT11), `qr` text (PNG data URL), `amount_msats` positive integer. |

Absent optional fields MUST be tolerated. Collections MUST use empty arrays or
objects rather than null unless their schema explicitly permits null.

### Resource references and bounds

`ResourceReference` is `{url: string, mime?: string}`. Catalog and detail icon
metadata retain their hash and MIME and add `icon.resource`. Cached icons use
`blossom:<64 lowercase hex hash>.<ext>`. RESOURCE checks locally cached original
bytes against that hash, then fetches through Blossom with the built-in's normal
fetch consent on a miss. Re-encoded desktop PNGs never stand in for differently
hashed originals. Profile references
use HTTPS. NAP-RESOURCE handles fetch consent, network policy and byte limits.
The store renders bounded raster blobs and revokes object URLs at teardown.

NAP-STEAMDUCK responses are limited to 262,144 bytes, including their envelope;
the proposed 8 MiB allowance is unnecessary now that these images are removed.
Convenience pages may return fewer records with an adjusted `next_offset` to
fit this bound. An oversized individual record or non-page result fails explicitly.
Direct daemon calls retain daemon schemas and fail on oversize; callers should
request smaller pages. Pairing and invoice QR codes remain bounded ephemeral
operation results rather than catalog/profile images.

## Wire protocol

Messages use the NAP envelope `{ "type": "domain.action", ...payload }`.
They are not raw JSON-RPC frames.

| Type | Direction | Fields |
| --- | --- | --- |
| `steamduck.info` | napplet → host | `id` |
| `steamduck.info.result` | host → napplet | `id`, exactly one of `result` or `error` |
| `steamduck.request` | napplet → host | `id`, `method`, optional `params` |
| `steamduck.request.result` | host → napplet | `id`, exactly one of `result` or `error` |
| `steamduck.kwakore.request` | napplet → host | `id`, `method`, optional `params` |
| `steamduck.kwakore.request.result` | host → napplet | `id`, exactly one of `result` or `error` |

`id` is a nonempty text correlation value, at most 128 UTF-8 bytes, unique among
pending requests from one endpoint generation across all operations. `method`
is nonempty text. `params`, when present, is an object or null; omitted and null
mean no named parameters. Successful void operations MUST include `result: null`.
All values must be JSON-serializable. Requests MUST NOT contain a caller identity,
socket path, generation override or authorization token.

The host MUST reject duplicate pending IDs without dispatching again or sending
a second terminal response for the original request. The binding MUST prevent
duplicates and ignore responses with an unknown ID or from an obsolete generation.
Malformed messages without a usable correlation ID MAY be discarded.

### Examples

```json
{"type":"steamduck.info","id":"info-1"}
{"type":"steamduck.info.result","id":"info-1","result":{"version":1,"methods":["status","installed","sync"],"kwakore_protocol_version":1}}
```

The example method list is illustrative; a host lists every convenience method
it implements.

```json
{"type":"steamduck.request","id":"sync-1","method":"sync"}
{"type":"steamduck.request.result","id":"sync-1","result":{"deferred":true,"shortcut_count":12}}
```

```json
{"type":"steamduck.kwakore.request","id":"settings-1","method":"settings.set","params":{"field":"theme","value":"dark"}}
{"type":"steamduck.kwakore.request.result","id":"settings-1","error":{"code":1004,"message":"Unavailable"}}
```

```js
const store = window.napplet.steamduck;
const installed = await store.call("installed");
await store.call("sync");
await store.kwakore.call("settings.set", { field: "theme", value: "dark" });
// Controller input is consumed through window.napplet.gamepad separately.
```

### Errors

`error` contains required integer `code` and text `message`, with optional text
`reason` and JSON object `data`. Daemon errors retain their codes and fixed
messages. Safe daemon `data.reason` MAY also appear as top-level `reason` for
existing store UI compatibility; if both exist they MUST agree.

| Code | Message | Use |
| ---: | --- | --- |
| -32600 | Invalid Request | Invalid NAP envelope or correlation fields. |
| -32601 | Method not found | Unknown convenience or daemon method. |
| -32602 | Invalid params | Parameters fail validation. |
| -32603 | Internal error | Bridge failure without a more specific public error. |
| 1001 | Unauthorized | Endpoint is not the current trusted store. |
| 1004 | Unavailable | Daemon or required native integration is unavailable. |
| 1007 | Closing | Endpoint or host is closing. |
| 1008 | Timeout | Request deadline elapsed; completion may be uncertain. |
| 1100 | Unsupported operation | Streaming, attached transport or host-only extension management requested through this binding. |

Other daemon codes, including partial cleanup and confirmation requirements,
are preserved as documented in the reference snapshot. Raw Go errors, paths,
credentials and internal diagnostic details MUST NOT be returned to the page.
Secrets supplied as input MUST NOT be reflected in errors or logs.

## Host behavior and request lifecycle

- Every valid authorized request receives exactly one terminal result while its
  endpoint remains alive. Responses may arrive out of order.
- Slow downloads, signer calls and pairing waits MUST NOT block unrelated
  browsing requests, controller delivery or cancellation of pairing.
- Requests MUST have finite host deadlines. The host MUST permit the existing
  store's budgets: 180 seconds for install/update, 135 seconds for pairing wait,
  90 seconds for launch/try, 70 seconds for social refresh/publication, 55 seconds
  for zap invoices, 45 seconds for discovery refresh and 30 seconds for signer
  switch/profile reads. Other convenience operations default to 30 seconds; direct Kwakore calls
  default to 180 seconds, subject to earlier daemon deadlines.
- The binding's timeout MUST allow the host deadline and transport overhead.
  A timeout is not proof that an installation, publication or setting change
  was rolled back. Mutations MUST NOT be automatically retried. The store
  re-reads relevant state before offering a retry.
- Closing an endpoint rejects its pending promises, stops response delivery and
  releases its native request resources. In-flight mutations may already have
  committed. Pairing cleanup MUST affect only offers owned by that endpoint;
  it MUST NOT cancel a replacement offer from another trusted generation.
- `quit` MUST acknowledge before requesting closure. It closes the store, not
  the Kwakore daemon or unrelated napplet windows.
- Launches retain Steam session behavior and independent napplet authorization.
  Returning focus to the store restores input through NAP-GAMEPAD's rules.
- Uninstall confirmation belongs in the store UI before calling the convenience
  operation. Direct daemon uninstall retains its explicit `confirm` parameter.
- Creating a zap invoice does not authorize or execute payment.

The domain MUST NOT expose arbitrary shell execution or credential readback.
Signer secrets are write-only; pairing offers are delivered only to their owning
trusted endpoint. Displaying untrusted catalog text, profiles and social content
inside this privileged store MUST treat them as data, never executable markup.

## Compatibility and acceptance criteria

Version 1 fixes the envelopes and convenience method semantics above. Additive
optional result fields and newly advertised convenience methods are compatible.
Changing required fields, meanings or result types requires a new bridge version.
The daemon protocol version remains independent.

An implementation is ready when these checks pass:

1. Only the bundled store receives the binding; a copied manifest or forged
   message from another endpoint cannot perform a privileged operation.
2. Reload and navigation invalidate the prior generation and its responses.
3. Store convenience calls preserve existing UI results, including null successes,
   paginated installed lists, pairing QR codes, errors and deferred Steam sync.
4. Direct control calls preserve daemon schemas and validation; streaming and
   attached calls fail explicitly rather than hanging or exposing descriptors.
5. Concurrent pairing/download requests leave catalog browsing and gamepad
   input responsive; timeouts do not cause automatic mutation retries.
6. NAP-GAMEPAD navigates the store on a physical Steam Deck, and launching a
   napplet transfers input without operating the background store.
7. Credentials never appear in status, errors or logs; closing the store leaves
   the daemon and independently running napplets alive.

## Implementations

The [runtime-injected binding](nap-steamduck-binding.md) implements the web API
using Kwakore's private lexical extension transport. It is built separately from
the app and embedded as `store.SteamduckPrelude()`. Native registration and the
handler are implemented in `internal/store/native.go` and `native_window.go`.
The host broker owns the authenticated socket; the child receives only its
Kwakore window protocol and approved extension routes.

The migration source for native handlers is the existing
[store dispatcher](../internal/store/api.go),
[account operations](../internal/store/account.go) and
[social operations](../internal/store/social.go). Their current webview and HTTP
transports remain available for compatibility testing.

Blossom references follow [BUD-10](https://github.com/hzrd149/blossom/blob/master/buds/10.md):
`blossom:<hash>.<ext>` with an explicit extension (`.bin` when unknown), repeated
`xs` server and `as` author hints, and an optional positive `sz` size hint.
The store includes the manifest publisher as an author hint. RESOURCE parses
hints before resolution, tries explicit servers before BUD-03 author lists and
fallback servers, and verifies content hash and advertised size. Kwakore retains
its HTTPS-only network policy; HTTP fallback is not enabled.

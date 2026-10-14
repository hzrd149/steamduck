# Kwakore control reference for NAP-STEAMDUCK

This is a local schema reference captured for the [NAP-STEAMDUCK draft](NAP-STEAMDUCK.md).
It describes the daemon protocol, not the NAP transport. Store convenience
operations override the daemon results where the draft explicitly says so.
Streaming events and attached launches require transport support outside the
v1 unary bridge. This reference does not grant access to an ordinary napplet.

Source: `kwakore/docs/control-protocol.md` in the adjacent Kwakore checkout.  
Checkout HEAD: `3fe5936b3f38ab91fccc217939e65c5093f11e07`.  
Captured: 2026-10-13. The snapshot includes the working tree's current contents;
HEAD alone does not identify any uncommitted documentation changes.  
Source file SHA-256: `6efc92187b30a55864febcc4e98e4fde61258b889e26fb7b88392b013d7c7b0b`.

The original document's service-guide links refer to the Kwakore repository.
The copied method, event and error sections below are a review reference;
NAP-STEAMDUCK's supported daemon schemas follow the connected daemon's reported
protocol version.

## Methods

All page offsets count records after sorting by canonical `address` ascending. Installed records with the same address use their internal key as a stable tie breaker. `offset` defaults to 0 and must be nonnegative; `limit` defaults to 100 and must be 1–500. `next_offset` is an integer when more records remain, otherwise `null`. `items` is an array, including when empty. A descriptor is `{address:string,name:string,format:string,available:boolean,version:Version}`; `name` is sanitized and at most 256 runes. `Version` is `{event_id:string,created_at:integer,artifact_hash:string}`. Addresses are full canonical Nostr addresses, `<kind>:<64 lowercase hex public key>:<identifier>`; the identifier is the `d` tag for kinds 35129 and 35130 and empty for a kind 15129 root napplet. A kind 5129 snapshot napplet is one immutable event with no Nostr address, so its canonical identifier is `5129:<64 lowercase hex public key>:<64 lowercase hex event id>`. The maximum address length is 4096 bytes. The wire is canonical-only: no method accepts an `naddr1…` or `nostr:` form. Clients that take human input, such as the bundled CLI, convert it to the canonical address before sending. Method input validation and mutation errors use the codes below.

| Method | Named params | `result` schema |
| --- | --- | --- |
| `service.status` | none | `{protocol_version:1,health:Health}` |
| `service.diagnostics` | none | `{observed_from:"live",health:Health,settings:Settings,recent_errors:DiagnosticError[],warning?:string,install_sync:InstallSync}` |
| `settings.get` | none | `Settings` |
| `settings.inspect` | none | `{settings:Settings,sources:{FIELD:"default"|"config"|"override",...}}` for each non-secret setting and signer selection |
| `settings.reload` | none | `{settings:Settings}` after re-reading declarative config; invalid reload keeps prior effective settings |
| `settings.set` | `{field:string,value:array<string>\|boolean\|string}` required (`sync_id`, `theme` and `social_refresh` take a string) | `{settings:Settings}` after persisting an override |
| `settings.clear` | `{field:string}` required | `{settings:Settings}` after removing one override |
| `signer.status` | none | `SignerStatus` |
| `signer.switch` | `{mode:"none"}`, `{mode:"nsec"\|"bunker",secret:string}` or `{mode:"system",socket?:string}` | final `SignerStatus` after the previous signer session ends |
| `signer.pair.start` | `{secret:string}` (32 lowercase hex characters, write only) | `{client_public_key:string,relay:string}` (public values only) |
| `signer.pair.wait` | none | final `SignerStatus` after validation, connection, and persistence |
| `signer.pair.cancel` | none | `{cancelled:boolean}` |
| `napplet.discover` | `{query?:string,refresh?:boolean,offset?:integer,limit?:integer,archetype?:string,intent?:string,domain?:string,compatible?:boolean,kind?:integer}` | `{items:Descriptor[],total:integer,next_offset:integer\|null,fetched_at:RFC3339 timestamp\|null,complete:boolean}` |
| `napplet.installed` | `{offset?:integer,limit?:integer}` | `{items:InstalledItem[],total:integer,next_offset:integer\|null}`, where `InstalledItem` is a `Descriptor` plus `update_available:Version\|null` |
| `napplet.updates.check` | `{address?:string}` | `{items:[{address:string,installed_version:Version,available_version:Version},...],complete:boolean}` |
| `napplet.detail` | `{address:string}` required | `Detail` |
| `napplet.versions` | `{address:string,refresh?:boolean,offset?:integer,limit?:integer}` | `HistoryPage` |
| `napplet.snapshots` | `{address:string,refresh?:boolean,offset?:integer,limit?:integer}` | `HistoryPage` |
| `napplet.social` | `{address:string,refresh?:boolean,categories?:("reactions"\|"comments"\|"mentions"\|"zaps")[]}`, `address` required | `{address:string,sync:{reactions:Sync,comments:Sync,mentions:Sync,zaps:Sync},votes:{up:integer,down:integer,score:integer},emoji:[{content:string,shortcode?:string,url?:string,count:integer},...],by_event:[{event_id:string,up:integer,down:integer,emoji:integer},...],comments:{count:integer,latest_at:integer\|null},mentions:{count:integer},zaps:{count:integer,total_msats:integer,top:[{pubkey:string,msats:integer,count:integer},...]},own:{vote:"up"\|"down"\|"none",vote_event_id:string,reactions:[{id:string,content:string,event_id:string},...]}\|null}`, where `Sync` is `{status:"not_requested"\|"answered"\|"partial"\|"unavailable"\|"timeout",synced_at:integer\|null}` |
| `napplet.reactions` | `{address:string,event_id?:string,offset?:integer,limit?:integer}`, `address` required | `{items:[{id:string,author:string,author_name?:string,content:string,shortcode?:string,url?:string,event_id:string,created_at:integer},...],total:integer,next_offset:integer\|null}` |
| `napplet.comments` | `{address:string,parent_id?:string,offset?:integer,limit?:integer}`, `address` required | `{items:[{id:string,author:string,author_name?:string,content:string,truncated:boolean,created_at:integer,parent_id:string,reply_count:integer,own:boolean},...],total:integer,next_offset:integer\|null}` |
| `napplet.mentions` | `{address:string,offset?:integer,limit?:integer}`, `address` required | `{items:[{id:string,author:string,author_name?:string,content:string,truncated:boolean,created_at:integer,quote:boolean},...],total:integer,next_offset:integer\|null}` |
| `napplet.zaps` | `{address:string,offset?:integer,limit?:integer}`, `address` required | `{items:[{receipt_id:string,sender:string,sender_name?:string,amount_msats:integer,comment:string,event_id:string,created_at:integer},...],total:integer,total_msats:integer,next_offset:integer\|null}` |
| `napplet.vote` | `{address:string,vote:"up"\|"down"\|"none",event_id?:string}`, `address` and `vote` required; no `event_id` with `none` | `{address:string,outcome:"published"\|"unchanged",event_id?:string,deleted:string[],published:integer,failed:integer}` |
| `napplet.react` | `{address:string,emoji:string,emoji_url?:string,event_id?:string}`, `address` and `emoji` required | `{address:string,outcome:"published",event_id:string,published:integer,failed:integer}` |
| `napplet.comment` | `{address:string,content:string,parent_id?:string}`, `address` and `content` required | `{address:string,outcome:"published",event_id:string,published:integer,failed:integer}` |
| `napplet.delete` | `{address:string,event_id:string}` required; `event_id` is one of the user's own reactions, comments or shares of the napplet | `{address:string,outcome:"published",deletion_id:string,published:integer,failed:integer}` |
| `napplet.share` | `{address:string,content?:string}`, `address` required | `{address:string,outcome:"published",event_id:string,published:integer,failed:integer}` |
| `napplet.zap` | `{address:string,amount_msats:integer,comment?:string,event_id?:string}`, `address` and `amount_msats` required | `{address:string,invoice:string,amount_msats:integer,recipient:string,zap_request_id:string}` |
| `napplet.icon` | `{address:string}` required | `{sha256:string,mime:"image/png",width:integer,height:integer,data:string}` |
| `napplet.windows` | none | `{items:[{window_id:string,address:string,name:string},...]}` for active napplet windows |
| `napplet.install` | `{address:string,relays?:string[],event_id?:string}`, `address` required | `{address:string,outcome:"installed"\|"updated"\|"reinstalled"\|"selected"\|"unchanged",installed_version:Version}` |
| `napplet.select` | `{address:string,event_id:string}` required | `{address:string,outcome:"selected"\|"unchanged",direction:"forward"\|"backward"\|"unchanged",previous_version:Version,installed_version:Version,pinned:true,data_reset:boolean}` |
| `napplet.update` | `{address:string}` required | `{address:string,outcome:"updated",previous_version:Version,installed_version:Version}` |
| `napplet.uninstall` | `{address:string,confirm:true}` required | `{address:string,outcome:"removed",previous_version:Version,cleanup_complete:true}` |
| `napplet.launch` | `{address:string}` required | `{address:string,window_id:string,outcome:"opened"}` after the checked child reports host-page `nap.start` |
| `napplet.try` | `{address:string,relays?:string[]}`, `address` required | `{address:string,window_id:string,outcome:"opened"}`, as `napplet.launch`, without installing |
| `napplet.stop` | `{window_id:string}` required | `{window_id:string,closed:true}` after that exact window has completed `WindowClosed` |
| `napplet.permissions.get` | `{address:string}` required | `{address:string,required_domains:string[],optional_domains:string[],saved_rules:SavedRule[]}` |
| `napplet.permissions.set` | `{address:string,permission:Permission,decision:"allow"\|"deny",subject?:string}` | `{address:string,permission:Permission,subject:string,decision:"allow"\|"deny"}` |
| `napplet.permissions.clear` | `{address:string,permission:Permission,subject?:string}` | `{address:string,permission:Permission,subject:string,cleared:boolean}` |
| `events.subscribe` | `{types?:string[]}` | `{subscribed:true,types:string[]}`, then the connection carries event notifications (see [events](#events)) |

`Health` is `{ready:boolean,version:string,uptime_seconds:number,config_status:"valid",storage_status:"open"\|"unhealthy"\|"closed",active_windows:integer}`. Readiness means initialized stores, valid active configuration, and acceptance of work; it does not imply a signer login or healthy relays. `Settings` is `{relays:string[],blossom_servers:string[],discover_on_user_relays:boolean,desktop_entries:boolean,gnome_search:boolean,sync_id:string,theme:"system"\|"light"\|"dark",social_refresh:string,signer:{mode:"none"\|"nsec"\|"bunker"\|"system",relay?:string,socket?:string}}`. Ordinary `settings.set` and `settings.clear` accept `relays`, `blossom_servers`, `discover_on_user_relays`, `desktop_entries`, `gnome_search`, `sync_id`, `theme`, and `social_refresh`; signer selection has its own method. `sync_id` is a JSON string: empty (sync off) or 1 to 64 printable ASCII characters without spaces. `theme` is a JSON string: `system` (the default), `light` or `dark`. It is the napplet window theme: new windows start with it and open windows switch at once. `system` follows the desktop's light/dark preference from the XDG desktop portal while the service runs, and is light without a portal. `social_refresh` is a JSON string: `"0"` or a Go duration from `15m` to `168h` (digits with the units `ns`, `us`, `ms`, `s`, `m` and `h`, no sign, at most 32 bytes), default `6h`. It is how often installed napplets' social data is refreshed in the background; `"0"` turns only that periodic refresh off. While `sync_id` is set, the service may install, update or uninstall napplets on its own to follow the synced list (see [syncing installed napplets](https://github.com/napplet/kwakore/blob/main/docs/service.md#syncing-installed-napplets)). Relay values must be canonical `wss://` URLs; Blossom values must be canonical `http://` or `https://` URLs. URL hosts must be lowercase, and duplicates, credentials, query strings, fragments, and explicit JSON `null` are invalid. Empty arrays and `false` are valid values. Clearing reveals the declarative or built-in value. `DiagnosticError` is `{category:string,time:RFC3339 timestamp,detail:string}`; at most 32 fixed, sanitized summaries are retained. The optional warning is also sanitized.

`InstallSync` is `{enabled:boolean,last_attempt:RFC3339 timestamp|null,last_success:RFC3339 timestamp|null,last_result:"ok"|"failed"|"unreadable"|"skipped"|null,napplets:integer}`, the status of the installed-list sync. `enabled` is true while `sync_id` is set. The other fields describe the passes for the current `sync_id` since the service started: they are `null` (and `napplets` 0) before the first pass, and again after `sync_id` changes until a pass for the new value ends. `last_result` is one fixed category: `ok` (the list was merged and published, or already agreed), `failed` (an install or uninstall from the list failed, the list could not be signed, or no relay accepted it), `unreadable` (the synced list could not be decrypted), or `skipped` (no signer). `last_success` is the end of the last `ok` pass, and `napplets` the size of the merged list that pass worked on. Nothing else about a pass, such as an address, relay or error, is reported.

`SignerStatus` is exactly `{mode:"none"|"nsec"|"bunker"|"system",public_key:string,connection_state:"connected"|"disconnected"}`. A connected public key is lowercase 64-character hex; a disconnected key is empty. Status never contains a credential, bunker URL, client key, or raw signer error. A switch ends the old signer and identity-bound windows before reporting the final new status. Failed local or bunker handshakes and credential writes return fixed `Unavailable`; `signer.status` then reports disconnected. Direct `bunker://` input is accepted only by the write method `signer.switch`; the URL and raw remote errors never appear in read results or diagnostics.

The optional top-level `signer` object in `config.json` accepts a non-secret mode and, for bunker only, a canonical `wss://` pairing relay. It never accepts a secret. Secret-bearing fields in declarative and override files are rejected before value formatting with a fixed field-name error. Effective precedence is explicit signer override, then declarative file, then built-in `none`. `signer.switch` writes the override and stores retained credentials separately in `signer-credentials.json` under the private XDG data directory. The file is owner-owned `0600` below a private `0700` directory. Bunker credentials retain the exact URL and a stable private NIP-46 client key together. Startup restores a requested signer only when its private record is valid; an invalid or missing bunker client key fails closed. Reload keeps the last valid effective file configuration if validation fails. A valid requested signer that cannot connect remains selected but disconnected, and reload returns a fixed failure. Ordinary settings reads and diagnostics remain public-only.

For nostrconnect, first call `signer.pair.start` with a locally generated random 16-byte secret encoded as 32 lowercase hex characters. The daemon begins a two-minute listener and returns only its public client key and relay. The initiating client constructs `nostrconnect://CLIENT_KEY?relay=...&secret=...&perms=...` locally and shares that **private pairing token only with the signer**. The daemon never returns the URI or one-time secret in any socket read. The relay comes from effective non-secret signer configuration, or `wss://bucket.coracle.social` when unset. Call `signer.pair.wait` to await the final public signer state; it reports connected only after the answer's signature, recipient, and one-time secret are checked, GetPublicKey succeeds, and the bunker credential is persisted. A wrong answer is ignored until timeout or a valid answer. `signer.pair.cancel` retires the offer; a later switch or service shutdown also cancels it. Linux does not support Amber/NIP-55.

`{mode:"system"}` signs through a signer service on a local Unix socket, which picks the key from the connecting user ([system signer protocol](system-signer.md)). `socket` is an absolute, clean path; without it, the switch uses the socket named in `config.json`, or `/run/nostr-signer.sock`. The switch persists only the non-secret selection; it neither writes nor removes `signer-credentials.json`. When the service does not answer yet, the final status is `system` and `disconnected`, and the daemon keeps retrying in the background until it connects or the signer is switched again.

Discovery searches the cached catalog case insensitively after trimming the query: the name, description, author, author name, archetypes and intent ids, or an exact event id. The optional filters match the built descriptor exactly and combine (every one given must hold): `archetype` is a member of `archetypes`, `intent` an `intents[].id`, `domain` a member of `required_domains` or `optional_domains`, `compatible` equals `compatible` (so `false` lists only napplets with missing domains), and `kind` equals `kind`. `archetype` and `domain` must be lowercase tokens (`^[a-z][a-z0-9-]*$`) of at most 64 bytes, `intent` 1–256 bytes of UTF-8 with no control or format characters, `compatible` a JSON boolean and `kind` a JSON integer, one of 5129, 15129, 35129 or 35130; anything else, including `null`, yields `Invalid params`. `total` and `next_offset` count after filtering. `refresh:false` reads that cache without network access; `refresh:true` waits for a refresh, which ends once every discovery relay has answered or failed, 15 seconds after the first relay finishes sending its stored events, or after 30 seconds, whichever comes first. A refresh that no relay answered yields `Unavailable` and keeps the cached catalog. `fetched_at:null` and `complete:false` mean no completed refresh has populated the cache. Install, update, and uninstall wait for a final committed outcome. `napplet.install` takes optional `relays`, the relay hints an `naddr` carried: at most 8 entries, each a `ws://` or `wss://` URL of at most 255 bytes with a host and no credentials, query or fragment, with no duplicates. Anything else, including `null`, yields `Invalid params`; an empty array is the same as omitting the field. The daemon uses the hints only as extra places to look for that one install, beside the author's write relays and the configured relays. Hints whose hosts are not public (loopback, private, `localhost`, `.local`, `.internal`) are skipped when the relays are asked. Hints are never stored, and every method other than `napplet.install` and `napplet.try` rejects a `relays` parameter. `napplet.uninstall` requires explicit `confirm:true` even for a notification; omitting it or sending `false` yields `Confirmation required` for a request with an ID. A partial file cleanup after removing the record yields error 1011 with only safe `data:{address,record_removed:true,cleanup_complete:false}`. Check `napplet.installed` after uncertain mutation outcomes.

`napplet.updates.check` looks for newer versions of installed napplets: every installed napplet, or only the one at `address`, which must be installed (`Not found` otherwise). It asks each napplet's relays the same way `napplet.update` does, eight napplets at a time, with every lookup bounded at 60 seconds together. It always asks, so what the relays say now replaces what an earlier check found. `items` lists, sorted by address, the checked napplets whose newest manifest is valid and newer than the installed one; a newest manifest that is invalid is never offered. `complete:false` means some napplet's relays did not answer; that napplet keeps the state an earlier check gave it. When no relay answers for any napplet the result is `Unavailable` and nothing changes. The service also runs this check in the background about two minutes after it starts and every six hours. Nothing is ever updated automatically: `napplet.update` installs a version, and it checks the version again first. Every `napplet.installed` item carries `update_available`: the newer valid version the last check found for that napplet, or `null`; its `available` field keeps describing the installed manifest. Kind 5129 snapshots are immutable: they are never checked or offered an update (a check of only a snapshot returns `{items:[],complete:true}`), and `napplet.update` on one returns `No update`.

`napplet.detail` describes the napplet at a canonical address from local knowledge only, without contacting relays: the installed record, else the cached discovery catalog, else the newest valid manifest in the archive or local event store; an address none of them knows yields `Not found`. `Detail` is a `Descriptor` plus `installed:boolean`, `installed_version:Version|null`, `update_available:Version|null` (as in `napplet.installed`) and `versions:Version[]`: the versions known locally (installed, offered update, catalog and stored manifests), newest first, at most 20. Use `napplet.versions` for paginated archived history. Its `kind`, `required_domains` and `optional_domains` (the manifest's declared NAP domains, `[]` when none) come from the descriptor. It contains no file path, server or blob URL, and the icon appears only as its hash.

`napplet.versions` lists every authentic publication Kwakore has archived for
the canonical address, including invalid manifests. `napplet.snapshots` lists
kind 5129 events whose valid `a` or `A` lineage references that address; it
requires a named (`35129`) or root (`15129`) napplet address. Same-author
snapshots have `relation:"author_snapshot"`; other-author snapshots have
`relation:"derivative"`. Lineage is provenance only: neither replaces the
mutable address's latest head or gives permission to use its identity. A
snapshot's own `napplet.versions` list contains its one immutable event with
`relation:"snapshot"`. Ordinary publications have `relation:"publication"`.

Both methods use `offset:0`, `limit:100` by default (limit 1–500). An unknown
address returns an empty page and can be refreshed before installation. Without
`refresh:true`, no relays are contacted. A refresh asks configured discovery
relays and author outboxes, with up to 500 distinct events per query and a
30-second overall lookup deadline. Snapshot refresh uses separate `#a` and `#A`
queries. Events accumulate in the archive across queries and daemon restarts;
refreshing never changes the installed version or deletes earlier observations.

`HistoryPage` contains:

- `items:HistoryItem[]`, `total:integer`, and `next_offset:integer|null`, ordered
  by descending publication time and ascending event ID for ties.
- `selected_version:Version|null` and `selected_manifest_available:boolean`.
  An existing installation may lack its original signed event; its parsed record
  is shown here without inventing an archived signed publication.
  `pinned:boolean` reports the installed selection policy (snapshots are
  inherently pinned).
- `latest_observed:HistoryItem|null`, independent of pagination. For a mutable
  address this is the latest authentic publication, even if invalid; snapshot
  listings keep that mutable head separate from snapshot timestamps.
- `lookup_status:"not_requested"|"answered"|"partial"|"unavailable"|"timeout"`.
  `answered` means every requested relay query reached EOSE; `partial` means
  some answered or sent events, but others failed, timed out, or the response
  limit was reached. `unavailable` means no usable relay answer;
  `timeout` means the lookup deadline elapsed without answers or events.
  Canceling the RPC itself uses the existing timeout error contract.
- `history_complete:false`, always: relays may discard replaced events, so even
  an answered refresh cannot establish a complete publication history.

`HistoryItem` contains the `Version` fields (`event_id`, `created_at`,
`artifact_hash`) plus `address`, full hex `author`, `kind`, `schema`, `name`,
`valid:boolean`, optional fixed `reason`, `relation`, `selected:boolean`,
`first_seen` and `last_seen` Unix seconds, `relays:string[]`, and
`lineage:Lineage|null`. Artifact hashes and lineage may be absent/empty for an
invalid manifest. Observation times describe when Kwakore recorded the event,
not when it was published. Relay provenance is deduplicated, capped at 64 URLs,
and excludes credentials. Full signed events are stored privately in
`manifest-history.db`; responses expose neither the event body nor local paths.
These history methods do not switch installed versions. `cached:boolean`
reports whether all of the
manifest's code files exist in the local artifact cache; cached bytes are
hash-verified again before installation, and a corrupt cache is never executed.

`napplet.install` with `event_id` installs that exact authentic publication and
pins it, including an older event at a mutable address. The ID must be 64
lowercase hex characters. `napplet.select` requires an existing installation
and moves its selection to that exact event. Both verify the event's author,
kind and identifier against `address`; a mismatch yields `Conflict`, a missing
event `Not found`, and an invalid manifest `Unavailable`. Neither silently
substitutes the latest publication. Snapshots keep their own event identities
and must be installed separately from mutable napplets; lineage cannot authorize
replacement. A snapshot address's identifier must equal the selected event ID.

Choosing a different event resets saved data and settings for both the previous
and selected artifact scopes, including metadata-only republications with the
same artifact hash. No older saves are restored. The switch returns `Busy` if
any window of that napplet is open; close it with `napplet.stop` first. Selecting
the same established pin is a no-op, with `data_reset:false`. Pinning an existing
unheld selection reports `unchanged` and preserves data. A pinned installation
still advertises updates, and an explicit `napplet.update` or latest install can
advance the pin. Background discovery and publication timestamps cannot move it.

Code downloads are retained in a separate `artifact-cache/`, including code
from installations made before this cache when their files are replaced.
Archived signed events plus verified cached artifacts allow offline selection.
An uncached version still requires available Blossom blobs. Uninstall removes
the active installation and its saved data, while signed history and cached code
remain available for reinstallation. There is no automatic cache pruning yet.

The mutation journal commits the selected record before deleting saved data.
A failed download or uncommitted mutation preserves the previous installation
and its data. A failure after commitment leaves the journal pending and returns
`Unavailable`; launch returns `Busy` until a daemon restart finishes recovery.
Inspect the installed record to determine the selected event after such a
failure. Recovery retries data reset before permitting execution. Permission
rules retain the existing address-based policy.

### Napplet reactions, comments and mentions

`napplet.social`, `napplet.reactions`, `napplet.comments` and
`napplet.mentions` read what other people published about a napplet: NIP-25
reactions (votes and emoji), NIP-22 comments and kind 1 notes that mention or
quote it. They work for any canonical address, installed or not. These four
read methods never publish anything; the actions below do. There are four
categories, `reactions`, `comments`, `mentions` and `zaps`; zaps (and
`napplet.zaps`) follow their own rules, described under "Napplet zaps".

Reads use only data Kwakore already holds. `napplet.social` with
`refresh:true` first asks relays: up to 12 of them, the author's NIP-65 inbox
and outbox relays (four each, public relays only), the discovery relays and the
user's read relays, with at most 500 events per query. `categories` narrows
which categories a refresh asks for (all of them by default); it must be a
non-empty array of distinct category names, and it never narrows the result.
A category synced less than 30 seconds ago is not asked again; the result then
shows what is held and the last status. One refresh runs per address at a
time, and refreshes of two addresses at a time; a concurrent refresh of the
same address waits for the running one. The server bounds a refresh at 60
seconds and a page read at 15 seconds. `sync` reports each category's last
refresh with the `lookup_status` vocabulary: `not_requested` (never asked,
`synced_at:null`), `answered`, `partial`, `unavailable` (no relay to ask, or
no usable answer) or `timeout`; `synced_at` is Unix seconds.

The daemon also refreshes every category of each installed napplet on its
own: once after an install, update or version selection, and again every
`social_refresh` (default `6h`; `"0"` turns off only this periodic refresh,
not the one after an install). The first periodic pass runs a few minutes
after the service starts; it refreshes the installed napplets one at a time,
and no pass runs while no relay is configured. These refreshes obey the same
30-second floor, the one-refresh-per-address rule and the two-address limit.
Napplets that are not installed are fetched only when a client asks. A client
follows the changes with the `social.updated` event (see
[events](#events)) instead of polling.

An event is about the napplet only when it names it exactly. A reaction
matches with an `a` tag equal to the canonical address or an `e` tag naming a
known version; a comment matches with its root scope, an `A` tag equal to the
address or an `E` tag naming a known version. Known versions are the newest 20
archived manifests of the address (see `napplet.versions`) plus the installed
event. A kind 5129 snapshot has no address, so only `e`/`E` equal to its event
id match it. A kind 15129 root napplet's address keeps its trailing colon
(`15129:<pubkey>:`); a value without it does not match. A kind 1 note
mentions the napplet with an `a` tag equal to the address (not for a
snapshot), or quotes it (NIP-18) with a `q` tag equal to the address (not for
a snapshot) or naming a known version; uppercase `A`/`Q` and `e` tags do not
count. A refresh asks for mentions with `{kinds:[1],#a:[ADDRESS]}` (never for
a snapshot) and `{kinds:[1],#q:[ADDRESS,...VERSION_IDS]}` (the address only
when it is not a snapshot); no filter carries an empty tag list. Every event
must have a valid id and signature, `created_at` at most 15 minutes in the
future, at most 200 tags and at most 8 KiB of content. A reaction's content
must be `+`, `-`, empty, an emoji of at most 64 bytes with no whitespace,
control or format characters, or a NIP-30 `:shortcode:` that carries its
`emoji` tag. Events that fail any check are not stored.

Deletions follow NIP-09: a refresh also asks for kind 5 events naming held
reactions, comments and mentions, and only a kind 5 signed by the event's own
author counts. It removes the event and is kept as a tombstone, so
the deleted event is never stored again. A kind 5 by anyone else is ignored
and not stored.

Votes are each author's latest `+`, empty (both up) or `-` reaction: the
newest `created_at`, and on a tie the lowest id. `score` is `up - down`. Any
other reaction is an emoji; `emoji` lists at most 20, counting distinct
authors per content, sorted by count, then content. A `:shortcode:` emoji has
its `shortcode` and, only when its image is an `https` URL with a host, no
user information and at most 512 bytes, its `url`. `by_event` splits votes
(each author's latest one, in its own bucket, so the buckets sum to `votes`)
and emoji by the version the reaction names in its first matching `e` tag;
`event_id:""` collects reactions that name only the address or an unknown
version. It lists at most 20 buckets, the busiest first. `comments` counts
the held comments and gives the newest `created_at`; `mentions` counts the
held mentions. `own` is the signed-in
user's latest vote (`vote_event_id` is that reaction, or empty) and their
emoji reactions, newest first, at most 20; it is `null` without a signer.

`napplet.reactions` lists reactions newest first (`created_at` descending,
then id ascending); `event_id` limits it to reactions on that version, and an
unknown id gives an empty page. `napplet.comments` lists the top-level
comments, or with `parent_id` the direct replies to that comment, newest
first. A comment's `parent_id` is the comment its lowercase `e` tag names, or
empty for a top-level comment; replies whose parent is not held appear at the
top level and keep their `parent_id`. `reply_count` counts held direct
replies, and `own` marks the user's comments. `napplet.mentions` lists the
mentions newest first; `quote` is true when a matching `q` tag is present and
false for a note that only has the `a` tag. All three pages use `offset:0`,
`limit:100` by default (limit 1–500). `author` is the full hex public key and
`author_name` an optional display name of at most 64 characters from the
local profile cache. Text is sanitized: control and format (bidi) characters
are removed, except newline and tab in comments and mentions, and a comment
or mention is cut at 2000 characters with `truncated:true`. `event_id` and
`parent_id` params must be 64 lowercase hex characters; `napplet.mentions`
takes neither.

Kwakore keeps these events in its local event store, with each address's sync
status in the kvstore. It keeps per author at most 50 comments, 50 mentions
and 20 emoji reactions for one napplet, and per address at most 2000
reactions, 1000 comments and 500 mentions, dropping the oldest. It tracks at
most 200 addresses and, past that, forgets the least recently synced address
that is not installed, together with its events. Uninstalling a napplet
removes its reactions, comments and mentions. The user's own events are never
removed by any of these limits.

These results never contain relay URLs or relay hints, any URL other than a
sanitized `https` emoji image, local paths, signatures, raw tags or raw
errors.

### Publishing reactions, comments and shares

`napplet.vote`, `napplet.react`, `napplet.comment`, `napplet.delete` and
`napplet.share` act
on a napplet with the service signer: each signs one event, publishes it and
applies the change to the local data at once, so `napplet.social` and the
pages show it without a refresh. Without a signer each returns 1004
`Unavailable` with exactly `data:{"reason":"signer_unavailable"}`, before
anything is signed. An event goes to the user's NIP-65 write relays (the
configured relays when there are none), plus up to 4 public inbox relays of
the napplet author and, for a reply, up to 4 of the parent comment's author.
Signing is bounded at 20 seconds (a slower signer gives `Timeout` and nothing
is published), publishing at 30 seconds, and the server bounds the call at 60
seconds. One action runs at a time; actions never wait for installs or
launches, nor block them. A signer answer for another key, or one that
changes the event or does not verify, gives `Unavailable` and nothing is
stored or published.

Reaching no relay is not an error: the result says `published:0` and the
event is kept locally (the next action or refresh does not resend it).
`published` and `failed` count the target relays that accepted and refused
or did not answer. Results never contain relay URLs, relay hints, keys,
signatures or raw errors.

A reaction (NIP-25 kind 7) has exactly these tags, in order: `["e",VERSION]`
(with the author's first public outbox relay as a third element when one is
known), `["p",AUTHOR]`, `["k",KIND]`, then `["a",ADDRESS]` except for a kind
5129 snapshot, which has no address (a kind 15129 address keeps its trailing
colon), then `["emoji",SHORTCODE,URL]` for a NIP-30 reaction. `VERSION` is
`event_id` when it names a known version (else `Not found`), otherwise the
installed event, otherwise the newest archived manifest. Without any known
version the result is `Not found`, because NIP-25 requires the `e` tag.

`napplet.vote` sets the user's vote. When the current vote (on that version,
with `event_id`) already is `vote`, the outcome is `unchanged`, `event_id` is
that vote reaction (absent for `none`) and nothing is published. Otherwise
`up` or `down` publishes a `+` or `-` reaction and, only when it reached at
least one relay, one kind 5 naming the user's previous vote reactions on the
napplet (at most 10, newest first) with `["k","7"]`; `deleted` lists those
withdrawn and they are removed locally. If the new vote reached no relay the
previous ones stay, but the new vote, dated after them, is the
current one locally. `none` publishes only that kind 5 and removes the
previous votes locally even when it reached no relay. `published` and
`failed` count the vote's relays, or for `none` the kind 5's.

`napplet.react` publishes an emoji reaction. `emoji` may not be `+`, `-` or
empty (use `napplet.vote`); it is at most 64 bytes with no whitespace,
control or format characters. A `:shortcode:` needs `emoji_url`, an `https`
URL with a host, no user information and at most 512 bytes; any other emoji
must not have one.

`napplet.comment` publishes a NIP-22 kind 1111 with non-blank `content` of
at most 8 KiB, sent as given. Its root is `["A",ADDRESS]` (`["E",EVENT_ID]`
for a snapshot), `["K",KIND]` and `["P",AUTHOR]`. A top-level comment then
names the napplet as its parent: `["a",ADDRESS]` (not for a snapshot),
`["e",VERSION]`, `["k",KIND]`, `["p",AUTHOR]`. A reply, with `parent_id`,
names the parent comment instead: `["e",PARENT]`, `["k","1111"]`,
`["p",PARENT_AUTHOR]`. The parent must be a comment on the same napplet that
Kwakore holds and that its author has not deleted; any other id is `Not
found`.

`napplet.share` publishes a kind 1 note that shares the napplet (a NIP-18
quote). Its content is `content` without trailing whitespace, a blank line and
the napplet's `nostr:naddr1…` URI (`nostr:nevent1…` for a kind 5129
snapshot); a blank or absent `content` publishes the bare URI. `content` is at
most 7 KiB, so the note stays within the 8 KiB every reader accepts. A kind
35129 or 15129 note has exactly the tags `["q",ADDRESS]` (with the author's
first public outbox relay as a third element when one is known),
`["p",AUTHOR]` and `["a",ADDRESS]` (a kind 15129 address keeps its trailing
colon). A snapshot has `["q",EVENT_ID,RELAY_OR_EMPTY,AUTHOR]` (NIP-18
positional) and `["p",AUTHOR]`, and no `a`. The URI carries that same relay
hint only inside the event; a hint over 255 bytes is dropped. NIP-19 stores
each value with a one-byte length, so an address whose d tag is over 255
bytes cannot be shared and gives `Invalid params`. The snapshot `nevent` has
no kind (NIP-19 makes it optional). Sharing works for any canonical address,
known locally or not, and needs no known version. The new note shows at once
as the user's own quote in `napplet.mentions`. The result never echoes the
text or the URI.

`napplet.delete` withdraws one of the user's own reactions, comments or
shares on the napplet: it publishes a kind 5 with `["e",EVENT_ID]` and
`["k",KIND]` (`["k","1"]` for a share), removes the event locally and keeps
the kind 5 as its tombstone. A share must be the user's own kind 1 that names
the address as above. An unknown id, another person's event and one about
another napplet (or no napplet) all give the same `Not found`. `deletion_id`
is the kind 5.

### Napplet zaps

Zaps (NIP-57) pay a napplet's author for it. Kwakore reads and totals the
zap receipts and asks for invoices, but it has no wallet: it never pays, and
a client pays the bolt11 invoice `napplet.zap` returns with its own wallet.

The provider lookup reads the `lud16` lightning address from the author's
kind 0 and its LUD-16 LNURL-pay endpoint,
`https://DOMAIN/.well-known/lnurlp/NAME`, over https to public addresses
only, with at most three https redirects and a 64 KiB answer; the answer is
cached for an hour. The author is zappable when the endpoint answers with
`allowsNostr:true` and a valid `nostrPubkey`, the key that signs the
author's receipts. A profile with only a `lud06` is not zappable.

A refresh of the `zaps` category does that lookup (bounded at 15 seconds),
then asks the refresh relays for `{kinds:[9735],authors:[PROVIDER],#a:[ADDRESS]}`
(never for a snapshot) and `{kinds:[9735],authors:[PROVIDER],#e:[VERSION_IDS]}`;
no filter carries an empty tag list. It records the provider with the
address's sync status, and reads count only receipts signed by that recorded
provider, so they never touch the network. When the author has no `lud16`,
an invalid one, or a provider without nostr support, `sync.zaps` is
`unavailable`, no receipt is asked for and the record is cleared, so nothing
counts until the author is zappable again. A failed or timed-out lookup is
also `unavailable` but keeps the record. Neither is an error.

A receipt counts only when all of these hold. It is a kind 9735 signed by
the recorded provider that names the napplet exactly with its own `a` (the
address, not for a snapshot) or `e` (a known version) tag, with a valid id
and signature, at most 200 tags, at most 8 KiB of content, `created_at` at
most 15 minutes ahead and a `description` of at most 16 KiB. That
`description` is a kind 9734 zap request with a valid id and signature,
exactly one `p` tag (the napplet's author), at most one `e` and one `a`
tag, and either an `a` equal to the address (not for a snapshot) or an `e`
naming a known version. Its `bolt11` names a positive amount of at most 21
million bitcoin, and every `amount` tag of the request equals that amount in
millisats. When the request has an `lnurl` tag, it equals the author's lnurl
(NIP-57 appendix F; compared case-insensitively). Receipts that fail a check
are never stored or counted. Several receipts that embed the same zap
request count once, as the newest.

NIP-09 is never applied to receipts: a receipt is the provider's record of a
payment, so a refresh asks for no kind 5 for them and ignores any it holds.
Kwakore keeps at most 1000 receipts per address, dropping the oldest, with no
per-sender cap, since one provider signs them all. Receipts also leave when
their address is evicted from the status index, and when the napplet is
uninstalled. The receipts of the user's own zaps (a request signed by the
user) are never dropped by any of these.

`napplet.social`'s `zaps` gives `count`, `total_msats` (a sum that stops at
the largest integer instead of overflowing) and `top`, at most 5 senders by
the zap request's pubkey, sorted by `msats`, then `count` (both descending),
then `pubkey`; `top` is `[]` when there are none. `napplet.zaps` pages the
receipts newest first with `offset:0`, `limit:100` by default (limit 1–500)
and no id param. `receipt_id` is the receipt, `sender` the zap request's
pubkey with an optional cached `sender_name`, `amount_msats` the invoice
amount, `comment` the request's content, sanitized like comments and cut at
280 characters, and `event_id` the version the request names, or `""`.
`total_msats` sums every receipt, not only the page.

`napplet.zap` asks the author's provider for an invoice of `amount_msats`
that zaps the napplet. It needs the signer: without one it returns 1004
`Unavailable` with `data:{"reason":"signer_unavailable"}` before any network
request. It then looks the provider up as above. An author who is not
zappable gives 1004 `Unavailable` with exactly
`data:{"reason":"not_zappable"}`. The answer must be a pay request
(`tag:"payRequest"`) with `1 <= minSendable <= maxSendable` and a callback
that is an `https` URL with a host, no user information, no fragment and at
most 2048 bytes; otherwise the result is `Unavailable` without data.
`amount_msats` must lie within `[minSendable,maxSendable]`, and `comment`
must have at most `commentAllowed` characters (any comment when the provider
allows none), else `Invalid params`. `event_id`, when given, must name a
known version (else `Not found`).

It then signs one kind 9734 with the service signer, with exactly these
tags, in order: `["relays",...]`, `["amount",AMOUNT_MSATS]`,
`["lnurl",LNURL]`, `["p",AUTHOR]`, `["e",VERSION]` (the given `event_id`,
else the installed version, else the newest archived one, else no `e`; a
snapshot's own id), `["a",ADDRESS]` (not for a snapshot) and `["k",KIND]`.
Its content is the comment. The relays are up to 3 of the user's read relays
(the configured relays when there are none) and up to 3 public inbox relays
of the author, else the discovery relays, at most 6. The zap request is
never published or stored. Kwakore requests the callback with the provider's
own query kept and `amount`, `nostr` (the signed request), `lnurl` and, only
when the provider allows comments and one is given, `comment` appended. The
answer's `pr` must be a bolt11 invoice whose amount equals `amount_msats`;
an invoice for another amount, a provider `status:"ERROR"` or anything else
gives `Unavailable` without data. The invoice's description hash is not
checked against the request, because Kwakore has no bolt11 decoder; only
its amount is. In the result, `invoice` is the lowercased bolt11, `recipient` the
author's hex pubkey and `zap_request_id` the signed request. Once the
client pays the invoice, the provider publishes the receipt to the
request's relays, and a later `napplet.social` refresh of `zaps` shows it.
Signing is bounded at 20 seconds and the server bounds the call at 45
seconds; either gives `Timeout`. The call holds the one social action slot
only while signing, so the provider requests never block other actions.

Results and errors of the zap methods never contain the lightning address,
the LNURL or callback URL, the provider's host or answer text, relay URLs,
keys, signatures, raw tags or raw errors; the only lightning string on the
wire is the bolt11 invoice.

`napplet.icon` returns the verified icon of the napplet at a canonical address (the installed record, else the catalog entry, else the newest stored manifest). The icon is fetched by its manifest hash from the user's, the manifest's and the author's Blossom servers. The source must match the hash, decode as the declared `image/png`, `image/jpeg` or `image/webp`, be at most 1 MiB and at most 1024×1024 pixels; it is then re-encoded as PNG and cached, so a later call reads the cache. `sha256` is the manifest's icon hash (never a URL), `mime` is always `image/png`, `width` and `height` are the PNG's size, and `data` is its standard base64. An unknown address or a napplet without a usable icon tag yields `Not found`; a fetch, verify or decode failure yields `Unavailable`. Neither error carries a URL or path. No event announces a cached icon: the service caches an installed napplet's icon in the background, and clients call `napplet.icon` after `napplet.installed` or `napplet.updated`, which fills the cache if needed.

`napplet.launch` accepts only a full canonical address already installed as a napplet. The response is a final opened outcome, not merely a spawned process. A child failure before its host page sends `nap.start` returns a fixed error and removes the window. With both `DISPLAY` and `WAYLAND_DISPLAY` empty, error 1004 carries exactly `data:{"reason":"session_unavailable"}` before any child starts. A stale display or unresponsive child is bounded by a 10-second host readiness wait and a 12-second daemon operation deadline. `window_id` is an opaque 32-character lowercase hex token, fresh for each service launch. `napplet.try` opens a napplet without installing it. An address already installed as a napplet opens exactly as `napplet.launch` would. Otherwise the daemon resolves the address as `napplet.install` does (with the same optional `relays` hints), downloads every file of the latest manifest into memory and checks each against its hash, and opens a trial window from that copy: nothing is written to the install directory, no desktop entry is made, and `napplet.installed` does not list it. Any file that fails to download or verify opens nothing and yields `Unavailable`. A trial makes no install offer when it closes; what it saved is kept only if the same version was installed while it ran, and is otherwise dropped. The downloads, bounded at 120 seconds, do not hold the daemon's operation lock, and the whole call is bounded at 150 seconds. A headless daemon returns the same `session_unavailable` error as `napplet.launch`, after the downloads and before any child starts. `napplet.stop` accepts that exact token, does not accept an internal napplet ID, and returns `Not found` for an unknown or already closed ID. The daemon confirms closure from the selected instance's `WindowClosed` signal; a timeout does not prove the window closed.

For Linux installation, place the existing hardened `kwaklet` child executable beside `kwakore`, with an adjacent `libwebview.so`. Both must be regular files under real path components owned by root or the current user and not writable by group or others; the executable must have an execute bit. The daemon resolves this sibling path from its own executable, never from the working directory or `PATH`. It sets `WEBVIEW_PATH` to the checked directory. The release archive (`kwakore-linux-ARCH.tar.gz`), the install helper and the Nix package all ship `kwakore`, `kwak`, `kwaklet` and `libwebview.so` in one directory; a sticky shared directory such as `/nix/store` above them is accepted.

Permission methods accept only the full canonical address of an installed napplet. `required_domains` and `optional_domains` are the manifest's declared network domains; they are separate from host `Permission` rules. `SavedRule` is `{permission:Permission,subject:string,decision:"allow"|"deny"}` and contains only persisted decisions for that address, sorted by permission then subject. Session-only answers never appear. `Permission` is one of `sign`, `encrypt`, `decrypt`, `publish`, `open_link`, `save_file`, `copy_text`, `upload`, `fetch`, `notify`, `media`, or `dispatch`. A `dispatch` rule requires a nonempty subject naming its action. A new `dispatch` allow is rejected unless that exact saved rule already has a handler target; this API has no handler-target parameter and cannot invent a valid one. Other permissions may use an optional subject. A subject must be valid UTF-8, at most 256 bytes, with no control characters. Set accepts only `allow` or `deny`; clear removes exactly one saved key and returns `cleared:false` when it was absent. Neither operation changes session-only answers. A saved allow does not override a NAP route declaration, an in-window prompt's owner, or any other runtime consent gate. Invalid enum, subject, decision, address, or parameter shape yields `Invalid params`; a missing installed address yields `Not found`; failed persistence yields `Unavailable` and restores the prior in-memory rule.

`Descriptor` includes `address`, `name`, `description`, `author` (the full
publisher pubkey in hex), optional cached `author_name`, `format`,
`available`, optional fixed `unavailable_reason`, optional validated `sources`,
and `version`. An invalid optional source tag is omitted; it does not make an
otherwise valid legacy NIP-5D napplet unavailable. Artifact and signature
validation still determine availability. A refresh drains events arriving
alongside EOSE before committing its catalog snapshot.

Every descriptor (in `napplet.discover`, `napplet.installed` and
`napplet.detail`) also carries the manifest fields below. Every list is an
array, `[]` when empty; every field is present.

`pinned:boolean` is true for an explicit installed selection or an immutable
snapshot. The discovered mutable manifest itself has no installation policy.

- `kind:integer`: the manifest kind, `5129`, `15129`, `35129` or `35130`.
- `schema:"nip5d"|"legacy"|"napp"`: `nip5d` is the current NIP-5D napplet
  manifest; `legacy` is the older NIP-5A-shaped napplet manifest with `path`
  tags, whose `requires` tags appear as `required_domains`; `napp` is a kind
  35130 napp.
- `icon:{sha256:string,mime:string}|null`: the manifest's icon tag, its hash
  and declared type (`image/png`, `image/jpeg` or `image/webp`), never a URL.
  `napplet.icon` serves the verified image.
- `archetypes:string[]`: the `z` tags, each a lowercase token of at most 64
  bytes, at most 32.
- `intents:[{id:string,params:string[]}]`: the `i` tags, at most 64; an id is
  at most 256 bytes with no control or format characters (others are
  dropped), with at most 16 parameter names.
- `required_domains:string[]`, `optional_domains:string[]`: the `R` and `O`
  NAP domains, lowercase tokens, at most 64 each.
- `missing_domains:string[]`: the required domains Kwakore does not
  implement. Such a napplet still launches, with a warning; `O` domains never
  count. `compatible:boolean` is true when nothing is missing.
- `lineage:{parent:string|null,root:string|null}|null`: a snapshot's `a` and
  `A` tags, canonical 35129 or 15129 addresses shown as provenance only and
  never resolved or trusted; `null` for every other kind and for a snapshot
  without usable lineage.

These are advertisements, never grants: no permission or NAP domain is
given because of them.

## Events

`events.subscribe` turns its connection into an event stream, so a client can follow the service without polling. `types` names the event types to receive: 1 to 10 distinct types from the table below. Omitting it, or sending `[]`, subscribes to every type; an unknown or repeated type, `events.overflow`, `null` or any other shape yields `Invalid params`. The result echoes the subscribed types, sorted. A connection subscribes at most once: a second `events.subscribe` on it, for example later in the same batch, yields `Conflict`. At most 16 connections may subscribe at once; one more yields `Busy` and stays an ordinary connection.

Once the frame holding the subscription has been answered, the server reads no further frames from that connection, which therefore has no idle read deadline, and writes one notification per event:

```json
{"jsonrpc":"2.0","method":"event","params":{"type":"window.opened","time":"2026-10-12T09:00:00Z","window_id":"0123456789abcdef0123456789abcdef","address":"35129:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:notes","name":"Notes"}}
```

A notification has no `id`. `params` always holds `type` and `time` (an RFC3339 timestamp) and only the fields of its type:

| Type | Fields | When |
| --- | --- | --- |
| `window.opened` | `window_id`, `address`, `name` | a napplet window listed by `napplet.windows` opened |
| `window.closed` | `window_id`, `address` | that window closed |
| `napplet.installed` | `address`, `outcome` (`installed`, `updated` or `reinstalled`), `installed_version:Version` | an install, including one made by the installed-list sync |
| `napplet.updated` | `address`, `previous_version:Version`, `installed_version:Version` | an update was applied |
| `napplet.uninstalled` | `address`, `cleanup_complete:boolean` | a napplet's record was removed |
| `updates.available` | `items:[{address,installed_version,available_version},...]` | the list of installed napplets with a newer valid version changed; `items` is the complete current list and may be `[]` |
| `signer.changed` | `signer:SignerStatus` | the public signer status changed |
| `social.updated` | `address`, `fields:string[]` (the changed categories: `reactions`, `comments`, `mentions`, `zaps`) | a refresh (asked for, periodic, or after an install, update or selection) added or removed held events, or the user's own vote, reaction, comment, share or deletion changed them; a zap invoice emits nothing; re-read with `napplet.social` |
| `settings.changed` | `fields:string[]` | settings changed through `settings.set`, `settings.clear` or `settings.reload`; it names the changed fields only, never their values, so read them with `settings.get` |
| `sync.completed` | `result` (an `InstallSync` result category), `ok:boolean`, `napplets:integer` | an installed-list sync pass ended |
| `events.overflow` | `dropped:integer` | always delivered, see below |

Explicit selections also emit the installed/updated events with optional
`pinned:boolean`, `data_reset:boolean`, and `outcome` (`selected` or `unchanged`
for an existing installation). Metadata-only pin changes preserve saved data.

Events arrive in the order they happened, and their times never decrease. No event carries a path, relay URL, key, secret or raw error. Each subscriber has a queue of 256 events. When a client reads too slowly and its queue fills, its further events are dropped and counted until the queued ones have been written; it then receives one `events.overflow` notification with the number dropped, and the stream goes on. After an overflow a client should re-read the state it follows with `napplet.installed`, `napplet.windows`, `signer.status`, `settings.get` and `service.diagnostics`. Each notification has the 30-second write deadline; a client that stops reading for longer is disconnected. The subscription ends when the client closes the connection, a write fails or the service shuts down; the server then closes the connection without a final notice. A subscription never delays shutdown.

## Errors

Errors use `{code:integer,message:string}` and, only for partial cleanup, headless launch, a social action without a signer or a zap of an author who cannot be zapped as described above, a safe `data` object. Error messages are fixed; paths, relay URLs, private keys, and raw internal errors are never returned. These are the wire codes and fixed messages:

| Code | Message | Meaning |
| ---: | --- | --- |
| -32700 | Parse error | Invalid JSON frame |
| -32600 | Invalid Request | Invalid JSON-RPC envelope, ID, frame, or batch |
| -32601 | Method not found | Method absent from this catalog |
| -32602 | Invalid params | Unknown, duplicate, positional, missing, or invalid named params |
| -32603 | Internal error | Result encoding or response limit failure |
| 1001 | Unauthorized | Peer UID denied |
| 1002 | Not found | Address or record missing |
| 1003 | Busy | Mutation conflicts with current work |
| 1004 | Unavailable | Daemon, discovery, storage, network, signer or window child unavailable; headless launch alone may carry `data:{"reason":"session_unavailable"}`, `napplet.vote`, `napplet.react`, `napplet.comment`, `napplet.delete`, `napplet.share` and `napplet.zap` without a signer carry `data:{"reason":"signer_unavailable"}`, and `napplet.zap` of an author without a lightning address that takes nostr zaps carries `data:{"reason":"not_zappable"}` |
| 1005 | No update | No newer version to install |
| 1006 | Invalid configuration | Reload or setting validation failed |
| 1007 | Closing | Shutdown has started |
| 1008 | Timeout | Server work or client wait timed out; inspect state |
| 1009 | Conflict | Mutation or refresh conflict |
| 1010 | Confirmation required | Uninstall intent missing |
| 1011 | Partial cleanup | Record removed; some cleanup failed |

Examples use synthetic addresses and paths:

```json
{"jsonrpc":"2.0","method":"service.status","id":7}
{"jsonrpc":"2.0","id":7,"result":{"protocol_version":1,"health":{"ready":true,"version":"development","uptime_seconds":12.5,"config_status":"valid","storage_status":"open","active_windows":0}}}
{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}
{"jsonrpc":"2.0","id":null,"error":{"code":1001,"message":"Unauthorized"}}
{"jsonrpc":"2.0","method":"napplet.uninstall","params":{"address":"35129:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:notes","confirm":true},"id":"remove-1"}
```

The unauthorized response is sent before dispatch and followed by close. `confirm:true` states uninstall intent; it is not a dry run.


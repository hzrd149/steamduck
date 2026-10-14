import { identityGetProfile } from "@napplet/nap/identity/sdk";
import { relayQuery, relayPublish } from "@napplet/nap/relay/sdk";
import { imageURL, type ResourceReference } from "./images";
import { intentOpen } from "@napplet/nap/intent/sdk";
import type { NostrEvent } from "@napplet/core";

export function hasIdentity(): boolean { return !!window.napplet?.identity; }
export function canEditProfile(): boolean { return hasIdentity() && !!window.napplet?.relay; }

export async function readProfile(publicKey: string) {
  const profile = await identityGetProfile();
  return { ...profile, public_key: publicKey, display_name: profile?.displayName,
    found: profile !== null, npub: publicKey };
}

// Both public identity URLs and native profile resource references use RESOURCE.
export function profileImage(reference: ResourceReference | string): Promise<string> { return imageURL(reference); }

export async function openUserProfile(pubkey: string): Promise<void> {
  const result = await intentOpen("napplet:profile/open", { payload: { pubkey } });
  if (!result.ok) throw new Error("No profile app was opened. Choose a profile app in Kwakore.");
}

export async function latestProfile(pubkey: string): Promise<NostrEvent | undefined> {
  const results = await relayQuery({ kinds: [0], authors: [pubkey], limit: 1 });
  return results.map((result) => result.event)
    .filter((event) => event.kind === 0 && event.pubkey === pubkey)
    .sort((a, b) => b.created_at - a.created_at || a.id.localeCompare(b.id))[0];
}

export function profileMetadata(event?: NostrEvent): Record<string, unknown> {
  if (!event) return {};
  const metadata: unknown = JSON.parse(event.content);
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) {
    throw new Error("Your existing profile could not be read. Refresh before editing.");
  }
  return metadata as Record<string, unknown>;
}

export async function publishProfile(pubkey: string, changes: Record<string, string>, baseline?: NostrEvent) {
  // Re-read before publishing to preserve fields written by another app.
  const latest = await latestProfile(pubkey);
  const base = !latest || baseline && baseline.created_at > latest.created_at ? baseline : latest;
  const metadata = profileMetadata(base);
  if ("display_name" in changes) delete metadata.displayName;
  for (const [field, value] of Object.entries(changes)) {
    if (value) metadata[field] = value;
    else delete metadata[field];
  }
  if (await window.napplet!.identity!.getPublicKey() !== pubkey) {
    throw new Error("Your account changed. Reopen the profile editor.");
  }
  const event = await relayPublish({ kind: 0, content: JSON.stringify(metadata),
    tags: base?.tags ?? [], created_at: Math.max(Math.floor(Date.now() / 1000), (base?.created_at ?? 0) + 1) });
  if (event.pubkey !== pubkey) throw new Error("Your account changed while publishing. Refresh your profile.");
  const visible: Record<string, string> = {};
  for (const field of ["name", "about", "picture", "banner", "website", "nip05", "lud16"]) {
    if (typeof metadata[field] === "string") visible[field] = metadata[field] as string;
  }
  const display = metadata.display_name ?? metadata.displayName;
  return { ...visible, display_name: typeof display === "string" ? display : "",
    public_key: pubkey, npub: pubkey, found: true };
}

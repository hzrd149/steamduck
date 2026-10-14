import type { BridgeInfo, SteamduckAPI } from "../nap-steamduck/types";
export type { BridgeInfo, SteamduckAPI } from "../nap-steamduck/types";

// NAP-STEAMDUCK and the draft GAMEPAD binding are provided by the host. This
// application never installs a shim or grants itself a runtime capability.
declare global {
  interface Window {
    napplet?: {
      steamduck?: SteamduckAPI;
      gamepad?: {
        available: boolean;
        focused: boolean;
        getGamepads(): Array<{
          buttons: Array<{ pressed: boolean }>;
          axes: number[];
        } | null>;
      };
      theme?: unknown;
    };
  }
}

let bridge: SteamduckAPI | undefined;
let info: Promise<BridgeInfo> | undefined;

export async function requireStore(): Promise<BridgeInfo> {
  const current = window.napplet?.steamduck;
  if (!current) throw new Error("Open the store in a supported Steamduck runtime.");
  if (current !== bridge || !info) {
    bridge = current;
    info = current.info().then((value) => {
      if (value.version !== 1) throw new Error("This Steamduck runtime needs an update.");
      return value;
    }).catch((error) => {
      info = undefined;
      throw error;
    });
  }
  return info;
}

export async function call(method: string, params?: Record<string, unknown> | null): Promise<any> {
  try {
    const metadata = await requireStore();
    if (!metadata.methods.includes(method)) throw new Error("This operation is unavailable in this Steamduck runtime.");
    return await window.napplet!.steamduck!.call<any>(method, params ?? null);
  } catch (error) {
    const publicError = error as { code?: number; reason?: string; data?: { reason?: string }; message?: string };
    const message = describeError({ code: publicError.code, message: publicError.message,
      reason: publicError.reason ?? publicError.data?.reason });
    throw Object.assign(new Error(message), { code: publicError.code });
  }
}

export function describeError(e: { code?: number; reason?: string; message?: string }): string {
  if (e.reason === "session_unavailable") return "kwakore has no display to open windows on. Run `steamduck doctor`.";
  if (e.reason === "signer_unavailable") return "This needs you to be signed in. Sign in on the Account tab.";
  if (e.reason === "not_zappable") return "The developer has no Lightning address that takes zaps.";
  switch (e.code) {
    case 1003: return "kwakore is busy with this napplet. Try again in a moment.";
    case 1004: return "Unavailable: the relays or the kwakore service did not answer.";
    case 1005: return "Already up to date.";
    case 1008: return "Timed out. Check the Library to see whether it finished.";
  }
  return e.message || "Something went wrong";
}

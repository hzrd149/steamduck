import type { SteamduckAPI } from "./api";

// Only the legacy build includes this adapter. The production napplet has no
// native binding, HTTP transport, token or browser-gamepad fallback.
export function installLegacyAdapter(): void {
  if (window.napplet?.steamduck) return;
  const native = (window as Window & {
    steamduckCall?: (method: string, params: unknown) => Promise<{ result?: unknown; error?: object }>;
  }).steamduckCall;
  const token = new URLSearchParams(location.hash.slice(1)).get("token");
  if (!native && !token) return;
  const methods = [
    "status", "discover", "installed", "detail", "icon", "install", "update", "uninstall", "launch", "try", "sync", "quit",
    "signer.status", "signer.switch", "signer.pair.start", "signer.pair.wait", "signer.pair.cancel", "profile.get",
    "permissions.get", "permissions.clear", "settings.get", "settings.set", "napplet.social", "napplet.reactions",
    "napplet.comments", "napplet.mentions", "napplet.zaps", "napplet.vote", "napplet.react", "napplet.comment",
    "napplet.delete", "napplet.share", "napplet.zap",
  ];
  const steamduck: SteamduckAPI = {
    async info() { return { version: 1, methods, kwakore_protocol_version: null }; },
    async call(method, params) {
      let reply;
      if (native) reply = await native(method, params ?? null);
      else {
        const response = await fetch("/api/" + method, {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Steamduck-Token": token! },
          body: JSON.stringify(params ?? null),
        });
        if (!response.ok) throw new Error("The store backend is unavailable.");
        reply = await response.json();
      }
      if (reply.error) throw Object.assign(new Error("Store request failed"), reply.error);
      return reply.result ?? null;
    },
    kwakore: { async call() { throw new Error("Direct control calls require the napplet runtime."); } },
  };
  window.napplet = { ...window.napplet, steamduck };
}

export function legacyGamepads(): Gamepad[] {
  return navigator.getGamepads ? [...navigator.getGamepads()].filter((pad): pad is Gamepad => !!pad) : [];
}

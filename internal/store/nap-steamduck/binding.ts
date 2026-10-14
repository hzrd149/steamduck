import type { BridgeInfo, PublicError, RequestType, SteamduckAPI } from "./types";

export class SteamduckError extends Error implements PublicError {
  readonly code: number;
  readonly reason?: string;
  readonly data?: Record<string, unknown>;

  constructor(error: PublicError) {
    super(error.message);
    this.name = "SteamduckError";
    this.code = error.code;
    this.reason = error.reason ?? (typeof error.data?.reason === "string" ? error.data.reason : undefined);
    this.data = error.data;
  }
}

/** Kwakore's lexical, document-owned extension helper; never a window global. */
export interface ExtensionTransport {
  call(type: RequestType, fields: Record<string, unknown>, deadlineMS?: number): Promise<unknown>;
}

export interface InstalledBinding {
  api: SteamduckAPI;
  /** Cancels local waits; it cannot roll back work already committed by the host. */
  dispose(): void;
}

const installations = new WeakMap<Window, InstalledBinding>();
const has = (object: object, field: string) => Object.prototype.hasOwnProperty.call(object, field);
const record = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);
const failure = (code: number, message: string) => new SteamduckError({ code, message });

// Copy strict JSON values before posting. JSON.stringify alone silently drops
// functions/undefined, changes non-finite numbers and calls arbitrary toJSON.
function jsonCopy(value: unknown, seen = new Set<object>()): any {
  if (value === null || typeof value === "string" || typeof value === "boolean") return value;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value !== "object" || value === null || seen.has(value)) throw failure(-32602, "Invalid params");
  const proto = Object.getPrototypeOf(value);
  if (!Array.isArray(value) && proto !== Object.prototype && proto !== null) throw failure(-32602, "Invalid params");
  if (Object.getOwnPropertySymbols(value).length) throw failure(-32602, "Invalid params");
  seen.add(value);
  let copy;
  if (Array.isArray(value)) {
    copy = [];
    for (let i = 0; i < value.length; i++) {
      const field = Object.getOwnPropertyDescriptor(value, String(i));
      if (!field || !has(field, "value")) throw failure(-32602, "Invalid params");
      copy.push(jsonCopy(field.value, seen));
    }
  }
  else {
    copy = Object.create(null);
    for (const key of Object.keys(value)) {
      const field = Object.getOwnPropertyDescriptor(value, key)!;
      if (!has(field, "value")) throw failure(-32602, "Invalid params");
      copy[key] = jsonCopy(field.value, seen);
    }
  }
  seen.delete(value);
  return copy;
}

// Host budget plus five seconds for transport and scheduling. These are local
// promise timeouts, never mutation retries or claims of native cancellation.
function timeout(type: RequestType, method?: string, params?: Record<string, unknown> | null): number {
  if (type === "steamduck.kwakore.request" || method === "install" || method === "update") return 185_000;
  if (method === "signer.pair.wait") return 140_000;
  if (method === "launch" || method === "try") return 95_000;
  if (method === "napplet.zap") return 60_000;
  if (method && ["napplet.social", "napplet.vote", "napplet.react", "napplet.comment", "napplet.delete", "napplet.share"].includes(method)) return 75_000;
  if (method === "discover" && params?.refresh === true) return 50_000;
  return 35_000;
}

/**
 * Installed only by the authorized runtime preamble. Kwakore supplies the
 * lexical transport and enforces endpoint/generation ownership independently.
 */
export function installSteamduckBinding(target: Window, transport: ExtensionTransport): InstalledBinding {
  const prior = installations.get(target);
  if (prior) return prior;
  const namespace = (target as Window & { napplet?: Record<string, unknown> }).napplet;
  if (target.parent === target || !record(namespace) || typeof transport?.call !== "function") {
    throw new Error("NAP-STEAMDUCK requires a hosted namespace and native extension transport");
  }
  if (has(namespace, "steamduck")) throw new Error("NAP-STEAMDUCK is already installed by another binding");
  let closed = false;
  const pending = new Set<(error: SteamduckError) => void>();

  function finishError(raw: unknown): SteamduckError {
    if (!record(raw) || !Number.isInteger(raw.code) || typeof raw.message !== "string" ||
        has(raw, "reason") && typeof raw.reason !== "string" || has(raw, "data") && !record(raw.data) ||
        record(raw.data) && typeof raw.data.reason === "string" && has(raw, "reason") && raw.reason !== raw.data.reason) {
      return failure(-32603, "Internal error");
    }
    if (raw.code === -32004) return failure(1008, "Timeout");
    if (raw.code === -32003) return failure(1003, "Busy");
    if (raw.code === -32000 && raw.reason === "document-ended") return failure(1007, "Closing");
    if (raw.code === -32001 && raw.reason === "invalid-request") return failure(-32602, "Invalid params");
    if ([-32000, -32001, -32002].includes(raw.code as number)) return failure(-32603, "Internal error");
    try { return new SteamduckError(jsonCopy(raw)); }
    catch { return failure(-32603, "Internal error"); }
  }

  function request(type: RequestType, method?: string, params?: Record<string, unknown> | null): Promise<unknown> {
    return new Promise((resolve, reject) => {
      if (closed) return reject(failure(1007, "Closing"));
      if (pending.size >= 64) return reject(failure(1003, "Busy"));
      let copied: Record<string, unknown> | null = null;
      try {
        if (type !== "steamduck.info" && (typeof method !== "string" || !method.length || params != null && !record(params))) {
          return reject(failure(-32602, "Invalid params"));
        }
        copied = jsonCopy(params ?? null);
      } catch { return reject(failure(-32602, "Invalid params")); }
      if (type === "steamduck.kwakore.request" && (method === "events.subscribe" ||
          ["napplet.launch", "napplet.try"].includes(method!) && copied?.attach === true ||
          method!.startsWith("runtime.builtin.") || method!.startsWith("runtime.extension."))) {
        return reject(failure(1100, "Unsupported operation"));
      }
      const cancel = (error: SteamduckError) => { pending.delete(cancel); reject(error); };
      pending.add(cancel);
      let response: Promise<unknown>;
      try {
        const fields = type === "steamduck.info" ? {} : { method, params: copied };
        response = transport.call(type, fields, timeout(type, method, copied));
      } catch { cancel(failure(1004, "Unavailable")); return; }
      Promise.resolve(response).then((value) => {
        if (!pending.delete(cancel)) return;
        try {
          const result = jsonCopy(value);
          if (type === "steamduck.info" && (!record(result) || !Number.isInteger(result.version) ||
              !Array.isArray(result.methods) || !result.methods.every((method) => typeof method === "string" && method.length > 0) ||
              !(result.kwakore_protocol_version === null || Number.isInteger(result.kwakore_protocol_version)))) {
            throw failure(-32603, "Internal error");
          }
          resolve(result);
        } catch { reject(failure(-32603, "Internal error")); }
      }, (error) => {
        if (pending.delete(cancel)) reject(finishError(error));
      });
    });
  }

  const api: SteamduckAPI = Object.freeze({
    info: () => request("steamduck.info") as Promise<BridgeInfo>,
    call: <T>(method: string, params?: Record<string, unknown> | null) => request("steamduck.request", method, params) as Promise<T>,
    kwakore: Object.freeze({
      call: <T>(method: string, params?: Record<string, unknown> | null) => request("steamduck.kwakore.request", method, params) as Promise<T>,
    }),
  });
  function dispose(): void {
    if (closed) return;
    closed = true;
    target.removeEventListener("pagehide", dispose);
    for (const cancel of pending) cancel(failure(1007, "Closing"));
    pending.clear();
  }
  target.addEventListener("pagehide", dispose);
  try {
    Object.defineProperty(namespace, "steamduck", { value: api, enumerable: true, writable: false, configurable: false });
  } catch (error) { dispose(); throw error; }
  const installation = { api, dispose };
  installations.set(target, installation);
  return installation;
}
